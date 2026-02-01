package zeitwork

import (
	"context"
	"log/slog"

	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileDeployment(ctx context.Context, objectID uuid.UUID) error {
	deployment, err := s.db.Queries.DeploymentFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	slog.Info("reconciling deployment", "deployment_id", deployment.ID, "status", deployment.Status)

	// Skip if deployment is already in a terminal state
	if deployment.Status == queries.DeploymentStatusFailed || deployment.Status == queries.DeploymentStatusStopped {
		slog.Info("deployment in terminal state, skipping", "deployment_id", deployment.ID, "status", deployment.Status)
		return nil
	}

	// ** Deployments should have a build ** //
	if !deployment.BuildID.Valid {
		build, err := s.db.BuildCreate(ctx, queries.BuildCreateParams{
			ID:             uuid.New(),
			Status:         queries.BuildStatusPending,
			ProjectID:      deployment.ProjectID,
			GithubCommit:   deployment.GithubCommit,
			GithubBranch:   "main", // TODO: later we will have environments where you can define the branch
			OrganisationID: deployment.OrganisationID,
		})
		if err != nil {
			return err
		}
		_, err = s.db.DeploymentMarkBuilding(ctx, queries.DeploymentMarkBuildingParams{
			ID:      deployment.ID,
			BuildID: build.ID,
		})
		if err != nil {
			return err
		}
		slog.Info("marked deployment as building", "deployment_id", deployment.ID, "build_id", build.ID)
		return nil
	}

	// ** Ensure build has completed ** //
	if deployment.BuildID.Valid {
		build, err := s.db.BuildFirstByID(ctx, deployment.BuildID)
		if err != nil {
			return err
		}

		// if build failed, mark deployment as failed
		if build.Status == queries.BuildStatusFailed {
			slog.Info("build failed, marking deployment as failed", "deployment_id", deployment.ID, "build_id", build.ID)
			return s.db.DeploymentMarkFailed(ctx, deployment.ID)
		}

		// if build has an image then advance to starting (if not already)
		if build.ImageID.Valid {
			if deployment.Status == queries.DeploymentStatusBuilding {
				err = s.db.DeploymentMarkStarting(ctx, queries.DeploymentMarkStartingParams{
					ID:      deployment.ID,
					ImageID: build.ImageID,
				})
				if err != nil {
					return err
				}
				slog.Info("marked deployment as starting", "deployment_id", deployment.ID, "image_id", build.ImageID)
			}
			// Fall through to VM creation
		} else {
			return nil
		}
	}

	// ** Ensure we have a VM ** //
	if !deployment.VmID.Valid {
		slog.Info("creating vm for deployment", "deployment_id", deployment.ID, "image_id", deployment.ImageID)
		// create a build vm for this deployment
		vm, err := s.VMCreate(ctx, VMCreateParams{
			VCPUs:   1,
			Memory:  2 * 1024,
			ImageID: deployment.ImageID,
			Port:    3000,
		})
		if err != nil {
			return err
		}
		err = s.db.DeploymentUpdateVMID(ctx, queries.DeploymentUpdateVMIDParams{
			ID:   deployment.ID,
			VmID: vm.ID,
		})
		if err != nil {
			return err
		}
		slog.Info("linked vm to deployment", "deployment_id", deployment.ID, "vm_id", vm.ID)
		return nil
	}

	if deployment.VmID.Valid {
		// if deployment has a vm check if that vm is healthy (status = running)
		vm, err := s.db.VMFirstByID(ctx, deployment.VmID)
		if err != nil {
			return err
		}

		if vm.Status == queries.VmStatusRunning {
			// mark the deployment as running if it isn't already
			if deployment.Status != queries.DeploymentStatusRunning {
				err = s.db.DeploymentMarkRunning(ctx, deployment.ID)
				if err != nil {
					return err
				}
				slog.Info("marked deployment as running", "deployment_id", deployment.ID, "vm_id", deployment.VmID)
				return nil
			}
		}
	}

	// TODO: stopping
	// TODO: stopping

	// switch deployment.Status {
	// case queries.DeploymentStatusPending:
	// 	// TODO: create build with `pending` status AND deployment => `building`

	// 	panic("unimplemented")
	// case queries.DeploymentStatusBuilding:
	// 	panic("unimplemented")
	// 	// -> if build status `pending` or `building` for more than 10 minutes then set deployment status to `failed`
	// 	// -> if build status `failed` then mark deployment `failed`
	// 	// -> if build status `succesful` then create vm with `pending` status and update deployment to `starting`
	// case queries.DeploymentStatusStarting:
	// 	panic("unimplemented")
	// 	// -> if vm status `pending` or `starting` for more than 10 minutes set deployment status to `failed`
	// case queries.DeploymentStatusRunning:
	// 	panic("unimplemented")
	// 	// -> if there is a newer deployment with status `running` then mark this one as `stopping`
	// case queries.DeploymentStatusStopping:
	// 	panic("unimplemented")
	// case queries.DeploymentStatusStopped:
	// 	panic("unimplemented")
	// 	// -> if vm status is `running` then mark it as stopping
	// 	// -> if vm status is `stopped` then mark the deployment as `stopped`
	// case queries.DeploymentStatusFailed:
	// 	panic("unimplemented")
	// }

	return nil
}
