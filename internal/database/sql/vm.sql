-- name: VMFind :many
SELECT *
FROM vms;

-- name: VMFindByServerID :many
-- Find all non-deleted VMs assigned to a specific server.
SELECT * FROM vms
WHERE server_id = $1
  AND deleted_at IS NULL;

-- name: VMFirstByID :one
SELECT *
FROM vms
WHERE id = $1
LIMIT 1;

-- name: VMUpdateStatus :one
update vms set status = $1 where id=$2 returning *;

-- name: VMCreate :one
INSERT INTO vms (id, vcpus, memory, status, image_id, server_id, port, ip_address, env_variables, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: VMNextIPAddress :one
-- Allocate the next /31 subnet within a server's IP range.
-- Each VM needs its own /31 subnet:
--   Block 1: base+0/31 -> host=base+0, vm=base+1
--   Block 2: base+2/31 -> host=base+2, vm=base+3
-- $1 = server_id, $2 = server's ip_range (cidr, e.g., '10.1.0.0/16')
WITH lock AS (
    SELECT pg_advisory_xact_lock(hashtext('vm_ip_allocation_' || $1::text))
)
SELECT COALESCE(
               (SELECT set_masklen((ip_address + 2)::inet, 31)
                FROM vms
                WHERE server_id = $1 AND deleted_at IS NULL
                ORDER BY ip_address DESC
                LIMIT 1),
               -- First VM in this server's range: base_addr + 1 (e.g., 10.1.0.1/31)
               set_masklen((host(network($2::cidr))::inet + 1), 31)
       ) AS next_ip
FROM lock;

-- name: VMSoftDelete :exec
UPDATE vms
SET deleted_at = now()
WHERE id = $1;

-- name: VMFindByImageID :many
SELECT * FROM vms WHERE image_id = $1;

-- name: VMReassign :one
-- Reassign a VM to a different server with a new IP address.
-- Used during failover when a server dies.
UPDATE vms
SET server_id = $2, ip_address = $3, status = 'pending'
WHERE id = $1
RETURNING *;
