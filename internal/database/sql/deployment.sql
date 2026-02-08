-- name: DeploymentFind :many
SELECT *
FROM deployments;

-- name: DeploymentFirstByID :one
SELECT *
FROM deployments
WHERE id = $1
LIMIT 1;

-- name: DeploymentFirstPending :one
SELECT *
FROM deployments WHERE status = 'pending'
ORDER BY id DESC
LIMIT 1;

-- name: DeploymentMarkBuilding :one
UPDATE deployments
SET build_id = $2, status = 'building', building_at = now()
WHERE id = $1
RETURNING *;

-- name: DeploymentMarkStarting :exec
UPDATE deployments
SET status = 'starting', starting_at = now(), image_id = $2
WHERE id = $1;

-- name: DeploymentUpdateVMID :exec
UPDATE deployments
SET vm_id = $2
WHERE id = $1;

-- name: DeploymentMarkRunning :exec
UPDATE deployments
SET status = 'running', running_at = now()
WHERE id = $1;

-- name: DeploymentMarkFailed :exec
UPDATE deployments
SET status = 'failed', failed_at = now()
WHERE id = $1;

-- name: DeploymentFindByBuildID :many
SELECT * FROM deployments WHERE build_id = $1;

-- name: DeploymentFindByVMID :one
SELECT * FROM deployments WHERE vm_id = $1 LIMIT 1;

-- name: DeploymentFindOtherRunningByProjectID :many
-- Find all running deployments for a project, excluding a specific deployment
SELECT * FROM deployments
WHERE project_id = $1
  AND id != $2
  AND status = 'running'
  AND deleted_at IS NULL;

-- name: DeploymentMarkStopped :exec
UPDATE deployments
SET status = 'stopped', stopped_at = now()
WHERE id = $1;

-- name: VMLogCreate :exec
INSERT INTO vm_logs (id, vm_id, message, level, created_at)
VALUES ($1, $2, $3, $4, NOW());
