-- name: DomainsGetById :one
-- Get domain by ID
SELECT 
    id,
    name,
    verification_token,
    verified_at,
    deployment_id,
    internal,
    organisation_id,
    created_at,
    updated_at
FROM domains 
WHERE id = $1 
    AND deleted_at IS NULL;

-- name: DomainsGetByName :one
-- Get domain by name
SELECT 
    id,
    name,
    verification_token,
    verified_at,
    deployment_id,
    internal,
    organisation_id,
    created_at,
    updated_at
FROM domains 
WHERE name = $1 
    AND deleted_at IS NULL;

-- name: DomainsGetByDeployment :many
-- Get domains by deployment
SELECT 
    id,
    name,
    verification_token,
    verified_at,
    deployment_id,
    internal,
    organisation_id,
    created_at,
    updated_at
FROM domains 
WHERE deployment_id = $1 
    AND deleted_at IS NULL;

-- name: DomainsListVerified :many
-- List all verified domains
SELECT 
    id,
    name,
    verification_token,
    verified_at,
    deployment_id,
    internal,
    organisation_id,
    created_at,
    updated_at
FROM domains
WHERE verified_at IS NOT NULL
  AND deleted_at IS NULL;

-- name: DomainsListAll :many
-- List all domains regardless of verification
SELECT 
    id,
    name,
    verification_token,
    verified_at,
    deployment_id,
    internal,
    organisation_id,
    created_at,
    updated_at
FROM domains
WHERE deleted_at IS NULL;

-- name: DomainsCreate :one
-- Create a new domain
INSERT INTO domains (
    id,
    name,
    verification_token,
    deployment_id,
    internal,
    organisation_id
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6
)
RETURNING 
    id,
    name,
    verification_token,
    verified_at,
    deployment_id,
    internal,
    organisation_id,
    created_at,
    updated_at;

-- name: DomainsVerify :one
-- Mark domain as verified
UPDATE domains 
SET verified_at = now(), 
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    name,
    verification_token,
    verified_at,
    deployment_id,
    internal,
    organisation_id,
    created_at,
    updated_at;
