package api

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/deepkpat/pulse/pkg/telemetry"
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

func NewRouter() http.Handler {
	mux := http.NewServeMux()

	// register handlers (using Go 1.22+ routing enhancements)
	mux.HandleFunc("GET /health", HealthHandler)

	return LoggerMiddleware(mux)
}

func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			slog.Duration("latency_ms", time.Since(start)),
		)
	})
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
