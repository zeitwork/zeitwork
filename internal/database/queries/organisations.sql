-- name: OrganisationFindById :one
SELECT * FROM organisations WHERE id = $1;

-- name: OrganisationFindBySlug :one
SELECT * FROM organisations WHERE slug = $1;

-- name: OrganisationFind :many
SELECT * FROM organisations ORDER BY created_at DESC;

-- name: OrganisationCreate :one
INSERT INTO organisations (name, slug) VALUES ($1, $2) RETURNING *;

-- name: OrganisationUpdate :one
UPDATE organisations SET name = $2, slug = $3, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: OrganisationDelete :exec
DELETE FROM organisations WHERE id = $1;

-- name: OrganisationMemberFindById :one
SELECT * FROM organisation_members WHERE id = $1;

-- name: OrganisationMemberFindByOrg :many
SELECT * FROM organisation_members WHERE organisation_id = $1 ORDER BY created_at DESC;

-- name: OrganisationMemberFindByUser :many
SELECT * FROM organisation_members WHERE user_id = $1 ORDER BY created_at DESC;

-- name: OrganisationMemberFindByUserAndOrg :one
SELECT * FROM organisation_members WHERE user_id = $1 AND organisation_id = $2;

-- name: OrganisationMemberCreate :one
INSERT INTO organisation_members (user_id, organisation_id) VALUES ($1, $2) RETURNING *;

-- name: OrganisationMemberDelete :exec
DELETE FROM organisation_members WHERE id = $1;
