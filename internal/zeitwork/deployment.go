package zeitwork

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/crypto"
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

		// if build failed, mark deployment as failed (only if not already failed)
		if build.Status == queries.BuildStatusFailed {
			slog.Info("build failed, marking deployment as failed", "deployment_id", deployment.ID, "build_id", build.ID)
			if deployment.Status != queries.DeploymentStatusFailed {
				return s.db.DeploymentMarkFailed(ctx, deployment.ID)
			}
			return nil
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
				return nil // Let next reconcile handle VM creation with fresh deployment data
			}
			// Only fall through to VM creation if we're already in 'starting' status
		} else {
			return nil
		}
	}

	// ** Ensure we have a VM ** //
	if !deployment.VmID.Valid {
		slog.Info("creating vm for deployment", "deployment_id", deployment.ID, "image_id", deployment.ImageID)

		// Fetch and prepare environment variables for the VM
		encryptedEnvVars, err := s.prepareEnvVariablesForVM(ctx, deployment.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to prepare environment variables: %w", err)
		}

		// create a build vm for this deployment
		vm, err := s.VMCreate(ctx, VMCreateParams{
			VCPUs:        1,
			Memory:       2 * 1024,
			ImageID:      deployment.ImageID,
			Port:         3000,
			EnvVariables: encryptedEnvVars,
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

// prepareEnvVariablesForVM fetches environment variables for a project,
// decrypts them, formats as "KEY=value" strings, and re-encrypts as JSON.
// Returns an encrypted JSON array string suitable for storing in vms.env_variables.
func (s *Service) prepareEnvVariablesForVM(ctx context.Context, projectID uuid.UUID) (string, error) {
	// Fetch environment variables for the project
	envVars, err := s.db.EnvironmentVariableFindByProjectID(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch environment variables: %w", err)
	}

	// Decrypt each value and format as "KEY=value"
	envStrings := make([]string, 0, len(envVars))
	for _, ev := range envVars {
		decryptedValue, err := crypto.Decrypt(ev.Value)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt environment variable %s: %w", ev.Name, err)
		}
		envStrings = append(envStrings, fmt.Sprintf("%s=%s", ev.Name, decryptedValue))
	}

	// Marshal to JSON
	envJSON, err := json.Marshal(envStrings)
	if err != nil {
		return "", fmt.Errorf("failed to marshal environment variables: %w", err)
	}

	// Re-encrypt the JSON array
	encryptedEnvVars, err := crypto.Encrypt(string(envJSON))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt environment variables: %w", err)
	}

	slog.Info("prepared environment variables for VM", "projectID", projectID, "count", len(envStrings))
	return encryptedEnvVars, nil
}
