-- name: GetActiveRoutes :many
-- Returns active routing information for the edgeproxy
-- Joins domains → deployments → vms → regions
SELECT 
    d.name as domain_name,
    v.public_ip as vm_public_ip,
    v.port as vm_port,
    v.region_id as vm_region_id,
    r.load_balancer_ipv4 as region_load_balancer_ip
FROM domains d
INNER JOIN deployments dep ON d.deployment_id = dep.id
INNER JOIN vms v ON dep.vm_id = v.id
INNER JOIN regions r ON v.region_id = r.id
WHERE d.verified_at IS NOT NULL
  AND dep.status = 'ready'
  AND v.status = 'running'
ORDER BY d.name;

-- name: GetDomainsNeedingCertificates :many
-- Returns domains that need SSL certificates (verified but not yet active)
SELECT 
    id,
    name,
    ssl_certificate_status,
    ssl_certificate_expires_at
FROM domains
WHERE verified_at IS NOT NULL
  AND (
    ssl_certificate_status IS NULL 
    OR ssl_certificate_status != 'active'
    OR ssl_certificate_expires_at IS NULL
    OR ssl_certificate_expires_at < NOW() + INTERVAL '30 days'
  )
ORDER BY name;

-- name: UpdateDomainCertificateStatus :exec
-- Updates the SSL certificate status for a domain
UPDATE domains
SET 
    ssl_certificate_status = $2,
    ssl_certificate_issued_at = $3,
    ssl_certificate_expires_at = $4,
    ssl_certificate_error = $5,
    updated_at = NOW()
WHERE id = $1;

-- Certmagic Storage Queries

-- name: StoreCertmagicData :exec
-- Stores or updates a certmagic data entry
INSERT INTO certmagic_data (key, value, modified)
VALUES ($1, $2, $3)
ON CONFLICT (key)
DO UPDATE SET 
    value = EXCLUDED.value, 
    modified = EXCLUDED.modified;

-- name: LoadCertmagicData :one
-- Loads a certmagic data entry by key
SELECT key, value, modified
FROM certmagic_data
WHERE key = $1;

-- name: DeleteCertmagicData :execrows
-- Deletes a certmagic data entry by key
DELETE FROM certmagic_data
WHERE key = $1;

-- name: ExistsCertmagicData :one
-- Checks if a certmagic data entry exists
SELECT EXISTS(SELECT 1 FROM certmagic_data WHERE key = $1);

-- name: ListCertmagicDataRecursive :many
-- Lists all certmagic data keys with a given prefix (recursive)
SELECT key
FROM certmagic_data
WHERE key LIKE $1 || '%'
ORDER BY key;

-- name: ListCertmagicDataNonRecursive :many
-- Lists certmagic data keys with a given prefix (non-recursive - no additional slashes)
SELECT key
FROM certmagic_data
WHERE key LIKE $1 || '%'
  AND position('/' in substring(key from length($1) + 1)) = 0
ORDER BY key;

-- name: StatCertmagicData :one
-- Returns metadata about a certmagic data entry
SELECT key, value, modified
FROM certmagic_data
WHERE key = $1;

-- name: AcquireCertmagicLock :execrows
-- Attempts to acquire a lock for a key
INSERT INTO certmagic_locks (key, expires)
VALUES ($1, $2)
ON CONFLICT (key) 
DO UPDATE SET expires = EXCLUDED.expires
WHERE certmagic_locks.expires < NOW();

-- name: ReleaseCertmagicLock :execrows
-- Releases a lock for a key
DELETE FROM certmagic_locks
WHERE key = $1;

-- name: CleanupExpiredCertmagicLocks :exec
-- Removes expired locks
DELETE FROM certmagic_locks
WHERE expires < NOW();

