package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deepkpat/pulse/pkg/api"
	"github.com/deepkpat/pulse/pkg/auth"
	"github.com/deepkpat/pulse/pkg/cache"
	"github.com/deepkpat/pulse/pkg/config"
	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/redis/go-redis/v9"
)

func main() {
	// load configuration (Precedence: env > yaml > code defaults)
	cfg := DefaultConfig()
	if err := config.Load("pulse.yaml", cfg); err != nil {
		slog.Warn("failed to load pulse.yaml, using defaults", "error", err)
	}
	cfg.ApplyEnvOverrides()

	// setup telemetry
	telemetry.InitLogger(cfg.Env)
	telemetry.RegisterMetrics()

	slog.Info("initializing application microservice", slog.String("env", cfg.Env))

	// generate a unique consumer name (hostname + random hex string)
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}

	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		slog.Error("failed to generate random bytes for consumer name", "error", err)
		os.Exit(1)
	}
	consumerName := fmt.Sprintf("%s-%x", hostname, randomBytes)

	// infrastructure setup
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
	eventQueue := queue.NewRedisQueue(rdb, cfg.Redis.StreamName, cfg.Redis.GroupName, consumerName)
	dedupCache := cache.NewDeduplicator(rdb, cfg.Redis.DedupTTL, "dedup:api")

	// initialize postgres connection pool
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.DBName, cfg.Postgres.SSLMode)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		slog.Error("failed to open postgres connection", "error", err)
		os.Exit(1)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	db.SetConnMaxLifetime(4 * time.Minute)

	if err := db.Ping(); err != nil {
		slog.Error("failed to ping postgres", "error", err)
		os.Exit(1)
	}

	pgStorage := auth.NewPostgresAuthenticator(db)
	defer pgStorage.Close()

	// initialize router & server specifications
	router := api.NewRouter(&api.RouterConfig{
		EventQueue: eventQueue,
		Auth:       pgStorage,
		Dedup:      dedupCache,
	})

	server := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// coordinate OS notification signals for closing down cleanly
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("http engine online", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server listener terminated unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	// block here until we intercept a shutdown request
	<-shutdownSignal
	slog.Info("shutdown invocation caught, flushing active processes...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("forced engine termination required", "error", err)
		os.Exit(1)
	}

	slog.Info("application exited cleanly")
}
