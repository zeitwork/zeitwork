-- name: ImageFind :many
select * from images;

-- name: ImageFindByID :one
select * from images where id=$1;

-- name: ImageFindByRepositoryAndTag :one
select *
from images
where registry = $1
  and repository = $2
  and tag = $3
limit 1;

-- name: ImageCreate :one
insert into images (id, registry, repository, tag)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ImageUpdateDiskImage :exec
UPDATE images
SET disk_image_key = $2
WHERE id = $1;

-- name: ImageFindOrCreate :one
WITH ins AS (
    INSERT INTO images (id, registry, repository, tag)
    VALUES ($1, $2, $3, $4)
    ON CONFLICT (registry, repository, tag) DO NOTHING
    RETURNING *
)
SELECT * FROM ins
UNION ALL
SELECT * FROM images
WHERE registry = $2 AND repository = $3 AND tag = $4
  AND NOT EXISTS (SELECT 1 FROM ins)
LIMIT 1;
