package errors

import "strings"

// IsTransientError returns true if the error is due to temporary conditions
// (network issues, timeouts, unavailable services) that may resolve on retry.
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	// Transient/Network error patterns
	transientPatterns := []string{
		// Network layer
		"connection refused",
		"connection reset",
		"connection broken",
		"broken pipe",
		"dial timeout",
		"dial tcp",

		// I/O and timeouts
		"i/o deadline exceeded",
		"context deadline exceeded",
		"eof",
		"read: connection reset",
		"write: broken pipe",

		// DNS resolution
		"no such host",
		"unknown host",
		"name resolution failed",

		// Database-specific transient messages
		"server is unavailable",
		"temporarily unavailable",
		"too many connections",
		"server closed the connection",
		"connection pool closed",
		"lost connection to mysql server",
		"connection lost",

		// ClickHouse specific
		"tcp_socket_receive_timeout_exceeded",
		"connection closed by peer",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	return false
}

// IsPermanentError returns true if the error indicates a permanent condition
// (schema mismatch, auth failure, invalid input) that won't resolve on retry.
func IsPermanentError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	permanentPatterns := []string{
		// Auth errors
		"auth failed",
		"authentication failed",
		"access denied",
		"permission denied",
		"invalid api key",
		"unauthorized",

		// Schema errors
		"schema mismatch",
		"invalid type",
		"no such table",
		"table does not exist",
		"column does not exist",
		"unknown column",

		// Syntax/validation errors
		"syntax error",
		"malformed",
		"invalid syntax",
		"parse error",

		// Data validation
		"constraint violation",
		"duplicate key",
		"unique constraint",
		"primary key",
		"foreign key",

		// JSON/codec errors
		"json error",
		"unmarshal error",
		"invalid json",
		"codec error",
	}

	for _, pattern := range permanentPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	return false
}

// ClassifyError categorizes an error as "ok", "transient", or "permanent".
// Unknown errors default to "transient" to be conservative (prefer retries over data loss).
func ClassifyError(err error) string {
	if err == nil {
		return "ok"
	}
	if IsPermanentError(err) {
		return "permanent"
	}
	if IsTransientError(err) {
		return "transient"
	}
	// Unknown errors: treat as transient to be conservative
	return "transient"
}
