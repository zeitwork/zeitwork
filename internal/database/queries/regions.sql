-- name: RegionsGetById :one
-- Get region by ID
SELECT 
    id,
    name,
    code,
    country,
    created_at,
    updated_at
FROM regions 
WHERE id = $1 
    AND deleted_at IS NULL;

-- name: RegionsGetByCode :one
-- Get region by code
SELECT 
    id,
    name,
    code,
    country,
    created_at,
    updated_at
FROM regions 
WHERE code = $1 
    AND deleted_at IS NULL;

-- name: RegionsCreate :one
-- Create a new region
INSERT INTO regions (
    id,
    name,
    code,
    country
) VALUES (
    $1,
    $2,
    $3,
    $4
)
RETURNING 
    id,
    name,
    code,
    country,
    created_at,
    updated_at;

-- name: RegionsGetAll :many
-- Get all regions
SELECT 
    id,
    name,
    code,
    country,
    created_at,
    updated_at
FROM regions 
WHERE deleted_at IS NULL
ORDER BY name ASC;
