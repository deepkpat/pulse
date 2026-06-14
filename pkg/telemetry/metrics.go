package telemetry

import "github.com/prometheus/client_golang/prometheus"

var (
	// --- API server ---

	// How many HTTP requests are in-flight right now.
	HTTPRequestsInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pulse_http_requests_in_flight",
		Help: "Current number of HTTP requests being served.",
	})

	// Full request latency histogram, broken out by route and status class.
	HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pulse_http_request_duration_seconds",
		Help:    "HTTP request latency.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5},
	}, []string{"method", "path", "status_class"})

	// Count of requests per status code — good for error-rate alerting.
	HTTPResponsesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pulse_http_responses_total",
		Help: "Total HTTP responses by method, path, and status code.",
	}, []string{"method", "path", "status_code"})

	// Auth outcomes — lets you see rejection rates per key separately.
	AuthValidationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pulse_auth_validations_total",
		Help: "API key validation outcomes.",
	}, []string{"result"}) // labels: "ok", "invalid", "missing", "error"

	// Tracks how long the Postgres auth lookup takes.
	AuthValidationDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pulse_auth_validation_duration_seconds",
		Help:    "Latency of Postgres API key lookups.",
		Buckets: prometheus.DefBuckets,
	})

	// Enqueue outcomes — critical for catching Redis write failures.
	EnqueueTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pulse_enqueue_total",
		Help: "Events sent to the Redis stream.",
	}, []string{"result"}) // labels: "ok", "error"

	// How long XADD takes — spikes indicate Redis pressure.
	EnqueueDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pulse_enqueue_duration_seconds",
		Help:    "Latency of Redis XADD calls.",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5},
	})

	// Payload rejections before the event reaches the queue.
	PayloadRejectedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pulse_payload_rejected_total",
		Help: "Events rejected at the API layer before enqueue.",
	}, []string{"reason"}) // labels: "too_large", "bad_json", "missing_fields"

	// --- Worker ---

	// Batches pulled from Redis — gives you throughput visibility.
	WorkerBatchesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pulse_worker_batches_total",
		Help: "Batches dequeued from Redis stream.",
	}, []string{"result"}) // labels: "ok", "error"

	// Events per batch — a histogram reveals whether batches are saturating.
	WorkerBatchSize = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pulse_worker_batch_size",
		Help:    "Number of events per dequeued batch.",
		Buckets: []float64{1, 10, 50, 100, 200, 512, 1024},
	})

	// Events dropped in the worker for any reason.
	WorkerEventsDropped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pulse_worker_events_dropped_total",
		Help: "Events dropped by the worker daemon.",
	}, []string{"reason"}) // labels: "invalid_uuid", "dlq_write_failed"

	// Duplicate events caught — useful for tuning the 16-min dedup TTL.
	WorkerDuplicatesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pulse_worker_duplicates_total",
		Help: "Duplicate events detected by the deduplication cache.",
	})

	// ClickHouse write latency — the most important worker SLO metric.
	StorageInsertDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pulse_storage_insert_duration_seconds",
		Help:    "End-to-end BulkInsert latency including retries.",
		Buckets: []float64{.01, .05, .1, .25, .5, 1, 2, 5, 10},
	})

	// ClickHouse write outcomes — track retries and permanent failures.
	StorageInsertsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pulse_storage_inserts_total",
		Help: "ClickHouse BulkInsert outcomes.",
	}, []string{"result"}) // labels: "ok", "error"

	// Retry attempts inside the backoff loop — spikes mean CH is struggling.
	StorageRetries = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pulse_storage_retries_total",
		Help: "ClickHouse insert retries due to transient errors.",
	})

	// Events written to ClickHouse per batch — throughput from the write side.
	StorageEventsInserted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pulse_storage_events_inserted_total",
		Help: "Total events successfully written to ClickHouse.",
	})

	// DLQ writes — any non-zero rate here warrants investigation.
	DLQWritesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pulse_dlq_writes_total",
		Help: "Messages written to the dead-letter queue.",
	}, []string{"reason", "result"}) // reason: "bad_json","invalid_uuid"; result: "ok","error"

	// Redis XACK failures — means events will be redelivered.
	CommitFailuresTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pulse_worker_commit_failures_total",
		Help: "Redis XACK failures that will cause batch redelivery.",
	})
)

// RegisterMetrics registers all metrics with the default Prometheus registry.
// Call this once at startup in both binaries.
func RegisterMetrics() {
	prometheus.MustRegister(
		HTTPRequestsInFlight,
		HTTPRequestDuration,
		HTTPResponsesTotal,
		AuthValidationsTotal,
		AuthValidationDuration,
		EnqueueTotal,
		EnqueueDuration,
		PayloadRejectedTotal,
		WorkerBatchesTotal,
		WorkerBatchSize,
		WorkerEventsDropped,
		WorkerDuplicatesTotal,
		StorageInsertDuration,
		StorageInsertsTotal,
		StorageRetries,
		StorageEventsInserted,
		DLQWritesTotal,
		CommitFailuresTotal,
	)
}
