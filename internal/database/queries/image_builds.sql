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
    deployment_id,
    started_at,
    completed_at,
    failed_at,
    organisation_id,
    created_at,
    updated_at;

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
    deployment_id,
    started_at,
    completed_at,
    failed_at,
    organisation_id,
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
    deployment_id,
    started_at,
    completed_at,
    failed_at,
    organisation_id,
    created_at,
    updated_at;

-- name: ImageBuildsGetByDeployment :many
-- Get image builds for a deployment
SELECT 
    id,
    status,
    deployment_id,
    started_at,
    completed_at,
    failed_at,
    organisation_id,
    created_at,
    updated_at
FROM image_builds 
WHERE deployment_id = $1
ORDER BY created_at DESC;

-- name: ImageBuildsCreate :one
-- Create a new image build
INSERT INTO image_builds (
    id,
    status,
    deployment_id,
    organisation_id
) VALUES (
    $1,
    'pending',
    $2,
    $3
)
RETURNING 
    id,
    status,
    deployment_id,
    started_at,
    completed_at,
    failed_at,
    organisation_id,
    created_at,
    updated_at;
