package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/zeitwork/zeitwork/internal/listener"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/logging"
)

func main() {
	// Load listener configuration
	cfg, err := config.LoadListenerConfig()
	if err != nil {
		panic("Failed to load configuration: " + err.Error())
	}

	// Create logger
	logger := logging.NewLogger(cfg.ServiceName, cfg.LogLevel, cfg.Environment)

	// Create the listener service
	service, err := listener.NewService(cfg, logger)
	if err != nil {
		logger.Error("Failed to create listener service", "error", err)
		panic("Failed to create listener service: " + err.Error())
	}
	defer service.Close()

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
	logger.Info("Starting listener service",
		"environment", cfg.Environment,
		"log_level", cfg.LogLevel,
		"database_url", cfg.DatabaseURL,
		"nats_urls", cfg.NATS.URLs,
		"publication", cfg.PublicationName,
		"slot", cfg.ReplicationSlotName,
	)

	// Run the service
	if err := service.Start(ctx); err != nil {
		if err == context.Canceled {
			logger.Info("Listener service stopped gracefully")
		} else {
			logger.Error("Listener service failed", "error", err)
			os.Exit(1)
		}
	}
}
