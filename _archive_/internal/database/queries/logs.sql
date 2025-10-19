-- name: InsertLogs :copyfrom
INSERT INTO logs (
  id,
  image_build_id,
  instance_id,
  level,
  message,
  logged_at
) VALUES (
  $1, $2, $3, $4, $5, $6
);

-- name: GetLogsByImageBuildId :many
SELECT * FROM logs
WHERE image_build_id = $1
ORDER BY logged_at ASC;

-- name: GetLogsByInstanceId :many
SELECT * FROM logs
WHERE instance_id = $1
ORDER BY logged_at ASC;

-- name: GetLogsByDeploymentId :many
SELECT l.* FROM logs l
LEFT JOIN image_builds ib ON l.image_build_id = ib.id
LEFT JOIN deployments d1 ON ib.id = d1.image_build_id
LEFT JOIN instances i ON l.instance_id = i.id
LEFT JOIN deployment_instances di ON i.id = di.instance_id
LEFT JOIN deployments d2 ON di.deployment_id = d2.id
WHERE d1.id = $1 OR d2.id = $1
ORDER BY l.logged_at ASC;

