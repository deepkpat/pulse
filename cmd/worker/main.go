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

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/deepkpat/pulse/pkg/cache"
	"github.com/deepkpat/pulse/pkg/config"
	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/storage"
	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/deepkpat/pulse/pkg/worker"
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

	dedupCache := cache.NewDeduplicator(rdb)

	// initialize clickhouse storage
	chStorage, err := storage.NewClickHouseStorage(
		cfg.ClickHouse.Addr,
		cfg.ClickHouse.User,
		cfg.ClickHouse.Password,
		cfg.ClickHouse.Database,
	)
	if err != nil {
		slog.Error("failed to connect to clickhouse storage pool", "error", err)
		os.Exit(1)
	}

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
	wg.Wait()

	slog.Info("all workers exited cleanly")
}
