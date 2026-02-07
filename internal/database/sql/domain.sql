-- name: DomainFind :many
SELECT *
FROM domains;


-- name: DomainFirstByID :one
SELECT *
FROM domains
WHERE id = $1
LIMIT 1;

-- name: DomainFindActiveByName :many
-- Find all active (non-deleted) domains with a given name, excluding a specific domain ID
SELECT *
FROM domains
WHERE name = $1 AND id != $2 AND deleted_at IS NULL;

-- name: DomainUpdateDeploymentForProject :exec
-- Update all custom domains (non-zeitwork.app) for a project to point to a new deployment
UPDATE domains
SET deployment_id = $1, updated_at = now()
WHERE project_id = $2
  AND name NOT LIKE '%.zeitwork.app'
  AND deleted_at IS NULL;
