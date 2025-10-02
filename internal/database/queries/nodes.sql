-- name: GetNodeByID :one
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
WHERE id = $1;

-- name: UpsertNode :exec
INSERT INTO nodes (
    id,
    region_id,
    hostname,
    ip_address,
    state,
    resources,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, NOW(), NOW()
)
ON CONFLICT (id) 
DO UPDATE SET
    hostname = EXCLUDED.hostname,
    ip_address = EXCLUDED.ip_address,
    state = EXCLUDED.state,
    resources = EXCLUDED.resources,
    updated_at = NOW();

-- name: UpdateNodeState :exec
UPDATE nodes
SET state = $2, updated_at = NOW()
WHERE id = $1;

