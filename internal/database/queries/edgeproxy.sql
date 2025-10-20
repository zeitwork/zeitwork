-- name: GetActiveRoutes :many
-- Returns active routing information for the edgeproxy
-- Joins domains → deployments → vms → regions
SELECT 
    d.name as domain_name,
    v.public_ip as vm_public_ip,
    v.port as vm_port,
    v.region_id as vm_region_id,
    r.load_balancer_ipv4 as region_load_balancer_ip
FROM domains d
INNER JOIN deployments dep ON d.deployment_id = dep.id
INNER JOIN vms v ON dep.vm_id = v.id
INNER JOIN regions r ON v.region_id = r.id
WHERE d.verified_at IS NOT NULL
  AND dep.status = 'ready'
  AND v.status = 'running'
ORDER BY d.name;

