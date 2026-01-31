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
