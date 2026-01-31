package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/caarlos0/env/v11"
	_ "github.com/joho/godotenv/autoload"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type Config struct {
	DatabaseURL string `env:"DATABASE_URL,required"`
}

type Service struct {
	db database.DB
}

func main() {
	firstRun := true

	// Parse configuration from environment variables
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		panic("failed to parse config")
	}
	svc := New(cfg)

	// reconciler loop
	for {
		if !firstRun {
			time.Sleep(1 * time.Second)
		}
		firstRun = false

		// ** deployments **
		// deployment status = `pending`
		// -> create build with `pending` status AND deployment => `building`
		// deployment status = `building`
		// -> if build status `pending` or `building` for more than 10 minutes then set deployment status to `failed`
		// -> if build status `failed` then mark deployment `failed`
		// -> if build status `succesful` then create vm with `pending` status and update deployment to `starting`
		// deployment status = `starting`
		// -> if vm status `pending` or `starting` for more than 10 minutes set deployment status to `failed`
		// deployment status = `running`
		// -> if there is a newer deployment with status `running` then mark this one as `stopping`
		// deployment status = `stopping`
		// -> if vm status is `running` then mark it as stopping
		// -> if vm status is `stopped` then mark the deployment as `stopped`

		// ** builds **
		// build status = `pending`
		// -> create a `vm` with status `pending` with the `zeitwork-build` image and update build to status `building`
		// build status = `building
		// -> if build status is `building` for more than 30 minutes mark it as failed
		// -> if vm status `pending`, `starting`, `running` or `stopping` for more than 10 minutes then set build status to `failed`
		// -> if vm status `failed` then set build status to `failed`
		// -> if vm status `stopped` then check build image
		// |-> if it exists then mark build as `successful`
		// |-> if it does not exist then mark build as `failed`

		// ** vms **
		// vm status = `pending`
		// -> mark vm as starting
		// vm status = `starting`
		// -> start the vm with cloud hypervisor and either mark it as `running` or `failed`
		// vm status = `running`
		// -> check status and ensure the vm is running
		// vm status = `stopping`
		// -> stop the vm
		// vm status = `stopped`
		// -> nothing
		// vm status = `failed`
		// -> nothing

		svc.createBuildForPendingDeployment()
		svc.buildPendingBuild()
	}

	// edge proxy

	// builder

	// vm manager
}

func New(cfg Config) Service {
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		panic("no db")
	}
	return Service{
		db: *db,
	}
}

func (s Service) createBuildForPendingDeployment() {
	deployment, err := s.db.Queries.DeploymentFirstPending(context.Background())
	if err != nil {
		slog.Info("no pending deployments")
		return
	}
	logger := slog.With("id", deployment.ID.String())

	logger.Info("found pending deployment")

	err = s.db.WithTx(context.Background(), func(q *queries.Queries) error {
		build, err := q.BuildCreate(context.Background(), queries.BuildCreateParams{
			ID:             uuid.New(),
			Status:         queries.BuildStatusPending,
			ProjectID:      deployment.ProjectID,
			GithubCommit:   deployment.GithubCommit,
			GithubBranch:   "main",
			OrganisationID: deployment.OrganisationID,
		})
		if err != nil {
			return err
		}

		_, err = q.DeploymentUpdateMarkBuilding(context.Background(), queries.DeploymentUpdateMarkBuildingParams{
			ID:      deployment.ID,
			BuildID: build.ID,
		})
		return err
	})
	if err != nil {
		logger.Error("marking deployment as building failed", "err", err)
	}
	logger.Info("deployment marked as deploying")

}

func (s Service) buildPendingBuild() {
	build, err := s.db.Queries.BuildFirstPending(context.Background())
	if err != nil {
		slog.Info("no pending build")
		return
	}

	time.Sleep(5 * time.Second)

	_, err = s.db.Queries.BuildUpdateMarkBuilding(context.Background(), build.ID)

	slog.Info("pending build", "id", build.ID.String())
}
