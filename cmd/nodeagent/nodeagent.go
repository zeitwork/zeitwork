package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/zeitwork/zeitwork/internal/nodeagent"
	"github.com/zeitwork/zeitwork/internal/nodeagent/config"
	"github.com/zeitwork/zeitwork/internal/shared/logging"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create logger
	logger := logging.NewLogger("nodeagent", getEnvOrDefault("LOG_LEVEL", "info"), cfg.Runtime.Mode)

	// Create node agent service
	svc, err := nodeagent.NewService(cfg, logger)
	if err != nil {
		logger.Error("Failed to create node agent service", "error", err)
		os.Exit(1)
	}

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
	logger.Info("Starting node agent service",
		"node_id", cfg.NodeID,
		"runtime_mode", cfg.Runtime.Mode,
		"poll_interval", cfg.PollInterval,
	)

	if err := svc.Start(ctx); err != nil {
		logger.Error("Service failed", "error", err)
		os.Exit(1)
	}

	logger.Info("Node agent service stopped")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
