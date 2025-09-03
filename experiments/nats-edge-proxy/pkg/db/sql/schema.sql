drop table if exists http_proxies, http_proxy_endpoints cascade ;

create table http_proxies (
    id uuid primary key default gen_random_uuid(),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    deleted_at timestamptz null,

    fqdn text not null unique
);

create table http_proxy_endpoints (
    id uuid primary key default gen_random_uuid(),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    deleted_at timestamptz null,

    http_proxy_id uuid not null references http_proxies(id),
    endpoint text not null,
    healthy bool not null default false,

    unique (http_proxy_id, endpoint)
);

insert into http_proxies (fqdn) values ('app.example.com');

insert into http_proxy_endpoints (http_proxy_id, endpoint, healthy)
select (select id from http_proxies limit 1), 'http://127.0.0.1:1234', true;
insert into http_proxy_endpoints (http_proxy_id, endpoint, healthy)
select (select id from http_proxies limit 1), 'http://127.0.0.1:1235', false;
insert into http_proxy_endpoints (http_proxy_id, endpoint, healthy)
select (select id from http_proxies limit 1), 'http://127.0.0.1:1236', true;

