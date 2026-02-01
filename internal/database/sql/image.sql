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
