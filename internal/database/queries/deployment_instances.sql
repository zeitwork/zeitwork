-- name: DeploymentInstancesCreate :one
-- Create a new deployment instance relationship
INSERT INTO deployment_instances (
    id,
    deployment_id,
    instance_id
) VALUES (
    $1,
    $2,
    $3
)
RETURNING 
    id,
    deployment_id,
    instance_id,
    created_at,
    updated_at;

-- name: DeploymentInstancesGetByDeployment :many
-- Get deployment instances by deployment ID
SELECT 
    id,
    deployment_id,
    instance_id,
    created_at,
    updated_at
FROM deployment_instances 
WHERE deployment_id = $1 
    AND deleted_at IS NULL;

-- name: DeploymentInstancesGetByInstance :one
-- Get deployment instance by instance ID
SELECT 
    id,
    deployment_id,
    instance_id,
    created_at,
    updated_at
FROM deployment_instances 
WHERE instance_id = $1 
    AND deleted_at IS NULL;

-- name: DeploymentInstancesDelete :exec
-- Soft delete a deployment instance
UPDATE deployment_instances 
SET deleted_at = now(), 
    updated_at = now()
WHERE id = $1;
