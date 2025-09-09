-- name: ImageBuildsDequeuePending :one
-- Get the oldest pending image build and mark it as building
UPDATE image_builds 
SET status = 'building', 
    started_at = now(), 
    updated_at = now()
WHERE id = (
    SELECT id 
    FROM image_builds 
    WHERE status = 'pending' 
    ORDER BY created_at ASC 
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING 
    id,
    status,
    github_repository,
    github_commit,
    image_id,
    started_at,
    completed_at,
    failed_at,
    created_at,
    updated_at;

-- name: ImageBuildsUpdateImageId :one
-- Set image_id for an image build
UPDATE image_builds
SET image_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    image_id;

-- name: ImageBuildsComplete :one
-- Mark an image build as completed
UPDATE image_builds 
SET status = 'completed', 
    completed_at = now(), 
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    status,
    github_repository,
    github_commit,
    image_id,
    started_at,
    completed_at,
    failed_at,
    created_at,
    updated_at;

-- name: ImageBuildsFail :one
-- Mark an image build as failed
UPDATE image_builds 
SET status = 'failed', 
    failed_at = now(), 
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    status,
    github_repository,
    github_commit,
    image_id,
    started_at,
    completed_at,
    failed_at,
    created_at,
    updated_at;

-- name: ImageBuildsGetById :one
-- Get image build by ID
SELECT 
    id,
    status,
    github_repository,
    github_commit,
    image_id,
    started_at,
    completed_at,
    failed_at,
    created_at,
    updated_at
FROM image_builds 
WHERE id = $1;

-- name: ImageBuildsCreate :one
-- Create a new image build
INSERT INTO image_builds (
    id,
    status,
    github_repository,
    github_commit
) VALUES (
    $1,
    'pending',
    $2,
    $3
)
RETURNING 
    id,
    status,
    github_repository,
    github_commit,
    image_id,
    started_at,
    completed_at,
    failed_at,
    created_at,
    updated_at;

-- name: ImageBuildsResetStale :many
-- Reset builds that have been "building" for too long (using minutes parameter)
UPDATE image_builds 
SET status = 'pending',
    started_at = NULL,
    updated_at = now()
WHERE status = 'building' 
  AND started_at < NOW() - ($1 || ' minutes')::INTERVAL
RETURNING 
    id,
    status,
    github_repository,
    github_commit,
    image_id,
    started_at,
    completed_at,
    failed_at,
    created_at,
    updated_at;