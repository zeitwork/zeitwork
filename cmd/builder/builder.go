package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"
	"github.com/zeitwork/zeitwork/internal/builder"
	"github.com/zeitwork/zeitwork/internal/shared/zlog"
)

func main() {
	// Initialize logger
	logger := zlog.New(zlog.Config{
		Level:   "info",
		Service: "builder",
	})

	// Parse configuration from environment
	var cfg builder.Config
	if err := env.Parse(&cfg); err != nil {
		logger.Error("failed to parse config", "error", err)
		os.Exit(1)
	}

	logger.Info("builder starting",
		"builder_id", cfg.BuilderID,
		"runtime_mode", cfg.BuilderRuntimeMode,
	)

	// Create service
	svc, err := builder.NewService(cfg, logger)
	if err != nil {
		logger.Error("failed to create service", "error", err)
		os.Exit(1)
	}

	// Setup graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start service in a goroutine
	go svc.Start()

	// Wait for shutdown signal
	sig := <-sigCh
	logger.Info("received shutdown signal", "signal", sig)

	// Cleanup
	svc.Close()
	logger.Info("builder stopped")
}
