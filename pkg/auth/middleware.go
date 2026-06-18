package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

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
				telemetry.FromContext(ctx).Error("api key validation failed", "error", err)
				telemetry.AuthValidationsTotal.WithLabelValues("error").Inc()
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
