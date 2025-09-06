-- name: DeploymentsGetActiveRoutes :many
-- Get active deployment routes for edge proxy
SELECT 
    d.id as deployment_id,
    d.deployment_id as deployment_name,
    d.status,
    dom.name as domain,
    i.ipv6_address as ip_address,
    i.default_port,
    CASE 
        WHEN i.state = 'running' THEN true 
        ELSE false 
    END as healthy
FROM deployments d
JOIN domains dom ON dom.deployment_id = d.id
JOIN deployment_instances di ON di.deployment_id = d.id
JOIN instances i ON i.id = di.instance_id
WHERE d.status = 'active'
    AND dom.verified_at IS NOT NULL
    AND i.state IN ('running', 'starting')
    AND dom.deleted_at IS NULL
    AND d.deleted_at IS NULL
    AND i.deleted_at IS NULL;

-- name: DeploymentsGetById :one
-- Get deployment by ID
SELECT 
    id,
    deployment_id,
    status,
    commit_hash,
    project_id,
    environment_id,
    image_id,
    organisation_id,
    created_at,
    updated_at
FROM deployments 
WHERE id = $1 
    AND deleted_at IS NULL;

-- name: DeploymentsGetByProject :many
-- Get deployments by project ID
SELECT 
    id,
    deployment_id,
    status,
    commit_hash,
    project_id,
    environment_id,
    image_id,
    organisation_id,
    created_at,
    updated_at
FROM deployments 
WHERE project_id = $1 
    AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: DeploymentsCreate :one
-- Create a new deployment
INSERT INTO deployments (
    id,
    deployment_id,
    status,
    commit_hash,
    project_id,
    environment_id,
    image_id,
    organisation_id
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8
)
RETURNING 
    id,
    deployment_id,
    status,
    commit_hash,
    project_id,
    environment_id,
    image_id,
    organisation_id,
    created_at,
    updated_at;

-- name: DeploymentsUpdateStatus :one
-- Update deployment status
UPDATE deployments 
SET status = $2, 
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    deployment_id,
    status,
    commit_hash,
    project_id,
    environment_id,
    image_id,
    organisation_id,
    created_at,
    updated_at;

-- name: DeploymentsGetPendingWithoutBuilds :many
-- Get pending deployments that don't have any image builds yet
SELECT 
    d.id,
    d.deployment_id,
    d.status,
    d.commit_hash,
    d.project_id,
    d.environment_id,
    d.image_id,
    d.organisation_id,
    d.created_at,
    d.updated_at,
    p.github_repository,
    p.default_branch
FROM deployments d
JOIN projects p ON p.id = d.project_id
LEFT JOIN image_builds ib ON ib.deployment_id = d.id
WHERE d.status = 'pending' 
    AND d.deleted_at IS NULL
    AND p.deleted_at IS NULL
    AND ib.id IS NULL
ORDER BY d.created_at ASC;

-- name: DeploymentsGetReadyForDeployment :many
-- Get deployments that have completed builds but no instances yet (ready for deployment)
SELECT 
    d.id,
    d.deployment_id,
    d.status,
    d.commit_hash,
    d.project_id,
    d.environment_id,
    d.image_id,
    d.organisation_id,
    d.created_at,
    d.updated_at,
    ib.id as build_id
FROM deployments d
JOIN image_builds ib ON ib.deployment_id = d.id
LEFT JOIN deployment_instances di ON di.deployment_id = d.id
WHERE d.status = 'deploying'
    AND ib.status = 'completed'
    AND d.deleted_at IS NULL
    AND di.id IS NULL
ORDER BY d.created_at ASC;

-- name: DeploymentsUpdateImageId :one
-- Update deployment image_id after successful build
UPDATE deployments 
SET image_id = $2, 
    updated_at = now()
WHERE id = $1
RETURNING 
    id,
    deployment_id,
    status,
    commit_hash,
    project_id,
    environment_id,
    image_id,
    organisation_id,
    created_at,
    updated_at;
