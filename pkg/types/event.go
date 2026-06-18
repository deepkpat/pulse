package types

import "time"

// RawEvent represents the untrusted input from the client SDK before enrichment.
type RawEvent struct {
	EventID    string            `json:"event_id"`
	EventName  string            `json:"event_name"`
	UserID     string            `json:"user_id"`
	Timestamp  time.Time         `json:"timestamp"`
	Properties map[string]string `json:"properties"`
}

// Event is the validated, enriched, and sanitized record
// passing through our Redis stream and ClickHouse pipeline.
type Event struct {
	EventID    string            `json:"event_id"`
	EventName  string            `json:"event_name"`
	UserID     string            `json:"user_id"`
	Timestamp  time.Time         `json:"timestamp"`
	Properties map[string]string `json:"properties"`
	RequestID  string            `json:"request_id"`
}
