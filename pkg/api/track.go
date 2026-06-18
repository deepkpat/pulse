package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/deepkpat/pulse/pkg/types"
	"github.com/google/uuid"
)

type TrackHandler struct {
	EventQueue queue.EventQueue // dependency injected
}

func (h *TrackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := telemetry.FromContext(r.Context())

	// enforce a hard cap before any decoding to prevent OOM from large payloads
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024) // 64 KB max

	// decode the untrusted raw client payload
	var raw types.RawEvent
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		// distinguish between body-too-large and genuinely malformed JSON
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			telemetry.PayloadRejectedTotal.WithLabelValues("too_large").Inc()
			logger.Warn("request body exceeded size limit", "limit_bytes", maxBytesErr.Limit)
			http.Error(w, "Request body too large (max 64 KB)", http.StatusRequestEntityTooLarge)
			return
		}
		telemetry.PayloadRejectedTotal.WithLabelValues("bad_json").Inc()
		logger.Warn("malformed JSON payload rejected", "error", err)
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// basic validation rule: every event requires a name and id
	if raw.EventID == "" || raw.EventName == "" {
		telemetry.PayloadRejectedTotal.WithLabelValues("missing_fields").Inc()
		logger.Warn("missing required fields in event payload", "event_id", raw.EventID, "event_name", raw.EventName)
		http.Error(w, "Missing event_id or event_name", http.StatusUnprocessableEntity)
		return
	}

	// validate UUID format
	if _, err := uuid.Parse(raw.EventID); err != nil {
		telemetry.PayloadRejectedTotal.WithLabelValues("invalid_uuid").Inc()
		logger.Warn("invalid event_id format (must be UUID)", "event_id", raw.EventID, "error", err)
		http.Error(w, "event_id must be a valid UUID", http.StatusBadRequest)
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
		RequestID:  telemetry.GetRequestID(r.Context()),
		Properties: sanitizedProperties,
	}

	// push the event in the queue
	enqStart := time.Now()
	if err := h.EventQueue.Enqueue(r.Context(), event); err != nil {
		telemetry.EnqueueTotal.WithLabelValues("error").Inc()
		logger.Error("failed to enqueue event", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	telemetry.EnqueueDuration.Observe(time.Since(enqStart).Seconds())
	telemetry.EnqueueTotal.WithLabelValues("ok").Inc()
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
