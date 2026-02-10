package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	_ "github.com/joho/godotenv/autoload"
	"github.com/lmittmann/tint"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/edgeproxy"
	"github.com/zeitwork/zeitwork/internal/zeitwork"
)

type Config struct {
	IPAdress               string `env:"LOAD_BALANCER_IP,required"`
	DatabaseURL            string `env:"DATABASE_URL,required"`
	DockerRegistryURL      string `env:"DOCKER_REGISTRY_URL,required"`
	DockerRegistryUsername string `env:"DOCKER_REGISTRY_USERNAME,required"`
	DockerRegistryPAT      string `env:"DOCKER_REGISTRY_PAT,required"` // GitHub PAT with write:packages scope
	GitHubAppID            string `env:"GITHUB_APP_ID"`
	GitHubAppPrivateKey    string `env:"GITHUB_APP_PRIVATE_KEY"` // base64-encoded

	// Edge proxy config
	EdgeProxyHTTPAddr    string `env:"EDGEPROXY_HTTP_ADDR" envDefault:":80"`
	EdgeProxyHTTPSAddr   string `env:"EDGEPROXY_HTTPS_ADDR" envDefault:":443"`
	EdgeProxyACMEEmail   string `env:"EDGEPROXY_ACME_EMAIL" envDefault:"admin@zeitwork.com"`
	EdgeProxyACMEStaging bool   `env:"EDGEPROXY_ACME_STAGING" envDefault:"false"`
}

func main() {
	logger := slog.New(tint.NewHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the service in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse configuration from environment variables
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		panic("failed to parse config" + err.Error())
	}

	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		panic("failed to init database")
	}

	service, err := zeitwork.New(zeitwork.Config{
		DB:                     db,
		IPAdress:               cfg.IPAdress,
		DatabaseURL:            cfg.DatabaseURL,
		DockerRegistryURL:      cfg.DockerRegistryURL,
		DockerRegistryUsername: cfg.DockerRegistryUsername,
		DockerRegistryPAT:      cfg.DockerRegistryPAT,
		GitHubAppID:            cfg.GitHubAppID,
		GitHubAppPrivateKey:    cfg.GitHubAppPrivateKey,
	})
	if err != nil {
		panic(err)
	}

	go func() {
		err = service.Start(ctx)
		if err != nil && err != context.Canceled {
			slog.Error("service error", "err", err)
		}
	}()

	// Edge proxy
	edgeProxy, err := edgeproxy.NewService(edgeproxy.Config{
		HTTPAddr:       cfg.EdgeProxyHTTPAddr,
		HTTPSAddr:      cfg.EdgeProxyHTTPSAddr,
		DatabaseURL:    cfg.DatabaseURL,
		UpdateInterval: 10 * time.Second,
		ACMEEmail:      cfg.EdgeProxyACMEEmail,
		ACMEStaging:    cfg.EdgeProxyACMEStaging,
	}, logger)
	if err != nil {
		slog.Error("failed to create edge proxy", "err", err)
	} else {
		go func() {
			if err := edgeProxy.Start(ctx); err != nil {
				slog.Error("edge proxy error", "err", err)
			}
		}()
	}

	sig := <-sigChan
	logger.Info("shutdown signal", "signal", sig)
	
	// Cancel context first so WAL listener and other ctx-dependent goroutines
	// start winding down in parallel with the shutdown sequence.
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if edgeProxy != nil {
		if err := edgeProxy.Stop(shutdownCtx); err != nil {
			slog.Error("edge proxy shutdown error", "err", err)
		}
	}
	service.Stop()
}
