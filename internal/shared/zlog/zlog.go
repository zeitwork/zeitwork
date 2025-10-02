package zlog

// use slog for logging

import (
	"log/slog"
	"os"
)

type Config struct {
	Level   string
	Service string
}

func New(cfg Config) *slog.Logger {
	level := slog.LevelInfo
	if cfg.Level == "debug" || cfg.Level == "Debug" {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	logger := slog.New(handler)

	// Add service name to all logs if provided
	if cfg.Service != "" {
		logger = logger.With(slog.String("service", cfg.Service))
	}

	return logger
}
