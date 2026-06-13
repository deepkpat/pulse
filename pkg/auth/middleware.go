package auth

import (
	"net/http"
	"strings"
)

// Authenticator defines the interface for validating API keys.
type Authenticator interface {
	ValidateAPIKey(key string) (bool, error)
}

// Middleware returns a http.Handler that authenticates requests using an API key.
func Middleware(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				// also check Authorization header with Bearer token format if needed,
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					key = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if key == "" {
				http.Error(w, "Unauthorized: Missing API Key", http.StatusUnauthorized)
				return
			}

			valid, err := auth.ValidateAPIKey(key)
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if !valid {
				http.Error(w, "Unauthorized: Invalid API Key", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
