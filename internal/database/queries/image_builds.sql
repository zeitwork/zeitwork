-- name: GetPendingImageBuild :one
SELECT 
    id,
    status,
    github_repository,
    github_commit,
    github_installation_id,
    image_id,
    started_at,
    completed_at,
    failed_at,
    created_at,
    updated_at
FROM image_builds
WHERE status = 'pending'
  AND started_at IS NULL
ORDER BY created_at ASC
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: UpdateImageBuildStarted :exec
UPDATE image_builds
SET 
    status = 'building',
    started_at = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateImageBuildCompleted :exec
UPDATE image_builds
SET 
    status = 'completed',
    image_id = $2,
    completed_at = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateImageBuildFailed :exec
UPDATE image_builds
SET 
    status = 'failed',
    failed_at = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- name: CreateImage :one
INSERT INTO images (
    id,
    name,
    size,
    hash,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, NOW(), NOW()
)
RETURNING *;

