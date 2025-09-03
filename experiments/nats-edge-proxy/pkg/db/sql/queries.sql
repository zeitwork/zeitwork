-- name: HttpProxyFindByID :one
select * from http_proxies where id = $1;

-- name: HttpProxyFindAllIDs :many
select id from http_proxies;

-- name: HttpProxyFindAll :many
select * from http_proxies;

-- name: HttpProxyEndpointFindByID :one
select * from http_proxy_endpoints where id = $1 and deleted_at is null;

-- name: HttpProxyEndpointFindByProxyId :many
select * from http_proxy_endpoints where http_proxy_id = $1 and deleted_at is null;

-- name: HttpProxyEndpointUpsert :one
insert into http_proxy_endpoints (http_proxy_id, endpoint, healthy) values ($1, $2, $3)
on conflict on constraint http_proxy_endpoints_http_proxy_id_endpoint_key do update SET http_proxy_id = $1, endpoint = $2, healthy = $3
returning *;

-- name: HttpProxyEndpointDelete :one
update http_proxy_endpoints set deleted_at=now() where id = $1 returning *;