-- BUILDER QUERIES

-- name: GetPendingBuild :one
-- Get next pending build with row-level locking
SELECT *
FROM builds
WHERE status = 'queued'
  AND deleted_at IS NULL
ORDER BY created_at ASC
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: MarkBuildBuilding :exec
-- Mark build as building
UPDATE builds
SET status = 'building',
    updated_at = NOW()
WHERE id = $1;

-- name: MarkBuildReady :exec
-- Mark build as ready with image_id
UPDATE builds
SET status = 'ready',
    image_id = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: MarkBuildError :exec
-- Mark build as error
UPDATE builds
SET status = 'error',
    updated_at = NOW()
WHERE id = $1;

-- name: GetGithubInstallationByID :one
-- Get GitHub installation details
SELECT *
FROM github_installations
WHERE id = $1
  AND deleted_at IS NULL;

-- name: UpdateBuildVM :exec
-- Assign VM to build
UPDATE builds
SET vm_id = $2,
    updated_at = NOW()
WHERE id = $1;

