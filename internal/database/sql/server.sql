-- name: ServerRegister :one
-- Upsert a server record. On startup, a server registers itself.
-- If it already exists (e.g., after restart), update heartbeat and status.
INSERT INTO servers (id, hostname, internal_ip, ip_range, status, last_heartbeat_at)
VALUES ($1, $2, $3, $4, 'active', now())
ON CONFLICT (id) DO UPDATE SET
    hostname = EXCLUDED.hostname,
    internal_ip = EXCLUDED.internal_ip,
    status = 'active',
    last_heartbeat_at = now(),
    updated_at = now()
RETURNING *;

-- name: ServerHeartbeat :exec
-- Update the heartbeat timestamp for a server.
UPDATE servers SET last_heartbeat_at = now() WHERE id = $1;

-- name: ServerFindActive :many
-- Find all servers that are active and have heartbeated recently (within 30s).
SELECT * FROM servers
WHERE status = 'active'
  AND last_heartbeat_at > now() - interval '30 seconds'
  AND deleted_at IS NULL;

-- name: ServerFindByID :one
SELECT * FROM servers WHERE id = $1 LIMIT 1;

-- name: ServerFindLeastLoaded :one
-- Pick the active server with the fewest non-deleted, non-terminal VMs.
-- Used for placement decisions when creating new VMs.
SELECT s.*, COUNT(v.id) as vm_count
FROM servers s
LEFT JOIN vms v ON v.server_id = s.id
    AND v.deleted_at IS NULL
    AND v.status NOT IN ('stopped', 'failed')
WHERE s.status = 'active'
  AND s.last_heartbeat_at > now() - interval '30 seconds'
  AND s.deleted_at IS NULL
GROUP BY s.id
ORDER BY vm_count ASC
LIMIT 1;

-- name: ServerFindDead :many
-- Find servers whose heartbeat has expired (no heartbeat for 60s).
-- Used by the failover detector to identify dead servers.
SELECT * FROM servers
WHERE status = 'active'
  AND last_heartbeat_at < now() - interval '60 seconds'
  AND deleted_at IS NULL;

-- name: ServerUpdateStatus :exec
UPDATE servers SET status = $2, updated_at = now() WHERE id = $1;

-- name: ServerSetDrained :exec
-- Mark a server as drained. All VMs have been migrated off.
UPDATE servers SET status = 'drained', updated_at = now() WHERE id = $1;

-- name: TryAdvisoryLock :one
-- Try to acquire a transaction-scoped advisory lock (non-blocking).
-- Returns true if the lock was acquired, false if another session holds it.
SELECT pg_try_advisory_xact_lock(hashtext($1)) as acquired;

-- name: ServerAllocateIPRange :one
-- Allocate the next available /20 IP range for a new server.
-- First server gets 10.1.0.0/20, second gets 10.1.16.0/20, etc.
-- Each /20 contains 4096 addresses (2048 VMs with /31 pairs).
WITH lock AS (
    SELECT pg_advisory_xact_lock(hashtext('server_ip_range_allocation'))
)
SELECT COALESCE(
    (SELECT host(
                set_masklen(
                    (ip_range + (1 << (32 - masklen(ip_range))))::inet,
                    20
                )
            )::cidr
     FROM servers
     WHERE deleted_at IS NULL
     ORDER BY ip_range DESC
     LIMIT 1),
    '10.1.0.0/20'::cidr
)::cidr AS next_range
FROM lock;
