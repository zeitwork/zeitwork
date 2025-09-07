-- SSL certificates and locks queries for CertManager and DB-backed storage

-- name: SslCertsGetByKey :one
SELECT 
    id,
    "key",
    value,
    expires_at,
    created_at,
    updated_at
FROM ssl_certs
WHERE "key" = $1
  AND deleted_at IS NULL;

-- name: SslCertsUpsert :one
INSERT INTO ssl_certs (
    id,
    "key",
    value,
    expires_at
) VALUES (
    $1,
    $2,
    $3,
    $4
)
ON CONFLICT ("key") DO UPDATE SET
    value = EXCLUDED.value,
    expires_at = EXCLUDED.expires_at,
    updated_at = now()
RETURNING 
    id,
    "key",
    value,
    expires_at,
    created_at,
    updated_at;

-- name: SslCertsDelete :execrows
UPDATE ssl_certs
SET deleted_at = now(), updated_at = now()
WHERE "key" = $1
  AND deleted_at IS NULL;

-- name: SslCertsListPrefix :many
SELECT "key" AS key
FROM ssl_certs
WHERE "key" LIKE ($1 || '%')
  AND deleted_at IS NULL;

-- name: SslCertsStat :one
SELECT 
    char_length(value) AS size,
    updated_at
FROM ssl_certs
WHERE "key" = $1
  AND deleted_at IS NULL;

-- Locking helpers

-- name: SslLocksTryAcquire :one
INSERT INTO ssl_locks (id, "key", expires_at)
VALUES ($2, $1, $3)
ON CONFLICT ("key") DO UPDATE SET
    id = EXCLUDED.id,
    expires_at = EXCLUDED.expires_at,
    updated_at = now()
WHERE ssl_locks.expires_at < now()
RETURNING id;

-- name: SslLocksRelease :execrows
DELETE FROM ssl_locks
WHERE "key" = $1;


