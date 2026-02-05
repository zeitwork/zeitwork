-- name: GithubInstallationFindByID :one
SELECT *
FROM github_installations
WHERE id = $1
  AND deleted_at IS NULL;
