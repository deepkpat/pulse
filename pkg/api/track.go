package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/deepkpat/pulse/pkg/types"
)

// List of sensitive payload keys we want to scrub before ingestion
var PIIBlacklist = map[string]bool{
	"password":    true,
	"credit_card": true,
	"address":     true,
	"secret":      true,
	"token":       true,
}

func TrackHandler(w http.ResponseWriter, r *http.Request) {
	logger := telemetry.FromContext(r.Context())

	// decode the untrusted raw client payload
	var raw types.RawEvent
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		logger.Warn("malformed JSON payload rejected", "error", err)
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// basic validation rule: every event requires a name and id
	if raw.EventID == "" || raw.EventName == "" {
		logger.Warn("missing required fields in event payload", "event_id", raw.EventID, "event_name", raw.EventName)
		http.Error(w, "Missing event_id or event_name", http.StatusUnprocessableEntity)
		return
	}

	// map and sanitize properties
	sanitizedProperties := sanitizeProperties(raw.Properties)

	// enrich into our reliable internal Event model
	event := types.Event{
		EventID:    raw.EventID,
		EventName:  raw.EventName,
		UserID:     raw.UserID,
		Timestamp:  time.Now().UTC(), // capture server-side truth arrival time
		Properties: sanitizedProperties,
	}

	// TODO: push the event into redis streams
	logger.Debug("event processed successfully", "event_name", event.EventName)

	// return 202 Accepted for fast async pipeline execution
	w.WriteHeader(http.StatusAccepted)
}

// sanitizeProperties scrubs blacklisted keys to protect user privacy
func sanitizeProperties(props map[string]string) map[string]string {
	if props == nil {
		return make(map[string]string)
	}

	for k := range props {
		normalizedKey := strings.ToLower(k)
		if PIIBlacklist[normalizedKey] {
			props[k] = "[REDACTED]"
		}
	}
	return props
}
