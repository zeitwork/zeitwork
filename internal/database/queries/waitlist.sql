-- name: WaitlistFindById :one
SELECT * FROM waitlist WHERE id = $1;

-- name: WaitlistFindByEmail :one
SELECT * FROM waitlist WHERE email = $1;

-- name: WaitlistFind :many
SELECT * FROM waitlist ORDER BY created_at DESC;

-- name: WaitlistCreate :one
INSERT INTO waitlist (email) VALUES ($1) RETURNING *;

-- name: WaitlistDelete :exec
DELETE FROM waitlist WHERE id = $1;

-- name: WaitlistDeleteByEmail :exec
DELETE FROM waitlist WHERE email = $1;
