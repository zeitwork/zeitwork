-- name: ImagesGetById :one
-- Get image by ID
SELECT 
    id,
    name,
    size,
    hash,
    object_key,
    created_at,
    updated_at
FROM images 
WHERE id = $1 
    AND deleted_at IS NULL;

-- name: ImagesGetByHash :one
-- Get image by hash
SELECT 
    id,
    name,
    size,
    hash,
    object_key,
    created_at,
    updated_at
FROM images 
WHERE hash = $1 
    AND deleted_at IS NULL;

-- name: ImagesCreate :one
-- Create a new image
INSERT INTO images (
    id,
    name,
    size,
    hash,
    object_key
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5
)
RETURNING 
    id,
    name,
    size,
    hash,
    object_key,
    created_at,
    updated_at;

-- name: ImagesUpdate :one
-- Update image
UPDATE images 
SET name = $2, 
    size = $3,
    hash = $4,
    object_key = $5,
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    name,
    size,
    hash,
    object_key,
    created_at,
    updated_at;

-- name: ImagesGetAll :many
-- Get all images
SELECT 
    id,
    name,
    size,
    hash,
    object_key,
    created_at,
    updated_at
FROM images 
WHERE deleted_at IS NULL
ORDER BY created_at DESC;
