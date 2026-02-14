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

-- name: DeploymentUpdateBuild :one
UPDATE deployments
SET build_id = $2, building_at = COALESCE(building_at, now()), updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeploymentUpdateImage :one
UPDATE deployments
SET image_id = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeploymentUpdateVM :one
UPDATE deployments
SET vm_id = $2, starting_at = COALESCE(starting_at, now()), updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeploymentMarkRunning :exec
UPDATE deployments
SET running_at = COALESCE(running_at, now()), updated_at = now()
WHERE id = $1;

-- name: DeploymentUpdateFailedAt :exec
UPDATE deployments
SET failed_at = COALESCE(failed_at, now()), updated_at = now()
WHERE id = $1;

-- name: DeploymentFindByBuildID :many
SELECT * FROM deployments WHERE build_id = $1;

-- name: DeploymentFindByVMID :one
SELECT * FROM deployments WHERE vm_id = $1 LIMIT 1;

-- name: DeploymentFindRunningAndOlder :many
-- Find all running deployments for a project, older than the specified deployment
SELECT * FROM deployments
WHERE project_id = $1
  AND id < $2
  AND running_at IS NOT NULL
  AND deleted_at IS NULL;


-- name: DeploymentMarkStopped :exec
UPDATE deployments
SET stopped_at = COALESCE(stopped_at, now()), updated_at = now()
WHERE id = $1;

-- name: VMLogCreate :exec
INSERT INTO vm_logs (id, vm_id, message, level, created_at)
VALUES ($1, $2, $3, $4, NOW());

-- name: DeploymentFindRunningByServerID :many
-- Find all running deployments whose VM is on a specific server.
SELECT d.* FROM deployments d
INNER JOIN vms v ON d.vm_id = v.id
WHERE v.server_id = $1
  AND d.status = 'running'
  AND d.deleted_at IS NULL;

-- name: DeploymentFindNewest :one
SELECT * 
FROM deployments 
WHERE project_id = $1 
ORDER BY id DESC 
LIMIT 1;