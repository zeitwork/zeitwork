-- name: RoutingCacheFindByDomain :one
SELECT * FROM routing_cache WHERE domain = $1;

-- name: RoutingCacheFind :many
SELECT * FROM routing_cache ORDER BY domain;

-- name: RoutingCacheCreate :one
INSERT INTO routing_cache (domain, deployment_id, instances, version) 
VALUES ($1, $2, $3, 1) RETURNING *;

-- name: RoutingCacheUpdate :one
UPDATE routing_cache 
SET deployment_id = $2, instances = $3, version = version + 1, updated_at = NOW() 
WHERE domain = $1 RETURNING *;

-- name: RoutingCacheUpsert :one
INSERT INTO routing_cache (domain, deployment_id, instances, version) 
VALUES ($1, $2, $3, 1) 
ON CONFLICT (domain) 
DO UPDATE SET 
    deployment_id = EXCLUDED.deployment_id,
    instances = EXCLUDED.instances,
    version = routing_cache.version + 1,
    updated_at = NOW()
RETURNING *;

-- name: RoutingCacheDelete :exec
DELETE FROM routing_cache WHERE domain = $1;

-- name: RoutingCacheFindByDeployment :many
SELECT * FROM routing_cache WHERE deployment_id = $1;
