package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zeitwork/zeitwork/internal/edgeproxy"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/logging"
)

func main() {
	// Load configuration
	cfg, err := config.LoadEdgeProxyConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Load NATS configuration
	natsCfg, err := config.LoadNATSConfigWithPrefix("EDGEPROXY")
	if err != nil {
		log.Fatalf("Failed to load NATS configuration: %v", err)
	}

	// Create logger
	logger := logging.NewLogger(cfg.ServiceName, cfg.LogLevel, cfg.Environment)

	// Create native Go edge proxy service
	svc, err := edgeproxy.NewService(&edgeproxy.Config{
		DatabaseURL:        cfg.DatabaseURL,
		ConfigPollInterval: 30 * time.Second, // Keep polling as fallback
		NATSConfig:         natsCfg,
	}, logger)
	if err != nil {
		logger.Error("Failed to create edge proxy service", "error", err)
		os.Exit(1)
	}
	defer svc.Close()

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Info("Received shutdown signal")
		cancel()
	}()

	// Start the service
	logger.Info("Starting edge proxy service",
		"environment", cfg.Environment,
		"database_url", cfg.DatabaseURL,
		"nats_urls", natsCfg.URLs,
		"poll_interval", "30s",
	)

	if err := svc.Start(ctx); err != nil {
		logger.Error("Service failed", "error", err)
		os.Exit(1)
	}

	logger.Info("Edge proxy service stopped")
}
