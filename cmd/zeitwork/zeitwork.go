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
	"github.com/zeitwork/zeitwork/internal/storage"
	"github.com/zeitwork/zeitwork/internal/zeitwork"
)

type Config struct {
	IPAdress               string `env:"LOAD_BALANCER_IP,required"`
	DatabaseURL            string `env:"DATABASE_URL,required"`        // PgBouncer pooled connection (for queries)
	DatabaseDirectURL      string `env:"DATABASE_DIRECT_URL,required"` // Direct connection (for WAL replication)
	InternalIP             string `env:"INTERNAL_IP,required"`         // This server's VLAN IP
	DockerRegistryURL      string `env:"DOCKER_REGISTRY_URL,required"`
	DockerRegistryUsername string `env:"DOCKER_REGISTRY_USERNAME,required"`
	DockerRegistryPAT      string `env:"DOCKER_REGISTRY_PAT,required"` // GitHub PAT with write:packages scope
	GitHubAppID            string `env:"GITHUB_APP_ID,required"`
	GitHubAppPrivateKey    string `env:"GITHUB_APP_PRIVATE_KEY,required"` // base64-encoded

	// S3/MinIO for shared image storage
	S3Endpoint  string `env:"S3_ENDPOINT,required"`
	S3Bucket    string `env:"S3_BUCKET,required"`
	S3AccessKey string `env:"S3_ACCESS_KEY,required"`
	S3SecretKey string `env:"S3_SECRET_KEY,required"`
	S3UseSSL    bool   `env:"S3_USE_SSL" envDefault:"false"`

	// Edge proxy config
	EdgeProxyHTTPAddr    string `env:"EDGEPROXY_HTTP_ADDR" envDefault:":80"`
	EdgeProxyHTTPSAddr   string `env:"EDGEPROXY_HTTPS_ADDR" envDefault:":443"`
	EdgeProxyACMEEmail   string `env:"EDGEPROXY_ACME_EMAIL" envDefault:"admin@zeitwork.com"`
	EdgeProxyACMEStaging bool   `env:"EDGEPROXY_ACME_STAGING" envDefault:"false"`

	// Metadata server config (serves env vars to VMs via HTTP)
	MetadataServerAddr string `env:"METADATA_SERVER_ADDR" envDefault:"0.0.0.0:8111"`
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
		panic("failed to parse config: " + err.Error())
	}

	// Load or create stable server identity
	serverID, err := zeitwork.LoadOrCreateServerID()
	if err != nil {
		panic("failed to load server ID: " + err.Error())
	}

	// Initialize database (pooled connection via PgBouncer)
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		panic("failed to init database: " + err.Error())
	}

	// Initialize S3 client for shared image storage
	s3Client, err := storage.NewS3(storage.S3Config{
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		UseSSL:    cfg.S3UseSSL,
	})
	if err != nil {
		panic("failed to init S3 client: " + err.Error())
	}

	// Route change notification channel (shared between zeitwork service and edge proxy)
	routeChangeNotify := make(chan struct{}, 1)

	service, err := zeitwork.New(zeitwork.Config{
		DB:                     db,
		IPAdress:               cfg.IPAdress,
		DatabaseDirectURL:      cfg.DatabaseDirectURL,
		InternalIP:             cfg.InternalIP,
		ServerID:               serverID,
		S3:                     s3Client,
		RouteChangeNotify:      routeChangeNotify,
		DockerRegistryURL:      cfg.DockerRegistryURL,
		DockerRegistryUsername: cfg.DockerRegistryUsername,
		DockerRegistryPAT:      cfg.DockerRegistryPAT,
		GitHubAppID:            cfg.GitHubAppID,
		GitHubAppPrivateKey:    cfg.GitHubAppPrivateKey,
		MetadataServerAddr:     cfg.MetadataServerAddr,
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

	// Edge proxy (shares DB connection and route change channel)
	edgeProxy, err := edgeproxy.NewService(edgeproxy.Config{
		HTTPAddr:          cfg.EdgeProxyHTTPAddr,
		HTTPSAddr:         cfg.EdgeProxyHTTPSAddr,
		ACMEEmail:         cfg.EdgeProxyACMEEmail,
		ACMEStaging:       cfg.EdgeProxyACMEStaging,
		DB:                db,
		RouteChangeNotify: routeChangeNotify,
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
