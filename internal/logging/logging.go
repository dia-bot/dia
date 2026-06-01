// Package logging configures the process-wide structured logger.
//
// Dia uses log/slog everywhere. In development we emit human-readable text;
// in production we emit JSON for ingestion by log pipelines.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New builds a slog.Logger for the given level ("debug"|"info"|"warn"|"error")
// and environment ("development" emits text, anything else emits JSON).
func New(level, env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}

	var handler slog.Handler
	if env == "development" {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
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
