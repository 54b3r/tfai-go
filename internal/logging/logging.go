// Package logging provides a structured logger built on [log/slog].
// It is configured once at startup via [New] and distributed through
// context values using [WithLogger] / [FromContext].
//
// Environment variables:
//
//	LOG_LEVEL  = debug | info | warn | error  (default: info)
//	LOG_FORMAT = json | text                  (default: json)
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// contextKey is an unexported type for context keys in this package.
type contextKey struct{}

// New constructs a [*slog.Logger] from environment variables.
// LOG_FORMAT selects the handler (json for production, text for local dev).
// LOG_LEVEL sets the minimum severity level.
func New() *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.ToLower(os.Getenv("LOG_FORMAT")) == "text" {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}

// WithLogger returns a copy of ctx carrying logger.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// FromContext returns the [*slog.Logger] stored in ctx.
// If no logger is present it returns [slog.Default] so callers never
// need to nil-check.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// parseLevel converts a string to a [slog.Level], defaulting to Info.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
