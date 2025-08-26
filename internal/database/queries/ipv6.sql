-- name: IPv6AllocationFindById :one
SELECT * FROM ipv6_allocations WHERE id = $1;

-- name: IPv6AllocationFindByAddress :one
SELECT * FROM ipv6_allocations WHERE ipv6_address = $1;

-- name: IPv6AllocationFindByInstance :one
SELECT * FROM ipv6_allocations WHERE instance_id = $1;

-- name: IPv6AllocationFindByNode :many
SELECT * FROM ipv6_allocations WHERE node_id = $1 ORDER BY ipv6_address;

-- name: IPv6AllocationFindByRegion :many
SELECT * FROM ipv6_allocations WHERE region_id = $1 ORDER BY ipv6_address;

-- name: IPv6AllocationFindAvailable :many
SELECT * FROM ipv6_allocations 
WHERE state = 'released' AND region_id = $1 
ORDER BY ipv6_address 
LIMIT $2;

-- name: IPv6AllocationCreate :one
INSERT INTO ipv6_allocations (region_id, node_id, instance_id, ipv6_address, prefix, state) 
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: IPv6AllocationAllocate :one
UPDATE ipv6_allocations 
SET instance_id = $2, state = 'allocated', updated_at = NOW() 
WHERE id = $1 AND state IN ('released', 'reserved')
RETURNING *;

-- name: IPv6AllocationRelease :one
UPDATE ipv6_allocations 
SET instance_id = NULL, state = 'released', updated_at = NOW() 
WHERE id = $1 RETURNING *;

-- name: IPv6AllocationReserve :one
UPDATE ipv6_allocations 
SET state = 'reserved', updated_at = NOW() 
WHERE id = $1 AND state = 'released'
RETURNING *;
