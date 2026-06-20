package health

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/redis/go-redis/v9"
)

// HealthMonitor runs periodic liveness checks against each backend connection
// and updates Prometheus gauges so operators can detect outages proactively.
// Any field can be nil — that backend's check is simply skipped.
type HealthMonitor struct {
	clickhouseCh  clickhouse.Conn
	postgresDB    *sql.DB
	redisClient   *redis.Client
	checkInterval time.Duration
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewHealthMonitor creates a new HealthMonitor. Pass nil for connections that
// are not used by the calling binary (e.g. API does not have ClickHouse).
func NewHealthMonitor(
	ch clickhouse.Conn,
	pg *sql.DB,
	rdb *redis.Client,
	interval time.Duration,
) *HealthMonitor {
	return &HealthMonitor{
		clickhouseCh:  ch,
		postgresDB:    pg,
		redisClient:   rdb,
		checkInterval: interval,
		stopCh:        make(chan struct{}),
	}
}

// Start begins periodic health checks. Designed to be called in a goroutine:
//
//	go monitor.Start(ctx)
//
// Exits when ctx is cancelled or Stop() is called.
func (h *HealthMonitor) Start(ctx context.Context) {
	h.wg.Add(1)
	defer h.wg.Done()

	ticker := time.NewTicker(h.checkInterval)
	defer ticker.Stop()

	slog.Info("health monitor started",
		slog.Duration("check_interval", h.checkInterval),
	)

	// Run initial checks immediately so metrics aren't 0 during the first interval.
	h.runChecks(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("health monitor stopping")
			return
		case <-h.stopCh:
			slog.Info("health monitor stopping (explicit stop signal)")
			return
		case <-ticker.C:
			h.runChecks(ctx)
		}
	}
}

// runChecks performs parallel PINGs to all non-nil backend connections.
func (h *HealthMonitor) runChecks(ctx context.Context) {
	var wg sync.WaitGroup

	if h.clickhouseCh != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.checkClickHouse(ctx)
		}()
	}

	if h.postgresDB != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.checkPostgres(ctx)
		}()
	}

	if h.redisClient != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.checkRedis(ctx)
		}()
	}

	wg.Wait()
}

// Stop gracefully stops the health monitor and waits for the goroutine to exit.
func (h *HealthMonitor) Stop() {
	close(h.stopCh)
	h.wg.Wait()
}

func (h *HealthMonitor) checkClickHouse(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	err := h.clickhouseCh.Ping(ctx)
	duration := time.Since(start)

	telemetry.ClickHousePingDuration.Observe(duration.Seconds())

	if err != nil {
		telemetry.ClickHouseConnectionHealth.Set(0)
		slog.Warn("ClickHouse health check failed",
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
	} else {
		telemetry.ClickHouseConnectionHealth.Set(1)
		slog.Debug("ClickHouse health check passed",
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
	}
}

func (h *HealthMonitor) checkPostgres(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	start := time.Now()
	err := h.postgresDB.PingContext(ctx)
	duration := time.Since(start)

	telemetry.PostgresPingDuration.Observe(duration.Seconds())

	if err != nil {
		telemetry.PostgresConnectionHealth.Set(0)
		slog.Warn("PostgreSQL health check failed",
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
	} else {
		telemetry.PostgresConnectionHealth.Set(1)
		slog.Debug("PostgreSQL health check passed",
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
	}
}

func (h *HealthMonitor) checkRedis(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	start := time.Now()
	err := h.redisClient.Ping(ctx).Err()
	duration := time.Since(start)

	telemetry.RedisPingDuration.Observe(duration.Seconds())

	if err != nil {
		telemetry.RedisConnectionHealth.Set(0)
		slog.Warn("Redis health check failed",
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
	} else {
		telemetry.RedisConnectionHealth.Set(1)
		slog.Debug("Redis health check passed",
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
	}
}
