-- name: DeploymentLogCreate :exec
INSERT INTO deployment_logs (id, deployment_id, message, level, organisation_id, created_at)
VALUES ($1, $2, $3, $4, $5, NOW());
