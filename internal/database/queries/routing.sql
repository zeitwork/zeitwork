-- name: RoutingCacheUpsert :one
INSERT INTO routing_cache (
    domain, 
    deployment_id, 
    instances,
    version
) VALUES (
    $1, $2, $3, $4
)
ON CONFLICT (domain) 
DO UPDATE SET 
    deployment_id = EXCLUDED.deployment_id,
    instances = EXCLUDED.instances,
    version = EXCLUDED.version,
    updated_at = NOW()
RETURNING *;

-- name: RoutingCacheFindByDomain :one
SELECT * FROM routing_cache WHERE domain = $1;

-- name: RoutingCacheFindByDeployment :many
SELECT * FROM routing_cache WHERE deployment_id = $1;

-- name: RoutingCacheDeleteByDomain :exec
DELETE FROM routing_cache WHERE domain = $1;

-- name: RoutingCacheDeleteByDeployment :exec
DELETE FROM routing_cache WHERE deployment_id = $1;

-- name: RoutingCacheCleanup :exec
-- Remove entries older than 24 hours
DELETE FROM routing_cache 
WHERE updated_at < NOW() - INTERVAL '24 hours';