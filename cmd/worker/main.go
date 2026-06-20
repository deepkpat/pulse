package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/deepkpat/pulse/pkg/cache"
	"github.com/deepkpat/pulse/pkg/config"
	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/storage"
	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/deepkpat/pulse/pkg/worker"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	// load configuration (precedence: env > yaml > code defaults)
	cfg := DefaultConfig()
	if err := config.Load("worker.yaml", cfg); err != nil {
		slog.Warn("failed to load worker.yaml, using defaults", "error", err)
	}
	cfg.ApplyEnvOverrides()

	// setup telemetry
	telemetry.InitLogger(cfg.Env)
	telemetry.RegisterMetrics()

	slog.Info("initializing worker daemon microservice",
		slog.String("env", cfg.Env),
		slog.Int("concurrency", cfg.Concurrency),
	)

	// spin up a lightweight metrics server on a separate port.
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		slog.Info("worker metrics server listening", "addr", cfg.MetricsAddr)
		if err := http.ListenAndServe(cfg.MetricsAddr, mux); err != nil {
			slog.Error("worker metrics server failed", "error", err)
		}
	}()

	hostname, _ := os.Hostname()

	// infrastructure setup (shared across all goroutines — both are thread-safe)
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})

	// ensure the consumer group exists (ignoring the error if it already does)
	_ = rdb.XGroupCreateMkStream(context.Background(), cfg.Redis.StreamName, cfg.Redis.GroupName, "0").Err()

	dedupCache := cache.NewDeduplicator(rdb, cfg.Redis.DedupTTL, "dedup:worker")

	// initialize clickhouse connection pool
	chConn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.ClickHouse.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.ClickHouse.Database,
			Username: cfg.ClickHouse.User,
			Password: cfg.ClickHouse.Password,
		},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		slog.Error("failed to open clickhouse connection pool", "error", err)
		os.Exit(1)
	}

	if err := chConn.Ping(context.Background()); err != nil {
		slog.Error("failed to ping clickhouse storage", "error", err)
		os.Exit(1)
	}

	if err := migrateClickHouse(context.Background(), chConn); err != nil {
		slog.Error("failed to run clickhouse migrations", "error", err)
		os.Exit(1)
	}

	chStorage := storage.NewClickHouseStorage(chConn)

	// setup graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// spin up one goroutine per concurrency slot; each gets its own RedisQueue so
	// that lastReadIDs and the Redis PEL are never shared or corrupted.
	for i := range cfg.Concurrency {
		randomBytes := make([]byte, 4)
		rand.Read(randomBytes)
		consumerName := fmt.Sprintf("%s-worker-%x-%d", hostname, randomBytes, i)

		redisQueue := queue.NewRedisQueue(rdb, cfg.Redis.StreamName, cfg.Redis.GroupName, consumerName)
		daemon := worker.NewDaemon(redisQueue, dedupCache, chStorage, redisQueue)

		wg.Add(1)
		go daemon.Start(ctx, &wg)
	}

	// listen for OS interrupt signals
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	<-shutdownSignal
	slog.Info("shutdown signal received, instructing workers to finish current batch...",
		slog.Int("concurrency", cfg.Concurrency),
	)

	// cancel the context; all goroutines will drain their current batch and exit
	cancel()

	// bounded wait for graceful shutdown to avoid hung processes
	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		slog.Info("all workers exited cleanly")
	case <-time.After(30 * time.Second):
		slog.Warn("graceful shutdown timed out; forcing exit")
	}
}

// migrateClickHouse runs idempotent DDL on startup so the application owns
// its own schema. This removes the need for Docker init-script mounts and
// mirrors how a managed cloud database would behave in production.
func migrateClickHouse(ctx context.Context, conn clickhouse.Conn) error {
	// read schema from infra/clickhouse/init.sql
	migrationPath := "infra/clickhouse/init.sql"
	ddl, err := os.ReadFile(migrationPath)
	if err != nil {
		return fmt.Errorf("failed to read clickhouse migration file (%s): %w", migrationPath, err)
	}

	if err := conn.Exec(ctx, string(ddl)); err != nil {
		return fmt.Errorf("clickhouse migration failed: %w", err)
	}
	slog.Info("clickhouse schema up to date")
	return nil
}
