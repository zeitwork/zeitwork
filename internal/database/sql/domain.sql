-- name: DomainFind :many
SELECT *
FROM domains;


-- name: DomainFirstByID :one
SELECT *
FROM domains
WHERE id = $1
LIMIT 1;
