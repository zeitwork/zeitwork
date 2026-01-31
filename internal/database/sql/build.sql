-- name: BuildFind :many
SELECT *
FROM builds;


-- name: BuildFirstByID :one
SELECT *
FROM builds
WHERE id = $1
LIMIT 1;

-- name: BuildCreate :one
INSERT INTO builds (
    id,
    status,
    project_id,
    github_commit,
    github_branch,
    organisation_id,
    created_at,
    updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
RETURNING *;


-- name: BuildFirstPending :one
SELECT *
FROM builds WHERE status = 'pending'
ORDER BY id DESC
LIMIT 1;

-- name: BuildUpdateMarkBuilding :one
UPDATE builds
SET status = 'building'
WHERE id = $1
RETURNING *;
