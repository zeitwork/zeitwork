package logging

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a new structured logger with the appropriate level and format
func NewLogger(serviceName string, level string, environment string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	if environment == "production" {
		// Use JSON format in production for better parsing by log aggregators
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		// Use text format in development for readability
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)

	// Add default attributes
	logger = logger.With(
		slog.String("service", serviceName),
		slog.String("environment", environment),
	)

	return logger
}
