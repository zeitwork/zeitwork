-- DOMAIN QUERIES

-- name: GetUnverifiedDomains :many
-- Get domains that need DNS verification (unverified and recently updated)
SELECT *
FROM domains
WHERE verified_at IS NULL
  AND deleted_at IS NULL
  AND updated_at > NOW() - INTERVAL '48 hours'
ORDER BY updated_at DESC;

-- name: MarkDomainVerified :exec
-- Mark a domain as verified
UPDATE domains
SET verified_at = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- DEPLOYMENT QUERIES

-- name: GetQueuedDeployments :many
-- Get deployments in queued state (no build assigned)
SELECT *
FROM deployments
WHERE status = 'queued'
  AND build_id IS NULL
  AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: GetBuildingDeploymentsWithoutImage :many
-- Get building deployments that have a build but no image yet
SELECT d.*, b.status as build_status, b.image_id as build_image_id
FROM deployments d
INNER JOIN builds b ON d.build_id = b.id
WHERE d.status = 'building'
  AND d.image_id IS NULL
  AND d.deleted_at IS NULL
  AND b.deleted_at IS NULL
ORDER BY d.created_at ASC;

-- name: GetBuildingDeploymentsWithoutVM :many
-- Get building deployments that have an image but no VM assigned
SELECT *
FROM deployments
WHERE status = 'building'
  AND image_id IS NOT NULL
  AND vm_id IS NULL
  AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: GetReadyDeployments :many
-- Get all ready deployments grouped by project+environment
SELECT *
FROM deployments
WHERE status = 'ready'
  AND deleted_at IS NULL
ORDER BY project_id, environment_id, created_at DESC;

-- name: GetInactiveDeployments :many
-- Get inactive deployments that need cleanup
SELECT *
FROM deployments
WHERE status = 'inactive'
  AND vm_id IS NOT NULL
  AND deleted_at IS NULL;

-- name: GetFailedDeployments :many
-- Get failed deployments that need cleanup
SELECT *
FROM deployments
WHERE status = 'failed'
  AND vm_id IS NOT NULL
  AND deleted_at IS NULL;

-- name: CreateBuild :one
-- Create a new build for a deployment
INSERT INTO builds (
    id,
    status,
    project_id,
    organisation_id,
    created_at,
    updated_at
)
VALUES ($1, $2, $3, $4, NOW(), NOW())
RETURNING *;

-- name: UpdateDeploymentWithBuild :exec
-- Update deployment with build_id and change status to building
UPDATE deployments
SET build_id = $2,
    status = 'building',
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateDeploymentWithImage :exec
-- Update deployment with image_id
UPDATE deployments
SET image_id = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateDeploymentWithVM :exec
-- Update deployment with vm_id and change status to ready
UPDATE deployments
SET vm_id = $2,
    status = 'ready',
    updated_at = NOW()
WHERE id = $1;

-- name: MarkDeploymentInactive :exec
-- Mark deployment as inactive
UPDATE deployments
SET status = 'inactive',
    updated_at = NOW()
WHERE id = $1;

-- name: MarkDeploymentFailed :exec
-- Mark deployment as failed
UPDATE deployments
SET status = 'failed',
    updated_at = NOW()
WHERE id = $1;

-- name: ClearDeploymentVM :exec
-- Clear VM assignment from deployment
UPDATE deployments
SET vm_id = NULL,
    updated_at = NOW()
WHERE id = $1;

-- BUILD QUERIES

-- name: GetTimedOutBuilds :many
-- Get builds that have been in "building" state for too long
SELECT *
FROM builds
WHERE status = 'building'
  AND deleted_at IS NULL
  AND updated_at < NOW() - INTERVAL '10 minutes'
ORDER BY updated_at ASC;

-- name: MarkBuildTimedOut :exec
-- Mark a build as error due to timeout
UPDATE builds
SET status = 'error',
    updated_at = NOW()
WHERE id = $1;

-- VM QUERIES

-- name: GetPoolVMs :many
-- Get VMs that are available in the pool
SELECT *
FROM vms
WHERE status = 'pooling'
  AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: GetPoolAndInitializingVMs :many
-- Get VMs that are in pool or being initialized (for pool size calculation)
SELECT *
FROM vms
WHERE status IN ('pooling', 'initializing')
  AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: GetVMsByRegion :many
-- Get count of VMs by region and status
SELECT 
    region_id,
    status,
    COUNT(*) as count
FROM vms
WHERE deleted_at IS NULL
GROUP BY region_id, status;

-- name: GetAllRegions :many
-- Get all regions
SELECT *
FROM regions
WHERE deleted_at IS NULL
ORDER BY no ASC;

-- name: GetNextRegionNumber :one
-- Get the next available region number
SELECT COALESCE(MAX(no), 0) + 1 as next_no
FROM regions;

-- name: CreateRegion :one
-- Create a new region
INSERT INTO regions (
    id,
    no,
    name,
    load_balancer_ipv4,
    load_balancer_ipv6,
    load_balancer_no,
    created_at,
    updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
RETURNING *;

-- name: CreateVM :one
-- Create a new VM
INSERT INTO vms (
    id,
    no,
    status,
    region_id,
    port,
    created_at,
    updated_at
)
VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
RETURNING *;

-- name: GetNextVMNumber :one
-- Get the next available VM number
SELECT COALESCE(MAX(no), 0) + 1 as next_no
FROM vms;

-- name: AssignVMToDeployment :exec
-- Assign a VM to a deployment and update VM status
UPDATE vms
SET status = 'starting',
    image_id = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: ReturnVMToPool :exec
-- Return a VM to the pool
UPDATE vms
SET status = 'pooling',
    image_id = NULL,
    updated_at = NOW()
WHERE id = $1;

-- name: MarkVMDeleting :exec
-- Mark a VM for deletion
UPDATE vms
SET status = 'deleting',
    updated_at = NOW()
WHERE id = $1;

-- name: GetVMByID :one
-- Get a VM by ID
SELECT *
FROM vms
WHERE id = $1
  AND deleted_at IS NULL;

-- name: UpdateVMServerDetails :exec
-- Update VM with server details after Hetzner server creation
UPDATE vms
SET server_name = $2,
    server_type = $3,
    public_ip = $4,
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateVMHetznerID :exec
-- Update VM with Hetzner server ID after server creation
UPDATE vms
SET server_no = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: ClearVMImage :exec
-- Clear image from VM
UPDATE vms
SET image_id = NULL,
    updated_at = NOW()
WHERE id = $1;

-- name: GetImageByID :one
-- Get image details by ID
SELECT *
FROM images
WHERE id = $1
  AND deleted_at IS NULL;

-- name: MarkVMRunning :exec
-- Mark VM as running after container deployment
UPDATE vms
SET status = 'running',
    updated_at = NOW()
WHERE id = $1;

-- name: GetDeletingVMs :many
-- Get VMs marked for deletion
SELECT *
FROM vms
WHERE status = 'deleting'
  AND deleted_at IS NULL
ORDER BY updated_at ASC;

-- name: MarkVMDeleted :exec
-- Mark VM as deleted
UPDATE vms
SET deleted_at = NOW()
WHERE id = $1;

