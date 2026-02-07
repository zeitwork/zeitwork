-- name: RouteFindActive :many
-- Domains -> Deployment -> VM
SELECT
    d.name as domain_name,
    v.port as vm_port,
    v.id as vm_id,
    v.ip_address as vm_ip
FROM domains d
         INNER JOIN deployments dep ON d.deployment_id = dep.id
         INNER JOIN vms v ON dep.vm_id = v.id
WHERE d.verified_at IS NOT NULL
  AND d.deleted_at IS NULL
  AND dep.status = 'running'
  AND v.status = 'running'
ORDER BY d.name;

-- name: DomainVerified :one
-- Checks if a domain exists and is verified (for on-demand certificate issuance)
SELECT verified_at
FROM domains
WHERE name = $1 AND deleted_at IS NULL;
