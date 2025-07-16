drop table if exists organisations, users, user_in_organisation;
create table organisations
(
    id              serial primary key,
    installation_id bigint null unique,
    slug text not null unique
);

create table users
(
    id serial primary key,
    username text not null unique,
    github_id bigint not null unique
);

create table user_in_organisation
(
    user_id int,
    organisation_id int,
    primary key (user_id, organisation_id),
    foreign key (user_id) references users(id),
    foreign key (organisation_id) references organisations(id)
);