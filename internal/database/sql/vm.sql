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
WITH lock AS (
    SELECT pg_advisory_xact_lock(hashtext('vm_ip_allocation'))
)
SELECT COALESCE(
               (SELECT set_masklen((ip_address + 1)::inet, 31)
                FROM vms
                WHERE deleted_at IS NULL
                ORDER BY ip_address DESC
                LIMIT 1),
               '10.0.0.1/31'::inet  -- Also update the default to include /31
       ) AS next_ip
FROM lock;

-- name: VMSoftDelete :exec
UPDATE vms
SET deleted_at = now()
WHERE id = $1;