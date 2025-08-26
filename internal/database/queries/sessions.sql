-- name: SessionFindById :one
SELECT * FROM sessions WHERE id = $1;

-- name: SessionFindByToken :one
SELECT * FROM sessions WHERE token = $1;

-- name: SessionFindByUser :many
SELECT * FROM sessions WHERE user_id = $1 ORDER BY created_at DESC;

-- name: SessionFindActive :many
SELECT * FROM sessions WHERE expires_at > NOW() ORDER BY created_at DESC;

-- name: SessionFindByUserAndNotExpired :one
SELECT * FROM sessions WHERE user_id = $1 AND expires_at > NOW() ORDER BY created_at DESC LIMIT 1;

-- name: SessionCreate :one
INSERT INTO sessions (user_id, token, expires_at) VALUES ($1, $2, $3) RETURNING *;

-- name: SessionUpdate :one
UPDATE sessions SET expires_at = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: SessionDelete :exec
DELETE FROM sessions WHERE id = $1;

-- name: SessionDeleteByToken :exec
DELETE FROM sessions WHERE token = $1;

-- name: SessionDeleteExpired :exec
DELETE FROM sessions WHERE expires_at < NOW();