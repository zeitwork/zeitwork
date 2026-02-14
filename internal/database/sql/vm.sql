-- name: VMFind :many
SELECT *
FROM vms;

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
-- Each VM needs its own /31 subnet, so we increment by 2 to skip to the next block.
-- The first VM in a range gets base+1 (e.g., 10.1.0.1/31), host side is base+0.
WITH lock AS (
  SELECT pg_advisory_xact_lock(hashtext('vm_ip_allocation'))
)
SELECT set_masklen((network(@ip_range::inet) + gs)::inet, 31) AS free_ip
FROM lock, generate_series(1, (2 ^ (32 - masklen(@ip_range::inet)))::int - 2, 2) gs
WHERE NOT EXISTS (
  SELECT 1 FROM vms
  WHERE deleted_at IS NULL
    AND server_id = @server_id
    AND ip_address = set_masklen((network(@ip_range::inet) + gs)::inet, 31)
)
ORDER BY gs
LIMIT 1;

-- name: VMSoftDelete :exec
UPDATE vms
SET deleted_at = COALESCE(deleted_at, now())
WHERE id = $1;

-- name: VMFindByImageID :many
SELECT * FROM vms WHERE image_id = $1;

-- name: VMFindByServerID :many
SELECT * FROM vms WHERE server_id = $1 AND deleted_at IS NULL;
