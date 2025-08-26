-- name: UserFindById :one
SELECT * FROM users WHERE id = $1;

-- name: UserFindByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: UserFindByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: UserFindByGithubId :one
SELECT * FROM users WHERE github_user_id = $1;

-- name: UserFind :many
SELECT * FROM users ORDER BY created_at DESC;

-- name: UserCreate :one
INSERT INTO users (name, email, username, github_user_id) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: UserUpdate :one
UPDATE users SET name = $2, username = $3, github_user_id = $4, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: UserDelete :exec
DELETE FROM users WHERE id = $1;
