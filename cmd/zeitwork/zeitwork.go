package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"
	_ "github.com/joho/godotenv/autoload"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/zeitwork"
)

type Config struct {
	DatabaseURL string `env:"DATABASE_URL,required"`
}

func main() {
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the service in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse configuration from environment variables
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		panic("failed to parse config")
	}

	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		panic("no db")
	}

	logger := slog.Default()

	service, err := zeitwork.New(zeitwork.Config{
		IPAdress:               "1.1.1.1", // TODO
		DB:                     db,
		DockerRegistryURL:      "",
		DockerRegistryUsername: "",
		DockerRegistryPassword: "",
	}, logger)
	if err != nil {
		panic(err)
	}

	go func() {
		service.Start(ctx)
	}()

	// TODO: edge proxy

	// TODO: builder

	// TODO: vm manager

	sig := <-sigChan
	logger.Info("shutdown signal", "signal", sig)

	service.Stop()

}
