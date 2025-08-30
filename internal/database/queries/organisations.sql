-- name: OrganisationsGetById :one
-- Get organisation by ID
SELECT 
    id,
    name,
    slug,
    created_at,
    updated_at
FROM organisations 
WHERE id = $1 
    AND deleted_at IS NULL;

-- name: OrganisationsGetBySlug :one
-- Get organisation by slug
SELECT 
    id,
    name,
    slug,
    created_at,
    updated_at
FROM organisations 
WHERE slug = $1 
    AND deleted_at IS NULL;

-- name: OrganisationsCreate :one
-- Create a new organisation
INSERT INTO organisations (
    id,
    name,
    slug
) VALUES (
    $1,
    $2,
    $3
)
RETURNING 
    id,
    name,
    slug,
    created_at,
    updated_at;

-- name: OrganisationsGetAll :many
-- Get all organisations
SELECT 
    id,
    name,
    slug,
    created_at,
    updated_at
FROM organisations 
WHERE deleted_at IS NULL
ORDER BY created_at DESC;
