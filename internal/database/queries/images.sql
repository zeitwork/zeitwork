-- name: ImageFindById :one
SELECT * FROM images WHERE id = $1;

-- name: ImageFindByStatus :many
SELECT * FROM images WHERE status = $1;

-- name: ImageFindByName :one
SELECT * FROM images WHERE name = $1;

-- name: ImageFind :many
SELECT * FROM images ORDER BY created_at DESC;

-- name: ImageCreate :one
INSERT INTO images (
    name, status, repository, image_size, image_hash
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: ImageUpdateStatus :one
UPDATE images SET status = $2, image_size = $3, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: ImageUpdateHash :one
UPDATE images SET image_hash = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: ImageDelete :exec
DELETE FROM images WHERE id = $1;
