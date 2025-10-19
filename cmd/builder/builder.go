package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/zeitwork/zeitwork/internal/builder"
)

type Config struct {
	DatabaseURL   string        `env:"BUILDER_DATABASE_URL,required"`
	BuildInterval time.Duration `env:"BUILDER_INTERVAL" envDefault:"10s"`
	BuildTimeout  time.Duration `env:"BUILDER_TIMEOUT" envDefault:"30m"`

	GitHubAppID  string `env:"BUILDER_GITHUB_APP_ID,required"`
	GitHubAppKey string `env:"BUILDER_GITHUB_APP_KEY,required"`

	RegistryURL      string `env:"BUILDER_REGISTRY_URL,required"`
	RegistryUsername string `env:"BUILDER_REGISTRY_USERNAME,required"`
	RegistryPassword string `env:"BUILDER_REGISTRY_PASSWORD,required"`

	HetznerToken  string `env:"BUILDER_HETZNER_TOKEN"`
	SSHPublicKey  string `env:"BUILDER_SSH_PUBLIC_KEY,required"`
	SSHPrivateKey string `env:"BUILDER_SSH_PRIVATE_KEY,required"`

	LogLevel string `env:"BUILDER_LOG_LEVEL" envDefault:"info"`
}

func main() {
	// Parse configuration from environment variables
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		slog.Error("failed to parse config", "error", err)
		os.Exit(1)
	}

	// Setup logger
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("starting builder",
		"build_interval", cfg.BuildInterval,
		"build_timeout", cfg.BuildTimeout,
		"log_level", cfg.LogLevel,
	)

	// Create builder service
	svc, err := builder.NewService(builder.Config{
		DatabaseURL:      cfg.DatabaseURL,
		BuildInterval:    cfg.BuildInterval,
		BuildTimeout:     cfg.BuildTimeout,
		GitHubAppID:      cfg.GitHubAppID,
		GitHubAppKey:     cfg.GitHubAppKey,
		RegistryURL:      cfg.RegistryURL,
		RegistryUsername: cfg.RegistryUsername,
		RegistryPassword: cfg.RegistryPassword,
		HetznerToken:     cfg.HetznerToken,
		SSHPublicKey:     cfg.SSHPublicKey,
		SSHPrivateKey:    cfg.SSHPrivateKey,
	}, logger)
	if err != nil {
		logger.Error("failed to create builder service", "error", err)
		os.Exit(1)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the service in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		if err := svc.Start(ctx); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("received shutdown signal", "signal", sig.String())
		cancel() // Cancel the context to stop the builder
	case err := <-errChan:
		logger.Error("builder error", "error", err)
		os.Exit(1)
	}

	// Give the service a moment to clean up
	time.Sleep(100 * time.Millisecond)

	if err := svc.Stop(); err != nil {
		logger.Error("error during shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("builder stopped gracefully")
}
