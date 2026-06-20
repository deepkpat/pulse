package telemetry

import "github.com/prometheus/client_golang/prometheus"

var (
	// --- Connection Health Gauges (1=healthy, 0=unhealthy) ---

	ClickHouseConnectionHealth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pulse_clickhouse_connection_health",
		Help: "ClickHouse connection health status (1=healthy, 0=unhealthy).",
	})

	PostgresConnectionHealth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pulse_postgres_connection_health",
		Help: "PostgreSQL connection health status (1=healthy, 0=unhealthy).",
	})

	RedisConnectionHealth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pulse_redis_connection_health",
		Help: "Redis connection health status (1=healthy, 0=unhealthy).",
	})

	// --- Ping Latency Histograms ---

	ClickHousePingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pulse_clickhouse_ping_duration_seconds",
		Help:    "ClickHouse PING operation latency.",
		Buckets: []float64{.01, .05, .1, .25, .5, 1},
	})

	PostgresPingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pulse_postgres_ping_duration_seconds",
		Help:    "PostgreSQL PING operation latency.",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25},
	})

	RedisPingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pulse_redis_ping_duration_seconds",
		Help:    "Redis PING operation latency.",
		Buckets: []float64{.0001, .001, .005, .01, .025, .05},
	})
)

// RegisterClickHouseHealthMetrics registers ClickHouse-specific health metrics.
func RegisterClickHouseHealthMetrics() {
	prometheus.MustRegister(
		ClickHouseConnectionHealth,
		ClickHousePingDuration,
	)
}

// RegisterPostgresHealthMetrics registers PostgreSQL-specific health metrics.
func RegisterPostgresHealthMetrics() {
	prometheus.MustRegister(
		PostgresConnectionHealth,
		PostgresPingDuration,
	)
}

// RegisterRedisHealthMetrics registers Redis-specific health metrics.
func RegisterRedisHealthMetrics() {
	prometheus.MustRegister(
		RedisConnectionHealth,
		RedisPingDuration,
	)
}

