-- name: RouteFindActive :many
-- Domains -> Deployment -> VM -> Server
-- Returns routes with server info so the edge proxy can forward cross-server.
SELECT
    d.name as domain_name,
    v.port as vm_port,
    v.id as vm_id,
    v.ip_address as vm_ip,
    v.server_id as server_id,
    s.internal_ip as server_internal_ip
FROM domains d
         INNER JOIN deployments dep ON d.deployment_id = dep.id
         INNER JOIN vms v ON dep.vm_id = v.id
         LEFT JOIN servers s ON v.server_id = s.id
WHERE d.verified_at IS NOT NULL
  AND dep.status = 'running'
  AND v.status = 'running'
ORDER BY d.name;

-- name: DomainVerified :one
-- Checks if a domain exists and is verified (for on-demand certificate issuance)
SELECT verified_at
FROM domains
WHERE name = $1;
