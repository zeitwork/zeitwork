-- name: ImageFindByID :one
select * from images where id=$1;