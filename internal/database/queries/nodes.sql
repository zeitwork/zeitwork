-- name: NodesGetById :one
-- Get node by ID
SELECT 
    id,
    region_id,
    hostname,
    ip_address,
    state,
    resources,
    created_at,
    updated_at
FROM nodes 
WHERE id = $1 
    AND deleted_at IS NULL;

-- name: NodesGetByHostname :one
-- Get node by hostname
SELECT 
    id,
    region_id,
    hostname,
    ip_address,
    state,
    resources,
    created_at,
    updated_at
FROM nodes 
WHERE hostname = $1 
    AND deleted_at IS NULL;

-- name: NodesGetByRegion :many
-- Get nodes by region
SELECT 
    id,
    region_id,
    hostname,
    ip_address,
    state,
    resources,
    created_at,
    updated_at
FROM nodes 
WHERE region_id = $1 
    AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: NodesCreate :one
-- Create a new node
INSERT INTO nodes (
    id,
    region_id,
    hostname,
    ip_address,
    state,
    resources
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6
)
RETURNING 
    id,
    region_id,
    hostname,
    ip_address,
    state,
    resources,
    created_at,
    updated_at;

-- name: NodesUpdateState :one
-- Update node state
UPDATE nodes 
SET state = $2, 
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    region_id,
    hostname,
    ip_address,
    state,
    resources,
    created_at,
    updated_at;

-- name: NodesGetAll :many
-- Get all nodes
SELECT 
    id,
    region_id,
    hostname,
    ip_address,
    state,
    resources,
    created_at,
    updated_at
FROM nodes 
WHERE deleted_at IS NULL
ORDER BY created_at DESC;
