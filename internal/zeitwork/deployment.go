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
	if !s.isControlPlaneLeader() {
		return nil
	}

	logger := slog.With("deployment_id", objectID)
	logger.Info("reconciling deployment")

	deployment, err := s.db.Queries.DeploymentFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	if deployment.DeletedAt.Valid || deployment.FailedAt.Valid || deployment.StoppedAt.Valid {
		logger.Info("deployment in terminal state, skipping", "status")

		// Ensure if the deployment is in a terminal state, the VM is also deleted
		if deployment.VmID.Valid {
			err = s.db.VMSoftDelete(ctx, deployment.VmID)
			if err != nil {
				return err
			}
			logger.Info("deleted VM for terminal deployment", "deployment_id", deployment.ID, "vm_id", deployment.VmID)
		}

		return nil
	}

	// Deployments should have a build
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
		deployment, err = s.db.DeploymentUpdateBuild(ctx, queries.DeploymentUpdateBuildParams{
			ID:      deployment.ID,
			BuildID: build.ID,
		})
		if err != nil {
			return err
		}
	}

	build, err := s.db.BuildFirstByID(ctx, deployment.BuildID)
	if err != nil {
		return err
	}

	// If build failed, mark the deployment as failed
	if build.FailedAt.Valid {
		err = s.db.DeploymentUpdateFailedAt(ctx, deployment.ID)
		if err != nil {
			return err
		}
		slog.Info("marked deployment as failed", "deployment_id", deployment.ID, "build_id", build.ID)
		return nil
	}

	if !build.ImageID.Valid {
		slog.Debug("build has no image_id yet, waiting for build to complete", "deployment_id", deployment.ID, "build_id", build.ID)
		s.deploymentScheduler.Schedule(deployment.ID, time.Now().Add(10*time.Second))
		return nil
	}

	// If the deployment's image_id does not match the build's image_id, update the deployment
	if deployment.ImageID != build.ImageID {
		deployment, err = s.db.DeploymentUpdateImage(ctx, queries.DeploymentUpdateImageParams{
			ID:      deployment.ID,
			ImageID: build.ImageID,
		})
		if err != nil {
			return err
		}
		slog.Info("updated deployment with new image", "deployment_id", deployment.ID, "image_id", build.ImageID, "build_id", build.ID)
	}

	// If the deployment does not have a VM, create one
	if !deployment.VmID.Valid {
		// Fetch and prepare environment variables for the VM
		encryptedEnvVars, err := s.prepareEnvVariablesForVM(ctx, deployment.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to prepare environment variables: %w", err)
		}

		// Create a VM for this deployment
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
		deployment, err = s.db.DeploymentUpdateVM(ctx, queries.DeploymentUpdateVMParams{
			ID:   deployment.ID,
			VmID: vm.ID,
		})
		if err != nil {
			return err
		}
		slog.Info("linked vm to deployment", "deployment_id", deployment.ID, "vm_id", vm.ID)
	}

	// If the deployment has a VM, check if it is healthy
	vm, err := s.db.VMFirstByID(ctx, deployment.VmID)
	if err != nil {
		return err
	}

	// If the VM is deleted, reset the VM of the deployment
	if vm.DeletedAt.Valid {
		deployment, err = s.db.DeploymentUpdateVM(ctx, queries.DeploymentUpdateVMParams{
			ID:   deployment.ID,
			VmID: uuid.Nil(),
		})
		if err != nil {
			return err
		}
		slog.Info("reset VM for deleted deployment", "deployment_id", deployment.ID, "vm_id", deployment.VmID)
		s.deploymentScheduler.Schedule(deployment.ID, time.Now())
		return nil
	}

	// Perform HTTP health check before marking as running
	healthy := s.checkDeploymentHealth(vm.IpAddress.Addr().String(), vm.Port.Int32)
	if !healthy {
		slog.Info("deployment health check failed, will retry", "deployment_id", deployment.ID, "vm_id", deployment.VmID)
		return fmt.Errorf("health check failed, will retry")
	}

	// Mark the deployment as running
	err = s.db.DeploymentMarkRunning(ctx, deployment.ID)
	if err != nil {
		return fmt.Errorf("failed to mark deployment as running: %w", err)
	}
	slog.Info("marked deployment as running", "deployment_id", deployment.ID)

	// Point custom domains to this new deployment
	err = s.pointCustomDomainsToDeployment(ctx, deployment)
	if err != nil {
		return fmt.Errorf("failed to point custom domains to deployment: %w", err)
	}

	// Stop older deployments for this project now that the new one is healthy
	err = s.stopOldDeployments(ctx, deployment)
	if err != nil {
		return fmt.Errorf("failed to stop old deployments: %w", err)
	}

	return nil
}

// prepareEnvVariablesForVM fetches environment variables for a project,
// decrypts them, formats as "KEY=value" strings, and re-encrypts as JSON.
// Returns an encrypted JSON array string suitable for storing in vms.env_variables.
func (s *Service) prepareEnvVariablesForVM(ctx context.Context, projectID uuid.UUID) (string, error) {
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
		Timeout: 10 * time.Second,
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
	return nil
}

// stopOldDeployments finds and stops all other running deployments for the same project
func (s *Service) stopOldDeployments(ctx context.Context, currentDeployment queries.Deployment) error {
	oldDeployments, err := s.db.DeploymentFindRunningAndOlder(ctx, queries.DeploymentFindRunningAndOlderParams{
		ProjectID: currentDeployment.ProjectID,
		ID:        currentDeployment.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to find old deployments: %w", err)
	}

	// If there are no old deployments, return
	if len(oldDeployments) == 0 {
		return nil
	}

	slog.Info("stopping old deployments", "deployment_id", currentDeployment.ID, "old_deployment_count", len(oldDeployments))
	for _, oldDep := range oldDeployments {
		// We deleted the VM, the reconciler will stop the process
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
