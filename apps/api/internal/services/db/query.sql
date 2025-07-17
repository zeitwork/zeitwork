-- name: OrganisationInsert :one
insert into organisations (installation_id, slug) values ($1, $2) returning *;

-- name: OrganisationFindByID :one
select * from organisations where id = $1;

-- name: OrganisationFindByInstallationID :one
select * from organisations where installation_id = $1::bigint;

-- name: UserFindByGithubID :one
select * from users where github_id = $1;

-- name: UserFindByID :one
select * from users where id = $1;

-- name: UserInsert :one
insert into users (username, github_id) values ($1, $2) returning *;

-- name: UserInOrganisationInsert :one
insert into user_in_organisation (user_id, organisation_id) VALUES ($1, $2) returning *;

-- name: OrganisationsFindByUserID :many
select o.* from user_in_organisation uio
         inner join public.organisations o on o.id = uio.organisation_id
where uio.user_id = $1;

-- name: OrganisationFindWithUser :one
select o.* from user_in_organisation uio
                    inner join public.organisations o on o.id = uio.organisation_id
where uio.user_id = $1 and o.id=$2;