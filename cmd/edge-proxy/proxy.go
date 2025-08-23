package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	edgeproxy "github.com/zeitwork/zeitwork/internal/edge-proxy"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/logging"
)

func main() {
	// Load configuration
	cfg, err := config.LoadEdgeProxyConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create logger
	logger := logging.NewLogger(cfg.ServiceName, cfg.LogLevel, cfg.Environment)

	// Create edge proxy service
	svc, err := edgeproxy.NewService(&edgeproxy.Config{
		Port:            cfg.Port,
		LoadBalancerURL: cfg.LoadBalancerURL,
		SSLCertPath:     cfg.SSLCertPath,
		SSLKeyPath:      cfg.SSLKeyPath,
		RateLimitRPS:    cfg.RateLimitRPS,
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
		"port", cfg.Port,
		"environment", cfg.Environment,
		"rate_limit", cfg.RateLimitRPS,
		"ssl", cfg.SSLCertPath != "",
	)

	if err := svc.Start(ctx); err != nil {
		logger.Error("Service failed", "error", err)
		os.Exit(1)
	}

	logger.Info("Edge proxy service stopped")
}
