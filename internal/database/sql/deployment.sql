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

-- name: DeploymentUpdateMarkBuilding :one
UPDATE deployments
SET build_id = $2, status = 'building'
WHERE id = $1
RETURNING *;
