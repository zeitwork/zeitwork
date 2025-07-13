-- name: OrganisationInsert :one
insert into organisations ( installation_id, github_username) values ($1, $2) returning *;

-- name: OrganisationFindByID :one
select * from organisations where id = $1;

-- name: OrganisationFindByInstallationID :one
select * from organisations where installation_id = $1;