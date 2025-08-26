-- name: InstanceFindById :one
SELECT * FROM instances WHERE id = $1;

-- name: InstanceFindByNode :many
SELECT * FROM instances WHERE node_id = $1;

-- name: InstanceFindByState :many
SELECT * FROM instances WHERE state = $1;

-- name: InstanceFindByImage :many
SELECT * FROM instances WHERE image_id = $1;

-- name: InstanceFind :many
SELECT * FROM instances ORDER BY created_at DESC;

-- name: InstanceFindByRegion :many
SELECT * FROM instances WHERE region_id = $1 ORDER BY created_at DESC;

-- name: InstanceCreate :one
INSERT INTO instances (
    region_id, node_id, image_id, state, resources,
    default_port, ip_address, environment_variables
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: InstanceUpdateState :one
UPDATE instances SET state = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: InstanceUpdateNode :one
UPDATE instances SET node_id = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: InstanceDelete :exec
DELETE FROM instances WHERE id = $1;

-- name: InstanceFindByDeployment :many
SELECT i.* FROM instances i
JOIN deployment_instances di ON i.id = di.instance_id
WHERE di.deployment_id = $1 AND i.state = 'running';
