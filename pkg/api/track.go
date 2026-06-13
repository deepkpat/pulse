package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/deepkpat/pulse/pkg/types"
)

type TrackHandler struct {
	EventQueue queue.EventQueue // dependency injected
}

func (h *TrackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	// push the event in the queue
	if err := h.EventQueue.Enqueue(r.Context(), event); err != nil {
		logger.Error("failed to enqueue event", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	logger.Debug("event processed successfully", "event_name", event.EventName)

	// return 202 Accepted for fast async pipeline execution
	w.WriteHeader(http.StatusAccepted)
}

// sanitizeProperties scrubs blacklisted keys to protect user privacy
func sanitizeProperties(props map[string]string) map[string]string {
	out := make(map[string]string, len(props))
	for k, v := range props {
		if PIIDenylist[strings.ToLower(k)] {
			out[k] = "[REDACTED]"
		} else {
			out[k] = v
		}
	}
	return out
}
