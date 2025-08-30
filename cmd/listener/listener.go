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
	// Load base configuration
	baseCfg, err := config.LoadListenerConfig()
	if err != nil {
		panic("Failed to load configuration: " + err.Error())
	}

	// Load NATS configuration
	natsCfg, err := config.LoadNATSConfigWithPrefix("LISTENER")
	if err != nil {
		panic("Failed to load NATS configuration: " + err.Error())
	}

	// Create logger
	logger := logging.NewLogger(baseCfg.ServiceName, baseCfg.LogLevel, baseCfg.Environment)

	// Create listener service configuration
	listenerCfg := &listener.Config{
		BaseConfig:          baseCfg,
		DatabaseURL:         getEnvWithDefault("LISTENER_DATABASE_URL", "postgres://postgres:root@localhost/zeitwork?sslmode=disable"),
		NATSConfig:          natsCfg,
		ReplicationSlotName: getEnvWithDefault("LISTENER_REPLICATION_SLOT", "zeitwork_listener"),
		PublicationName:     getEnvWithDefault("LISTENER_PUBLICATION_NAME", "zeitwork_changes"),
	}

	// Create the listener service
	service, err := listener.NewService(listenerCfg, logger)
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
		"environment", baseCfg.Environment,
		"log_level", baseCfg.LogLevel,
		"database_url", listenerCfg.DatabaseURL,
		"nats_urls", natsCfg.URLs,
		"publication", listenerCfg.PublicationName,
		"slot", listenerCfg.ReplicationSlotName,
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

// getEnvWithDefault gets an environment variable with a default value
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
