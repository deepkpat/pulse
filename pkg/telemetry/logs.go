package telemetry

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey struct{}

var (
	loggerKey    = ctxKey{}
	requestIDKey = ctxKey{}
)

// InitLogger sets up a global structured JSON logger based on the environment.
func InitLogger(env string) {
	var level slog.Level
	switch env {
	case "prod", "production":
		level = slog.LevelInfo
	default:
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: env != "prod" && env != "production", // disabling file source lines in prod saves cpu
	}

	// production standard: stream JSON to stdout
	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	// inject it as the global default logger
	slog.SetDefault(logger)
}

// ToContext embeds a request-scoped logger into the context.
func ToContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext extracts the logger from context or falls back to the global default.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// ToRequestIDContext embeds a request ID into the context.
func ToRequestIDContext(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// GetRequestID extracts the request ID from context or returns an empty string.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}
