package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	loadbalancer "github.com/zeitwork/zeitwork/internal/load-balancer"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/logging"
)

func main() {
	// Load configuration
	cfg, err := config.LoadLoadBalancerConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create logger
	logger := logging.NewLogger(cfg.ServiceName, cfg.LogLevel, cfg.Environment)

	// Create load balancer service
	svc, err := loadbalancer.NewService(&loadbalancer.Config{
		Port:        cfg.Port,
		OperatorURL: cfg.OperatorURL,
		Algorithm:   cfg.Algorithm,
	}, logger)
	if err != nil {
		logger.Error("Failed to create load balancer service", "error", err)
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
	logger.Info("Starting load balancer service",
		"port", cfg.Port,
		"environment", cfg.Environment,
		"algorithm", cfg.Algorithm,
	)

	if err := svc.Start(ctx); err != nil {
		logger.Error("Service failed", "error", err)
		os.Exit(1)
	}

	logger.Info("Load balancer service stopped")
}
