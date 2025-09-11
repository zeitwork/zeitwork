-- name: InstancesFindByNode :many
-- Find instances by node ID for node agent
SELECT 
    id,
    region_id,
    node_id,
    image_id,
    state,
    vcpus,
    memory,
    default_port,
    ip_address,
    environment_variables,
    created_at,
    updated_at
FROM instances 
WHERE node_id = $1 
    AND deleted_at IS NULL;

-- name: InstancesUpdateState :one
-- Update instance state
UPDATE instances 
SET state = $2, 
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    region_id,
    node_id,
    image_id,
    state,
    vcpus,
    memory,
    default_port,
    ip_address,
    environment_variables,
    created_at,
    updated_at;

-- name: InstancesUpdateIpAddress :one
-- Update instance IP address after container creation
UPDATE instances 
SET ip_address = $2, 
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    region_id,
    node_id,
    image_id,
    state,
    vcpus,
    memory,
    default_port,
    ip_address,
    environment_variables,
    created_at,
    updated_at;

-- name: InstancesGetById :one
-- Get instance by ID
SELECT 
    id,
    region_id,
    node_id,
    image_id,
    state,
    vcpus,
    memory,
    default_port,
    ip_address,
    environment_variables,
    created_at,
    updated_at
FROM instances 
WHERE id = $1 
    AND deleted_at IS NULL;

-- name: InstancesCreate :one
-- Create a new instance
INSERT INTO instances (
    id,
    region_id,
    node_id,
    image_id,
    state,
    vcpus,
    memory,
    default_port,
    ip_address,
    environment_variables
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10
)
RETURNING 
    id,
    region_id,
    node_id,
    image_id,
    state,
    vcpus,
    memory,
    default_port,
    ip_address,
    environment_variables,
    created_at,
    updated_at;

-- name: InstancesGetByDeployment :many
-- Get instances for a deployment
SELECT 
    i.id,
    i.region_id,
    i.node_id,
    i.image_id,
    i.state,
    i.vcpus,
    i.memory,
    i.default_port,
    i.ip_address,
    i.environment_variables,
    i.created_at,
    i.updated_at
FROM instances i
JOIN deployment_instances di ON di.instance_id = i.id
WHERE di.deployment_id = $1 
    AND i.deleted_at IS NULL;

-- name: InstancesDelete :exec
-- Soft delete an instance
UPDATE instances 
SET deleted_at = now(), 
    updated_at = now()
WHERE id = $1;

-- name: InstancesCheckIpInUse :one
-- Check if an IP address is already in use by any non-deleted instance
SELECT EXISTS(
    SELECT 1 
    FROM instances 
    WHERE ip_address = $1 
        AND deleted_at IS NULL
) as in_use;
