package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/zeitwork/zeitwork/internal/certmanager"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/logging"
)

func main() {
	// Load configuration
	cfg, err := config.LoadCertManagerConfig()
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
	logger.Info("Starting certmanager service",
		"environment", cfg.Environment,
		"log_level", cfg.LogLevel,
		"provider", cfg.Provider,
	)

	svc, err := certmanager.NewService(cfg, logger)
	if err != nil {
		panic("Failed to create certmanager service: " + err.Error())
	}
	defer svc.Close()

	if err := svc.Start(ctx); err != nil {
		panic("Certmanager service failed: " + err.Error())
	}
}
