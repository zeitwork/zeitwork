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
	"github.com/zeitwork/zeitwork/internal/reconciler"
)

type Config struct {
	DatabaseURL string `env:"DATABASE_URL,required"`
}

type Service struct {
	db database.DB
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

	reconcilerd, err := reconciler.New(reconciler.Config{
		IPAdress:               "1.1.1.1",
		DB:                     db,
		DockerRegistryURL:      "",
		DockerRegistryUsername: "",
		DockerRegistryPassword: "",
	}, logger)
	reconcilerd.Start(ctx)

	// TODO: edge proxy

	// TODO: builder

	// TODO: vm manager
}

// func New(cfg Config) Service {
// 	db, err := database.New(cfg.DatabaseURL)
// 	if err != nil {
// 		panic("no db")
// 	}
// 	return Service{
// 		db: *db,
// 	}
// }

// func (s Service) createBuildForPendingDeployment() {
// 	deployment, err := s.db.Queries().DeploymentFirstPending(context.Background())
// 	if err != nil {
// 		slog.Info("no pending deployments")
// 		return
// 	}
// 	logger := slog.With("id", deployment.ID.String())

// 	logger.Info("found pending deployment")

// 	err = s.db.WithTx(context.Background(), func(q *queries.Queries) error {
// 		build, err := q.BuildCreate(context.Background(), queries.BuildCreateParams{
// 			ID:             uuid.New(),
// 			Status:         queries.BuildStatusPending,
// 			ProjectID:      deployment.ProjectID,
// 			GithubCommit:   deployment.GithubCommit,
// 			GithubBranch:   "main",
// 			OrganisationID: deployment.OrganisationID,
// 		})
// 		if err != nil {
// 			return err
// 		}

// 		_, err = q.DeploymentUpdateMarkBuilding(context.Background(), queries.DeploymentUpdateMarkBuildingParams{
// 			ID:      deployment.ID,
// 			BuildID: build.ID,
// 		})
// 		return err
// 	})
// 	if err != nil {
// 		logger.Error("marking deployment as building failed", "err", err)
// 	}
// 	logger.Info("deployment marked as deploying")

// }

// func (s Service) buildPendingBuild() {
// 	build, err := s.db.Queries().BuildFirstPending(context.Background())
// 	if err != nil {
// 		slog.Info("no pending build")
// 		return
// 	}

// 	time.Sleep(5 * time.Second)

// 	_, err = s.db.Queries().BuildUpdateMarkBuilding(context.Background(), build.ID)

// 	slog.Info("pending build", "id", build.ID.String())
// }
