package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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

	slog.Info("initializing worker daemon microservice", slog.String("env", env))

	// generate a unique consumer name
	hostname, _ := os.Hostname()
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	consumerName := fmt.Sprintf("%s-worker-%x", hostname, randomBytes)

	// infrastructure setup
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

	// ensure the consumer group exists (ignoring the error if it already does)
	_ = rdb.XGroupCreateMkStream(context.Background(), streamName, groupName, "0").Err()

	redisQueue := queue.NewRedisQueue(rdb, streamName, groupName, consumerName)
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

	// initialize the daemon
	daemon := worker.NewDaemon(redisQueue, dedupCache, chStorage, redisQueue)

	// setup graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// start the worker loop in a goroutine
	wg.Add(1)
	go daemon.Start(ctx, &wg)

	// listen for OS interrupt signals
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	<-shutdownSignal
	slog.Info("shutdown signal received, instructing worker to finish current batch...")

	// cancel the context to stop the polling loop, then wait for the current batch to finish
	cancel()
	wg.Wait()

	slog.Info("worker exited cleanly")
}
