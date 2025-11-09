package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/zeitwork/zeitwork/internal/edgeproxy"
)

type Config struct {
	HTTPAddr              string        `env:"EDGEPROXY_HTTP_ADDR" envDefault:":8080"`
	HTTPSAddr             string        `env:"EDGEPROXY_HTTPS_ADDR" envDefault:":8443"`
	DatabaseURL           string        `env:"EDGEPROXY_DATABASE_URL,required"`
	RegionID              string        `env:"EDGEPROXY_REGION_ID,required"`
	UpdateInterval        time.Duration `env:"EDGEPROXY_UPDATE_INTERVAL" envDefault:"10s"`
	ACMEEmail             string        `env:"EDGEPROXY_ACME_EMAIL,required"`
	ACMEStaging           bool          `env:"EDGEPROXY_ACME_STAGING" envDefault:"false"`
	ACMECertCheckInterval time.Duration `env:"EDGEPROXY_ACME_CERT_CHECK_INTERVAL" envDefault:"1h"`
	LogLevel              string        `env:"EDGEPROXY_LOG_LEVEL" envDefault:"info"`
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

	logger.Info("starting edgeproxy",
		"http_addr", cfg.HTTPAddr,
		"https_addr", cfg.HTTPSAddr,
		"region_id", cfg.RegionID,
		"update_interval", cfg.UpdateInterval,
		"acme_email", cfg.ACMEEmail,
		"acme_staging", cfg.ACMEStaging,
		"acme_cert_check_interval", cfg.ACMECertCheckInterval,
		"log_level", cfg.LogLevel,
	)

	// Create edgeproxy service
	svc, err := edgeproxy.NewService(edgeproxy.Config{
		HTTPAddr:              cfg.HTTPAddr,
		HTTPSAddr:             cfg.HTTPSAddr,
		DatabaseURL:           cfg.DatabaseURL,
		RegionID:              cfg.RegionID,
		UpdateInterval:        cfg.UpdateInterval,
		ACMEEmail:             cfg.ACMEEmail,
		ACMEStaging:           cfg.ACMEStaging,
		ACMECertCheckInterval: cfg.ACMECertCheckInterval,
	}, logger)
	if err != nil {
		logger.Error("failed to create edgeproxy service", "error", err)
		os.Exit(1)
	}

	// Start the service
	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		logger.Error("failed to start edgeproxy service", "error", err)
		os.Exit(1)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigChan
	logger.Info("received shutdown signal", "signal", sig.String())

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := svc.Stop(shutdownCtx); err != nil {
		logger.Error("error during shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("edgeproxy stopped gracefully")
}
