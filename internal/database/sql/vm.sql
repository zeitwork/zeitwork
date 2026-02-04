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
INSERT INTO vms (id, vcpus, memory, status, image_id, port, ip_address, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: VMNextIPAddress :one
-- Each VM needs its own /31 subnet, so we increment by 2 to skip to the next block
-- Block 1: 10.0.0.0/31 -> host=10.0.0.0, vm=10.0.0.1
-- Block 2: 10.0.0.2/31 -> host=10.0.0.2, vm=10.0.0.3
-- etc.
WITH lock AS (
    SELECT pg_advisory_xact_lock(hashtext('vm_ip_allocation'))
)
SELECT COALESCE(
               (SELECT set_masklen((ip_address + 2)::inet, 31)
                FROM vms
                WHERE deleted_at IS NULL
                ORDER BY ip_address DESC
                LIMIT 1),
               '10.0.0.1/31'::inet  -- First VM gets 10.0.0.1, host side is 10.0.0.0
       ) AS next_ip
FROM lock;

-- name: VMSoftDelete :exec
UPDATE vms
SET deleted_at = now()
WHERE id = $1;

-- name: VMFindByImageID :many
SELECT * FROM vms WHERE image_id = $1;