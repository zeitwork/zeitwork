-- name: ProjectFindById :one
SELECT * FROM projects WHERE id = $1;

-- name: ProjectFindBySlug :one
SELECT * FROM projects WHERE slug = $1;

-- name: ProjectFindByOrganisation :many
SELECT * FROM projects WHERE organisation_id = $1 ORDER BY created_at DESC;

-- name: ProjectFind :many
SELECT * FROM projects ORDER BY created_at DESC;

-- name: ProjectCreate :one
INSERT INTO projects (name, slug, organisation_id) VALUES ($1, $2, $3) RETURNING *;

-- name: ProjectUpdate :one
UPDATE projects SET name = $2, slug = $3, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: ProjectDelete :exec
DELETE FROM projects WHERE id = $1;

-- name: ProjectEnvironmentFindById :one
SELECT * FROM project_environments WHERE id = $1;

-- name: ProjectEnvironmentFindByProject :many
SELECT * FROM project_environments WHERE project_id = $1 ORDER BY created_at DESC;

-- name: ProjectEnvironmentFindByName :one
SELECT * FROM project_environments WHERE project_id = $1 AND name = $2;

-- name: ProjectEnvironmentCreate :one
INSERT INTO project_environments (project_id, name, organisation_id) VALUES ($1, $2, $3) RETURNING *;

-- name: ProjectEnvironmentUpdate :one
UPDATE project_environments SET name = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: ProjectEnvironmentDelete :exec
DELETE FROM project_environments WHERE id = $1;

-- name: ProjectSecretFindById :one
SELECT * FROM project_secrets WHERE id = $1;

-- name: ProjectSecretFindByProject :many
SELECT * FROM project_secrets WHERE project_id = $1 ORDER BY name;

-- name: ProjectSecretFindByName :one
SELECT * FROM project_secrets WHERE project_id = $1 AND name = $2;

-- name: ProjectSecretCreate :one
INSERT INTO project_secrets (project_id, name, value, organisation_id) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ProjectSecretUpdate :one
UPDATE project_secrets SET value = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: ProjectSecretDelete :exec
DELETE FROM project_secrets WHERE id = $1;
