-- name: RegionFindById :one
SELECT * FROM regions WHERE id = $1;

-- name: RegionFindByCode :one
SELECT * FROM regions WHERE code = $1;

-- name: RegionFind :many
SELECT * FROM regions ORDER BY name;

-- name: RegionFindByCountry :many
SELECT * FROM regions WHERE country = $1 ORDER BY name;

-- name: RegionCreate :one
INSERT INTO regions (name, code, country) VALUES ($1, $2, $3) RETURNING *;

-- name: RegionUpdate :one
UPDATE regions SET name = $2, code = $3, country = $4, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: RegionDelete :exec
DELETE FROM regions WHERE id = $1;
