-- name: RouteFindActive :many
-- Domains -> Deployment -> VM -> Server
-- Returns routes with server info so the edge proxy knows which server hosts each VM.
-- With L2 routing between servers, the edge proxy can reach any VM directly by IP.
SELECT d.name       AS domain_name,
       v.port       AS vm_port,
       v.id         AS vm_id,
       v.ip_address AS vm_ip,
       v.server_id  AS server_id,
       d.redirect_to AS redirect_to,
       d.redirect_status_code AS redirect_status_code
FROM domains d
         LEFT JOIN deployments dep ON d.deployment_id = dep.id AND dep.stopped_at IS NULL AND dep.failed_at IS NULL AND dep.deleted_at IS NULL
         LEFT JOIN vms v ON dep.vm_id = v.id AND v.deleted_at IS NULL
WHERE d.verified_at IS NOT NULL
  AND d.deleted_at IS NULL
  AND (v.id IS NOT NULL OR d.redirect_to IS NOT NULL)
ORDER BY d.name;

-- name: DomainVerified :one
-- Checks if a domain exists and is verified (for on-demand certificate issuance)
SELECT verified_at
FROM domains
WHERE name = $1 AND deleted_at IS NULL;
