-- name: DeploymentFindById :one
SELECT * FROM deployments WHERE id = $1;

-- name: DeploymentFindByProject :many
SELECT * FROM deployments WHERE project_id = $1 ORDER BY created_at DESC;

-- name: DeploymentFindByOrganisation :many
SELECT * FROM deployments WHERE organisation_id = $1 ORDER BY created_at DESC;

-- name: DeploymentFindByEnvironment :many
SELECT * FROM deployments WHERE project_environment_id = $1 ORDER BY created_at DESC;

-- name: DeploymentFindByStatus :many
SELECT * FROM deployments WHERE status = $1 ORDER BY created_at DESC;

-- name: DeploymentFindByImage :many
SELECT * FROM deployments WHERE image_id = $1 ORDER BY created_at DESC;

-- name: DeploymentFind :many
SELECT * FROM deployments ORDER BY created_at DESC;

-- name: DeploymentCreate :one
INSERT INTO deployments (
    project_id, project_environment_id, status, commit_hash, image_id, organisation_id,
    deployment_url, nanoid, min_instances, rollout_strategy
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: DeploymentUpdateStatus :one
UPDATE deployments 
SET status = $2, activated_at = $3, updated_at = NOW() 
WHERE id = $1 
RETURNING *;

-- name: DeploymentUpdateImage :one
UPDATE deployments SET image_id = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: DeploymentDelete :exec
DELETE FROM deployments WHERE id = $1;

-- name: DeploymentInstanceFindById :one
SELECT * FROM deployment_instances WHERE id = $1;

-- name: DeploymentInstanceFindByDeployment :many
SELECT * FROM deployment_instances WHERE deployment_id = $1 ORDER BY created_at DESC;

-- name: DeploymentInstanceFindByInstance :many
SELECT * FROM deployment_instances WHERE instance_id = $1 ORDER BY created_at DESC;

-- name: DeploymentInstanceCreate :one
INSERT INTO deployment_instances (deployment_id, instance_id, organisation_id) VALUES ($1, $2, $3) RETURNING *;

-- name: DeploymentInstanceDelete :exec
DELETE FROM deployment_instances WHERE id = $1;
