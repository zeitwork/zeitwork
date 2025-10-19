package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"
	"github.com/zeitwork/zeitwork/internal/nodeagent"
	"github.com/zeitwork/zeitwork/internal/shared/zlog"
)

func main() {
	// Initialize logger
	logger := zlog.New(zlog.Config{
		Level:   "info",
		Service: "nodeagent",
	})

	// Parse configuration from environment
	var cfg nodeagent.Config
	if err := env.Parse(&cfg); err != nil {
		logger.Error("failed to parse config", "error", err)
		os.Exit(1)
	}

	logger.Info("nodeagent starting",
		"node_id", cfg.NodeID,
		"region_id", cfg.NodeRegionID,
		"runtime_mode", cfg.NodeRuntimeMode,
	)

	// Create service
	svc, err := nodeagent.NewService(cfg, logger)
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
	logger.Info("nodeagent stopped")
}
