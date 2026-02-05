package zeitwork

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

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
				// Update local state to continue with VM creation
				deployment.ImageID = build.ImageID
				deployment.Status = queries.DeploymentStatusStarting
			}
			// Fall through to VM creation below
		} else {
			return nil
		}
	}

	// ** Ensure we have a VM ** //
	if !deployment.VmID.Valid {
		// Guard: can't create VM without an image
		if !deployment.ImageID.Valid {
			slog.Debug("deployment has no image_id yet, waiting for build to complete", "deployment_id", deployment.ID)
			return nil
		}

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
				// Perform HTTP health check before marking as running
				if !s.checkDeploymentHealth(vm.IpAddress.Addr().String(), vm.Port.Int32) {
					slog.Info("deployment health check failed, will retry", "deployment_id", deployment.ID, "vm_id", deployment.VmID)
					return fmt.Errorf("health check failed, will retry")
				}

				err = s.db.DeploymentMarkRunning(ctx, deployment.ID)
				if err != nil {
					return err
				}
				slog.Info("marked deployment as running", "deployment_id", deployment.ID, "vm_id", deployment.VmID)

				// Point custom domains to this new deployment
				if err := s.pointCustomDomainsToDeployment(ctx, deployment); err != nil {
					slog.Error("failed to point custom domains to deployment", "deployment_id", deployment.ID, "error", err)
					// Don't return error - deployment is running, domain update can be retried
				}

				// Stop older deployments for this project now that the new one is healthy
				if err := s.stopOldDeployments(ctx, deployment); err != nil {
					slog.Error("failed to stop old deployments", "deployment_id", deployment.ID, "error", err)
					// Don't return error - the new deployment is running, stopping old ones can be retried
				}

				return nil
			}
		}
	}

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

// checkDeploymentHealth performs an HTTP health check on a deployment's VM
func (s *Service) checkDeploymentHealth(ip string, port int32) bool {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	healthURL := fmt.Sprintf("http://%s:%d/", ip, port)
	resp, err := client.Get(healthURL)
	if err != nil {
		slog.Debug("deployment health check failed", "url", healthURL, "error", err)
		return false
	}
	defer resp.Body.Close()

	// Consider 2xx and 3xx status codes as healthy
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	slog.Debug("deployment health check", "url", healthURL, "status", resp.StatusCode, "healthy", healthy)
	return healthy
}

// pointCustomDomainsToDeployment updates all custom domains for the project to point to this deployment
func (s *Service) pointCustomDomainsToDeployment(ctx context.Context, deployment queries.Deployment) error {
	err := s.db.DomainUpdateDeploymentForProject(ctx, queries.DomainUpdateDeploymentForProjectParams{
		DeploymentID: deployment.ID,
		ProjectID:    deployment.ProjectID,
	})
	if err != nil {
		return fmt.Errorf("failed to update domains: %w", err)
	}
	slog.Info("pointed custom domains to deployment", "deployment_id", deployment.ID, "project_id", deployment.ProjectID)
	return nil
}

// stopOldDeployments finds and stops all other running deployments for the same project
func (s *Service) stopOldDeployments(ctx context.Context, currentDeployment queries.Deployment) error {
	// Find all other running deployments for this project
	oldDeployments, err := s.db.DeploymentFindOtherRunningByProjectID(ctx, queries.DeploymentFindOtherRunningByProjectIDParams{
		ProjectID: currentDeployment.ProjectID,
		ID:        currentDeployment.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to find old deployments: %w", err)
	}

	if len(oldDeployments) == 0 {
		slog.Info("no old deployments to stop", "deployment_id", currentDeployment.ID)
		return nil
	}

	slog.Info("stopping old deployments", "deployment_id", currentDeployment.ID, "old_deployment_count", len(oldDeployments))

	for _, oldDep := range oldDeployments {
		// Soft delete the VM (this will trigger the VM reconciler to kill the process)
		if oldDep.VmID.Valid {
			if err := s.db.VMSoftDelete(ctx, oldDep.VmID); err != nil {
				slog.Error("failed to soft delete VM for old deployment", "deployment_id", oldDep.ID, "vm_id", oldDep.VmID, "error", err)
				continue
			}
			slog.Info("soft deleted VM for old deployment", "deployment_id", oldDep.ID, "vm_id", oldDep.VmID)
		}

		// Mark the deployment as stopped
		if err := s.db.DeploymentMarkStopped(ctx, oldDep.ID); err != nil {
			slog.Error("failed to mark old deployment as stopped", "deployment_id", oldDep.ID, "error", err)
			continue
		}
		slog.Info("marked old deployment as stopped", "deployment_id", oldDep.ID)
	}

	return nil
}
