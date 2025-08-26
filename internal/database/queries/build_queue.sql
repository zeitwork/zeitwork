-- name: BuildQueueFindById :one
SELECT * FROM build_queue WHERE id = $1;

-- name: BuildQueueFindByProject :many
SELECT * FROM build_queue WHERE project_id = $1 ORDER BY created_at DESC;

-- name: BuildQueueFindByImage :one
SELECT * FROM build_queue WHERE image_id = $1;

-- name: BuildQueueFindPending :many
SELECT * FROM build_queue 
WHERE status = 'pending' 
ORDER BY priority DESC, created_at ASC;

-- name: BuildQueueDequeuePending :one
UPDATE build_queue 
SET status = 'processing', build_started_at = NOW(), updated_at = NOW()
WHERE id = (
    SELECT id FROM build_queue 
    WHERE status = 'pending' 
    ORDER BY priority DESC, created_at ASC 
    LIMIT 1 
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: BuildQueueCreate :one
INSERT INTO build_queue (project_id, image_id, priority, status, github_repo, commit_hash, branch) 
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: BuildQueueUpdateStatus :one
UPDATE build_queue 
SET status = $2, updated_at = NOW() 
WHERE id = $1 RETURNING *;

-- name: BuildQueueComplete :one
UPDATE build_queue 
SET status = 'completed', build_completed_at = NOW(), build_log = $2, updated_at = NOW() 
WHERE id = $1 RETURNING *;

-- name: BuildQueueFail :one
UPDATE build_queue 
SET status = 'failed', build_completed_at = NOW(), build_log = $2, updated_at = NOW() 
WHERE id = $1 RETURNING *;

-- name: BuildQueueDelete :exec
DELETE FROM build_queue WHERE id = $1;
