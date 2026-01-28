package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/zeitwork/zeitwork/internal/reconciler"
)

type Config struct {
	DatabaseURL           string        `env:"RECONCILER_DATABASE_URL,required"`
	ReconcileInterval     time.Duration `env:"RECONCILER_INTERVAL" envDefault:"5s"`
	VMPoolSize            int           `env:"RECONCILER_VM_POOL_SIZE" envDefault:"3"`
	BuildTimeout          time.Duration `env:"RECONCILER_BUILD_TIMEOUT" envDefault:"10m"`
	DeploymentGracePeriod time.Duration `env:"RECONCILER_DEPLOYMENT_GRACE_PERIOD" envDefault:"5m"`
	LogLevel              string        `env:"RECONCILER_LOG_LEVEL" envDefault:"info"`
	AllowedIPTarget       string        `env:"NUXT_PUBLIC_DOMAIN_TARGET,required"`

	// Hetzner configuration
	HetznerToken           string `env:"RECONCILER_HETZNER_TOKEN"`
	HetznerSSHKeyName      string `env:"RECONCILER_HETZNER_SSH_KEY_NAME" envDefault:"zeitwork-reconciler-key"`
	HetznerServerType      string `env:"RECONCILER_HETZNER_SERVER_TYPE" envDefault:"cx23"`
	HetznerImage           string `env:"RECONCILER_HETZNER_IMAGE" envDefault:"ubuntu-24.04"`
	DockerRegistryURL      string `env:"RECONCILER_DOCKER_REGISTRY_URL"`
	DockerRegistryUsername string `env:"RECONCILER_DOCKER_REGISTRY_USERNAME"`
	DockerRegistryPassword string `env:"RECONCILER_DOCKER_REGISTRY_PASSWORD"`

	// SSH configuration
	SSHPublicKey  string `env:"RECONCILER_SSH_PUBLIC_KEY,required"`
	SSHPrivateKey string `env:"RECONCILER_SSH_PRIVATE_KEY,required"`
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

	logger.Info("starting reconciler",
		"reconcile_interval", cfg.ReconcileInterval,
		"vm_pool_size", cfg.VMPoolSize,
		"build_timeout", cfg.BuildTimeout,
		"deployment_grace_period", cfg.DeploymentGracePeriod,
		"log_level", cfg.LogLevel,
	)

	// Create reconciler service
	svc, err := reconciler.NewService(reconciler.Config{
		DatabaseURL:            cfg.DatabaseURL,
		ReconcileInterval:      cfg.ReconcileInterval,
		VMPoolSize:             cfg.VMPoolSize,
		BuildTimeout:           cfg.BuildTimeout,
		DeploymentGracePeriod:  cfg.DeploymentGracePeriod,
		AllowedIPTarget:        cfg.AllowedIPTarget,
		HetznerToken:           cfg.HetznerToken,
		HetznerSSHKeyName:      cfg.HetznerSSHKeyName,
		HetznerServerType:      cfg.HetznerServerType,
		HetznerImage:           cfg.HetznerImage,
		DockerRegistryURL:      cfg.DockerRegistryURL,
		DockerRegistryUsername: cfg.DockerRegistryUsername,
		DockerRegistryPassword: cfg.DockerRegistryPassword,
		SSHPublicKey:           cfg.SSHPublicKey,
		SSHPrivateKey:          cfg.SSHPrivateKey,
	}, logger)
	if err != nil {
		logger.Error("failed to create reconciler service", "error", err)
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
		cancel() // Cancel the context to stop the reconciler
	case err := <-errChan:
		logger.Error("reconciler error", "error", err)
		os.Exit(1)
	}

	// Give the service a moment to clean up
	time.Sleep(100 * time.Millisecond)

	if err := svc.Stop(); err != nil {
		logger.Error("error during shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("reconciler stopped gracefully")
}
