package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/logging"
)

func main() {
	// Load configuration
	cfg, err := config.LoadCertsConfig()
	if err != nil {
		panic("Failed to load configuration: " + err.Error())
	}

	// Create logger
	logger := logging.NewLogger(cfg.ServiceName, cfg.LogLevel, cfg.Environment)

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
	logger.Info("Starting certs service",
		"environment", cfg.Environment,
		"log_level", cfg.LogLevel,
	)

	// Create a ticker that fires every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	logger.Warn("Certs service is not implemented yet - this is a placeholder service")

	// Main service loop
	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutting down certs service")
			return
		case <-ticker.C:
			logger.Warn("Certs service is not implemented - this is a placeholder service running every 10 seconds")
		}
	}
}
