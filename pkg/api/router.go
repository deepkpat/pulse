package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/deepkpat/pulse/pkg/auth"
	"github.com/deepkpat/pulse/pkg/cache"
	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// responseWriter captures the HTTP status code for logging analytics
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

type RouterConfig struct {
	EventQueue queue.EventQueue
	Auth       auth.Authenticator
	Dedup      *cache.Deduplicator
}

func NewRouter(cfg *RouterConfig) http.Handler {
	mux := http.NewServeMux()

	// instantiate the handler with the injected dependency
	trackHandler := &TrackHandler{
		EventQueue: cfg.EventQueue,
		Dedup:      cfg.Dedup,
	}

	// register handlers (using Go 1.22+ routing enhancements)
	mux.HandleFunc("GET /health", HealthHandler)
	mux.Handle("GET /metrics", promhttp.Handler())

	// protected routes
	authMiddleware := auth.Middleware(cfg.Auth)
	mux.Handle("POST /track", authMiddleware(trackHandler))

	return LoggerMiddleware(mux)
}

func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		telemetry.HTTPRequestsInFlight.Inc()
		defer telemetry.HTTPRequestsInFlight.Dec()

		start := time.Now()

		// trace/correlation ID extraction
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = generateRequestID()
		}
		w.Header().Set("X-Request-ID", reqID)

		// create standard structured logger bound to this specific request
		logger := slog.Default().With(
			slog.String("request_id", reqID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("ip", r.RemoteAddr),
		)

		// pass it down the context chain
		ctx := telemetry.ToContext(r.Context(), logger)
		ctx = telemetry.ToRequestIDContext(ctx, reqID)
		r = r.WithContext(ctx)

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// graceful panic recovery to keep the process alive and log the disaster
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered during request",
					slog.Any("error", err),
					slog.Duration("latency_ms", time.Since(start)),
				)
				http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}

			// capture metrics even on panic
			elapsed := time.Since(start).Seconds()
			statusCode := fmt.Sprintf("%d", rw.statusCode)
			statusClass := fmt.Sprintf("%dxx", rw.statusCode/100)
			telemetry.HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path, statusClass).Observe(elapsed)
			telemetry.HTTPResponsesTotal.WithLabelValues(r.Method, r.URL.Path, statusCode).Inc()
		}()

		next.ServeHTTP(rw, r)

		// smart log-level assignment based on the HTTP status result
		logFn := logger.Info
		if rw.statusCode >= 500 {
			logFn = logger.Error
		} else if rw.statusCode >= 400 {
			logFn = logger.Warn
		}

		logFn("http request completed",
			slog.Int("status", rw.statusCode),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
	})
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
