package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/zeitwork/zeitwork/internal/api"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/logging"
)

func main() {
	// Load configuration
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create logger
	logger := logging.NewLogger("api", cfg.LogLevel, cfg.Environment)

	// Connect to database
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create API service
	svc, err := api.NewService(&api.Config{
		Port:           cfg.Port,
		DatabaseURL:    cfg.DatabaseURL,
		GitHubClientID: cfg.GitHubClientID,
		GitHubSecret:   cfg.GitHubSecret,
		JWTSecret:      cfg.JWTSecret,
		BaseURL:        cfg.BaseURL,
	}, db, logger)
	if err != nil {
		logger.Error("Failed to create API service", "error", err)
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
	logger.Info("Starting public API service",
		"port", cfg.Port,
		"environment", cfg.Environment,
		"base_url", cfg.BaseURL,
	)

	if err := svc.Start(ctx); err != nil {
		logger.Error("Service failed", "error", err)
		os.Exit(1)
	}

	logger.Info("API service stopped")
}
