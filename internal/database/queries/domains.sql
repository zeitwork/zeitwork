-- name: DomainFindById :one
SELECT * FROM domains WHERE id = $1;

-- name: DomainFindByName :one
SELECT * FROM domains WHERE name = $1;

-- name: DomainFindByOrganisation :many
SELECT * FROM domains WHERE organisation_id = $1 ORDER BY name;

-- name: DomainFindUnverified :many
SELECT * FROM domains WHERE verified_at IS NULL ORDER BY created_at DESC;

-- name: DomainFind :many
SELECT * FROM domains ORDER BY name;

-- name: DomainCreate :one
INSERT INTO domains (name, verification_token, organisation_id) VALUES ($1, $2, $3) RETURNING *;

-- name: DomainVerify :one
UPDATE domains SET verified_at = NOW(), updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: DomainUpdateToken :one
UPDATE domains SET verification_token = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: DomainDelete :exec
DELETE FROM domains WHERE id = $1;

-- name: DomainRecordFindById :one
SELECT * FROM domain_records WHERE id = $1;

-- name: DomainRecordFindByDomain :many
SELECT * FROM domain_records WHERE domain_id = $1 ORDER BY type, name;

-- name: DomainRecordFindByType :many
SELECT * FROM domain_records WHERE domain_id = $1 AND type = $2 ORDER BY name;

-- name: DomainRecordCreate :one
INSERT INTO domain_records (domain_id, type, name, content, ttl, organisation_id) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: DomainRecordUpdate :one
UPDATE domain_records SET content = $2, ttl = $3, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: DomainRecordDelete :exec
DELETE FROM domain_records WHERE id = $1;

-- name: ProjectDomainFindById :one
SELECT * FROM project_domains WHERE id = $1;

-- name: ProjectDomainFindByProject :many
SELECT * FROM project_domains WHERE project_id = $1 ORDER BY created_at DESC;

-- name: ProjectDomainFindByDomain :many
SELECT * FROM project_domains WHERE domain_id = $1 ORDER BY created_at DESC;

-- name: ProjectDomainCreate :one
INSERT INTO project_domains (project_id, domain_id, organisation_id) VALUES ($1, $2, $3) RETURNING *;

-- name: ProjectDomainDelete :exec
DELETE FROM project_domains WHERE id = $1;
