-- name: GetActiveRoutes :many
SELECT 
    d.name as domain_name,
    i.id as instance_id,
    i.ip_address,
    i.default_port,
    i.region_id
FROM domains d
INNER JOIN deployments dep ON d.deployment_id = dep.id
INNER JOIN deployment_instances di ON di.deployment_id = dep.id
INNER JOIN instances i ON di.instance_id = i.id
WHERE d.deleted_at IS NULL
  AND dep.deleted_at IS NULL
  AND dep.status = 'active'
  AND i.state = 'running'
  AND i.deleted_at IS NULL
ORDER BY d.name, i.created_at DESC;

