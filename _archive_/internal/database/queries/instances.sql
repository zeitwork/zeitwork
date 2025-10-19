-- name: GetInstancesByNodeID :many
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
    i.updated_at,
    img.name as image_name,
    img.hash as image_hash
FROM instances i
INNER JOIN images img ON i.image_id = img.id
WHERE i.node_id = $1
ORDER BY i.created_at DESC;

-- name: UpdateInstanceState :exec
UPDATE instances
SET state = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateInstanceIPAddress :exec
UPDATE instances
SET ip_address = $2, updated_at = NOW()
WHERE id = $1;

-- name: GetInstanceByID :one
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
    i.updated_at,
    img.name as image_name,
    img.hash as image_hash
FROM instances i
INNER JOIN images img ON i.image_id = img.id
WHERE i.id = $1;

