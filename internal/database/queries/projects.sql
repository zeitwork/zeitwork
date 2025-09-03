-- name: ProjectsGetById :one
-- Get project by ID
SELECT 
    id,
    name,
    slug,
    github_repository,
    default_branch,
    latest_deployment_id,
    organisation_id,
    created_at,
    updated_at
FROM projects 
WHERE id = $1 
    AND deleted_at IS NULL;

-- name: ProjectsGetBySlugAndOrg :one
-- Get project by slug and organisation
SELECT 
    id,
    name,
    slug,
    github_repository,
    default_branch,
    latest_deployment_id,
    organisation_id,
    created_at,
    updated_at
FROM projects 
WHERE slug = $1 
    AND organisation_id = $2
    AND deleted_at IS NULL;

-- name: ProjectsGetByOrganisation :many
-- Get projects by organisation
SELECT 
    id,
    name,
    slug,
    github_repository,
    default_branch,
    latest_deployment_id,
    organisation_id,
    created_at,
    updated_at
FROM projects 
WHERE organisation_id = $1 
    AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: ProjectsCreate :one
-- Create a new project
INSERT INTO projects (
    id,
    name,
    slug,
    github_repository,
    default_branch,
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
    slug,
    github_repository,
    default_branch,
    latest_deployment_id,
    organisation_id,
    created_at,
    updated_at;

-- name: ProjectsUpdateLatestDeployment :one
-- Update project's latest deployment
UPDATE projects 
SET latest_deployment_id = $2, 
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    name,
    slug,
    github_repository,
    default_branch,
    latest_deployment_id,
    organisation_id,
    created_at,
    updated_at;
