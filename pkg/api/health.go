package api

import (
	"encoding/json"
	"net/http"

	"github.com/deepkpat/pulse/pkg/telemetry"
)

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	// extract the contextual logger containing this specific request ID
	logger := telemetry.FromContext(r.Context())

	logger.Debug("processing health verification query")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	res := map[string]string{"status": "ok"}
	if err := json.NewEncoder(w).Encode(res); err != nil {
		logger.Error("failed to serialize health payload", "error", err)
	}
}
