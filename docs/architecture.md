zeitwork is a platform as a service

users can connect their github

select a github project

and deploy that

we will try to build it using docker if there is a Dockerfile in the root dir OR if not we try to detect common frameworks (nuxt, nextjs, rails, laravel)

once build we will try to deploy it

we have a service nodeagent that runs on metal servers with kvm support

we run images using firecracker

the nodeagent is also has a reverse proxy

we know what instances run on our node and we know what instances run on other nodes

if the incoming request Host header has a instance on our node, we route to that instance

if not we route to another node, to that reverse proxy, which will then handle the routing to the instance

the edge proxy is a simple go http proxy that routes incoming requests (has a Host header) to the targeted service

cloudflare is used for global geo routing, https and l4 load balancing for customer loads

app.customer.com CNAME edge.zeitwork.com -> l4 cloudflare -> closest region -> nodeagent

for now we just have all services use polling for new state

this is a mvp. polling is fine :)

all state lives in a centralized postgres database

we always assume that servers are running latest ubuntu

the backend should be using a task queue approach that relies on the database. just write tasks to the backend. the backend just will try to do that work one by one

the backend and also all other services shall always be horizontally scalable

the database is not. we have one postgres database. that is okay

we always want to use uuidv7

always use slog or rather or intal shared logging slog

we use sqlc for go database. we manage migrations with drizzle in pacakages/database

use api versioning /v1

we never generate uuids in the database. we always do that in the application level

use libs like

github.com/caarlos0/env/v11
github.com/samber/lo
