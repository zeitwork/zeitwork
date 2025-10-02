package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"
	"github.com/zeitwork/zeitwork/internal/edgeproxy"
	"github.com/zeitwork/zeitwork/internal/shared/zlog"
)

func main() {
	// Initialize logger
	logger := zlog.New(zlog.Config{
		Level:   "info",
		Service: "edgeproxy",
	})

	// Parse configuration from environment
	var cfg edgeproxy.Config
	if err := env.Parse(&cfg); err != nil {
		logger.Error("failed to parse config", "error", err)
		os.Exit(1)
	}

	logger.Info("edgeproxy starting",
		"edgeproxy_id", cfg.EdgeProxyID,
		"region_id", cfg.EdgeProxyRegionID,
		"port", cfg.EdgeProxyPort,
	)

	// Create service
	svc, err := edgeproxy.NewService(cfg, logger)
	if err != nil {
		logger.Error("failed to create service", "error", err)
		os.Exit(1)
	}

	// Setup graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start service in a goroutine
	go func() {
		if err := svc.Start(); err != nil {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sig := <-sigCh
	logger.Info("received shutdown signal", "signal", sig)

	// Cleanup
	svc.Close()
	logger.Info("edgeproxy stopped")
}
