-- name: RouteFindActive :many
-- Domains -> Deployment -> VM -> Server
-- Returns routes with server info so the edge proxy knows which server hosts each VM.
-- With L2 routing between servers, the edge proxy can reach any VM directly by IP.
SELECT
    d.name as domain_name,
    v.port as vm_port,
    v.id as vm_id,
    v.ip_address as vm_ip,
    v.server_id as server_id
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
