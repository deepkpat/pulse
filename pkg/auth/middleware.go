package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	pulserrors "github.com/deepkpat/pulse/pkg/errors"
	"github.com/deepkpat/pulse/pkg/telemetry"
)

// Authenticator defines the interface for validating API keys.
type Authenticator interface {
	ValidateAPIKey(ctx context.Context, key string) (bool, error)
}

// Middleware returns a http.Handler that authenticates requests using an API key.
func Middleware(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := telemetry.FromContext(ctx)
			key := r.Header.Get("X-API-Key")

			if key == "" {
				if authHeader := r.Header.Get("Authorization"); authHeader != "" {
					if len(authHeader) > 7 && strings.EqualFold(authHeader[:7], "bearer ") {
						key = authHeader[7:]
					}
				}
			}

			if key == "" {
				telemetry.AuthValidationsTotal.WithLabelValues("missing").Inc()
				http.Error(w, "Unauthorized: Missing API Key", http.StatusUnauthorized)
				return
			}

			authStart := time.Now()
			valid, err := auth.ValidateAPIKey(ctx, key)
			telemetry.AuthValidationDuration.Observe(time.Since(authStart).Seconds())

			if err != nil {
				// Classify the error so operators see transient vs permanent failures.
				classification := pulserrors.ClassifyError(err)

				if classification == "transient" {
					// Database is unreachable; return 503 so load balancers can retry.
					telemetry.AuthValidationsTotal.WithLabelValues("error_transient").Inc()
					logger.Warn("api key validation failed: database unavailable",
						slog.String("error", err.Error()),
					)
					http.Error(w, "Service Temporarily Unavailable", http.StatusServiceUnavailable)
				} else {
					// Other internal error; return 500.
					telemetry.AuthValidationsTotal.WithLabelValues("error_permanent").Inc()
					logger.Error("api key validation failed: internal error",
						slog.String("error", err.Error()),
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}

			if !valid {
				telemetry.AuthValidationsTotal.WithLabelValues("invalid").Inc()
				http.Error(w, "Unauthorized: Invalid API Key", http.StatusUnauthorized)
				return
			}

			telemetry.AuthValidationsTotal.WithLabelValues("ok").Inc()
			next.ServeHTTP(w, r)
		})
	}
}
