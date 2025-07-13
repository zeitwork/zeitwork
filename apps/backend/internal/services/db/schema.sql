drop table if exists organisations;
create table organisations
(
    id              serial primary key,
    installation_id bigint not null unique,
    github_username text   not null unique
);