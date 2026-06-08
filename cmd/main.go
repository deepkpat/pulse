package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deepkpat/pulse/pkg/api"
	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/redis/go-redis/v9"
)

const (
	streamName = "pulse_stream"
	groupName  = "pulse_worker_group" // constant across all instances
)

func main() {
	// setup environment
	env := os.Getenv("PULSE_ENV")
	if env == "" {
		env = "development"
	}
	telemetry.InitLogger(env)

	slog.Info("initializing application microservice", slog.String("env", env))

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
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	eventQueue := queue.NewRedisQueue(rdb, streamName, groupName, consumerName)

	// initialize router & server specifications
	router := api.NewRouter(&api.RouterConfig{
		EventQueue: eventQueue,
	})

	server := &http.Server{
		Addr:         ":8000",
		Handler:      router,
		ReadTimeout:  4 * time.Second,
		WriteTimeout: 8 * time.Second,
		IdleTimeout:  128 * time.Second,
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
