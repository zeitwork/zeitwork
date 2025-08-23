-- name: GithubInstallationFindById :one
SELECT * FROM github_installations WHERE id = $1;

-- name: GithubInstallationFindByInstallationId :one
SELECT * FROM github_installations WHERE github_installation_id = $1;

-- name: GithubInstallationFindByOrganisation :many
SELECT * FROM github_installations WHERE organisation_id = $1 ORDER BY created_at DESC;

-- name: GithubInstallationFindByUser :many
SELECT * FROM github_installations WHERE user_id = $1 ORDER BY created_at DESC;

-- name: GithubInstallationFind :many
SELECT * FROM github_installations ORDER BY created_at DESC;

-- name: GithubInstallationCreate :one
INSERT INTO github_installations (id, github_installation_id, github_org_id, organisation_id, user_id) 
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GithubInstallationUpdate :one
UPDATE github_installations SET organisation_id = $2, user_id = $3, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: GithubInstallationDelete :exec
DELETE FROM github_installations WHERE id = $1;
