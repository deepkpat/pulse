package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/deepkpat/pulse/pkg/cache"
	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/storage"
	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/deepkpat/pulse/pkg/worker"
	"github.com/redis/go-redis/v9"
)

const (
	streamName = "pulse_stream"
	groupName  = "pulse_worker_group" // must perfectly match the API's groupName
)

func main() {
	env := os.Getenv("PULSE_ENV")
	if env == "" {
		env = "development"
	}
	telemetry.InitLogger(env)

	// WORKER_CONCURRENCY controls how many parallel polling goroutines run within
	// this single process. each goroutine owns a dedicated RedisQueue instance with
	// a unique consumerName so that redis can track the PEL independently per
	// consumer — identical to running that many separate processes.
	concurrency := 1
	if v := os.Getenv("WORKER_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			slog.Error("invalid WORKER_CONCURRENCY value; must be a positive integer", "value", v)
			os.Exit(1)
		}
		concurrency = n
	}

	slog.Info("initializing worker daemon microservice",
		slog.String("env", env),
		slog.Int("concurrency", concurrency),
	)

	hostname, _ := os.Hostname()

	// infrastructure setup (shared across all goroutines — both are thread-safe)
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

	// ensure the consumer group exists (ignoring the error if it already does)
	_ = rdb.XGroupCreateMkStream(context.Background(), streamName, groupName, "0").Err()

	dedupCache := cache.NewDeduplicator(rdb)

	// initialize clickhouse storage
	chStorage, err := storage.NewClickHouseStorage(
		"localhost:9000",
		"pulse_admin",
		"pulse_super_secret_password",
		"pulse",
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
	for i := range concurrency {
		randomBytes := make([]byte, 4)
		rand.Read(randomBytes)
		consumerName := fmt.Sprintf("%s-worker-%x-%d", hostname, randomBytes, i)

		redisQueue := queue.NewRedisQueue(rdb, streamName, groupName, consumerName)
		daemon := worker.NewDaemon(redisQueue, dedupCache, chStorage, redisQueue)

		wg.Add(1)
		go daemon.Start(ctx, &wg)
	}

	// listen for OS interrupt signals
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	<-shutdownSignal
	slog.Info("shutdown signal received, instructing workers to finish current batch...",
		slog.Int("concurrency", concurrency),
	)

	// cancel the context; all goroutines will drain their current batch and exit
	cancel()
	wg.Wait()

	slog.Info("all workers exited cleanly")
}
