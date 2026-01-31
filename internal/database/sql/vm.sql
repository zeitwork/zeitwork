-- name: VMFind :many
SELECT *
FROM vms;

-- name: VMFirstByID :one
SELECT *
FROM vms
WHERE id = $1
LIMIT 1;
