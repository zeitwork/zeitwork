-- name: EnvironmentVariableFindByProjectID :many
SELECT name, value FROM environment_variables 
WHERE project_id = $1 AND deleted_at IS NULL;
