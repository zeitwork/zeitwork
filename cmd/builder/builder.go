package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/zeitwork/zeitwork/internal/builder"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/logging"
)

func main() {
	// Load configuration
	cfg, err := config.LoadBuilderConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create logger
	logger := logging.NewLogger(cfg.ServiceName, cfg.LogLevel, cfg.Environment)

	// Create builder service
	svc, err := builder.NewService(cfg, logger)
	if err != nil {
		logger.Error("Failed to create builder service", "error", err)
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
	logger.Info("Starting builder service",
		"environment", cfg.Environment,
		"database_url", cfg.DatabaseURL,
		"builder_type", cfg.BuilderType,
		"max_concurrent_builds", cfg.MaxConcurrentBuilds,
	)

	if err := svc.Start(ctx); err != nil {
		logger.Error("Service failed", "error", err)
		os.Exit(1)
	}

	logger.Info("Builder service stopped")
}
