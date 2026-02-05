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

-- name: BuildMarkBuilding :exec
UPDATE builds
SET status = 'building', building_at = now(), vm_id = $2
WHERE id = $1;

-- name: BuildMarkSuccessful :exec
UPDATE builds
SET status = 'succesful', successful_at = now(), image_id = $2
WHERE id = $1;

-- name: BuildMarkFailed :exec
UPDATE builds
SET status = 'failed', failed_at = now()
WHERE id = $1;

-- name: BuildLogCreate :exec
INSERT INTO build_logs (id, build_id, message, level, organisation_id, created_at)
VALUES ($1, $2, $3, $4, $5, NOW());

-- name: BuildFindByVMID :many
SELECT * FROM builds WHERE vm_id = $1;

-- name: BuildFindWaitingForBuildImage :many
SELECT * FROM builds 
WHERE status IN ('pending', 'building') 
  AND image_id IS NULL 
  AND deleted_at IS NULL;
