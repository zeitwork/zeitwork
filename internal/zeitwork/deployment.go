package zeitwork

import (
	"context"

	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileDeployment(ctx context.Context, objectID uuid.UUID) error {
	deployment, err := s.db.Queries.DeploymentFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	// IF THERE IS NO BUILD THEN CREATE A BUILD
	if !deployment.BuildID.Valid {
		build, err := s.db.BuildCreate(ctx, queries.BuildCreateParams{
			ID:             uuid.New(),
			Status:         queries.BuildStatusPending,
			ProjectID:      deployment.ProjectID,
			GithubCommit:   deployment.GithubCommit,
			GithubBranch:   "main", // TODO later we will have environments where you can define the branch
			OrganisationID: deployment.OrganisationID,
		})
		if err != nil {
			return err
		}
		s.db.DeploymentUpdateMarkBuilding(ctx, queries.DeploymentUpdateMarkBuildingParams{
			ID:      deployment.ID,
			BuildID: build.ID,
		})
		return nil
	}

	switch deployment.Status {
	case queries.DeploymentStatusPending:
		// TODO: create build with `pending` status AND deployment => `building`

		panic("unimplemented")
	case queries.DeploymentStatusBuilding:
		panic("unimplemented")
		// -> if build status `pending` or `building` for more than 10 minutes then set deployment status to `failed`
		// -> if build status `failed` then mark deployment `failed`
		// -> if build status `succesful` then create vm with `pending` status and update deployment to `starting`
	case queries.DeploymentStatusStarting:
		panic("unimplemented")
		// -> if vm status `pending` or `starting` for more than 10 minutes set deployment status to `failed`
	case queries.DeploymentStatusRunning:
		panic("unimplemented")
		// -> if there is a newer deployment with status `running` then mark this one as `stopping`
	case queries.DeploymentStatusStopping:
		panic("unimplemented")
	case queries.DeploymentStatusStopped:
		panic("unimplemented")
		// -> if vm status is `running` then mark it as stopping
		// -> if vm status is `stopped` then mark the deployment as `stopped`
	case queries.DeploymentStatusFailed:
		panic("unimplemented")
	}

	return nil
}
