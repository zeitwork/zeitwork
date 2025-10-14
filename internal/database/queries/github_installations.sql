-- name: GetGithubInstallation :one
SELECT 
    id,
    user_id,
    github_account_id,
    github_installation_id,
    organisation_id,
    created_at,
    updated_at
FROM github_installations
WHERE id = $1
LIMIT 1;

