package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	nodeagent "github.com/zeitwork/zeitwork/internal/node-agent"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/logging"
)

func main() {
	// Load configuration
	cfg, err := config.LoadNodeAgentConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create logger
	logger := logging.NewLogger(cfg.ServiceName, cfg.LogLevel, cfg.Environment)

	// Create node agent service
	svc, err := nodeagent.NewService(&nodeagent.Config{
		Port:              cfg.Port,
		OperatorURL:       cfg.OperatorURL,
		NodeID:            cfg.NodeID,
		FirecrackerBin:    cfg.FirecrackerBin,
		FirecrackerSocket: cfg.FirecrackerSocket,
		VMWorkDir:         cfg.VMWorkDir,
	}, logger)
	if err != nil {
		logger.Error("Failed to create node agent service", "error", err)
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
	logger.Info("Starting node agent service",
		"port", cfg.Port,
		"environment", cfg.Environment,
		"operator_url", cfg.OperatorURL,
	)

	if err := svc.Start(ctx); err != nil {
		logger.Error("Service failed", "error", err)
		os.Exit(1)
	}

	logger.Info("Node agent service stopped")
}
