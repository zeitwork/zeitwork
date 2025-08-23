-- name: NodeFindById :one
SELECT * FROM nodes WHERE id = $1;

-- name: NodeFindByState :many
SELECT * FROM nodes WHERE state = $1;

-- name: NodeFindByHostname :one
SELECT * FROM nodes WHERE hostname = $1;

-- name: NodeFind :many
SELECT * FROM nodes ORDER BY created_at DESC;

-- name: NodeFindByRegion :many
SELECT * FROM nodes WHERE region_id = $1 ORDER BY created_at DESC;

-- name: NodeCreate :one
INSERT INTO nodes (
    region_id, hostname, ip_address, state, resources
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: NodeUpdateState :one
UPDATE nodes SET state = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: NodeUpdateResources :one
UPDATE nodes SET resources = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: NodeDelete :exec
DELETE FROM nodes WHERE id = $1;
