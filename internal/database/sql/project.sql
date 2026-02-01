-- name: ProjectFirstByID :one
SELECT *
FROM projects
WHERE id = $1
  AND deleted_at IS NULL;
