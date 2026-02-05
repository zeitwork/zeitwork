-- name: DomainListUnverified :many
SELECT *
FROM domains
WHERE verified_at IS NULL AND deleted_at IS NULL;

-- name: DomainMarkVerified :exec
UPDATE domains
SET verified_at = NOW()
WHERE id = $1;
