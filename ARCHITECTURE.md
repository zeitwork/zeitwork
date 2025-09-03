# how to bootstrap a new zeitwork platform

the bootstrap script also needs to recover an installation, ie if the database already has data, try to recover to that state

requirements

- dns entries
- s3 bucket
- postgres db

steps

1.run db migrations

2. create a new region in the database

3. install binaries

3 operator nodes running:

- edge-proxy
- active-notifier
- management-api

3 or more worker nodes running:

- node-agent

## edge proxy

either envoy or traefik proxy

sticky sessions for e.g. websocket support

## active notifier

using e.g. NATS we notify listeners eg. edge proxies and node agents about relevant changes

## management api

provides an api

## node agent

on first install needs to check if kvm is properly supported and configured, same goes for firecracker vm spawning (needs images etc)

connects to a management api to pull desired state. aims to turn into that state

all changes and stats are pushed to the management api

===

## user flow

1. signs up with github
2. creates a new project and connects a github repo to it
3. we auto build and deploy the application using the dockerfile in the project
4. we spin up at least 3 microvms for each deployment
5. we wait 5 minutes before shutting down the old deployment instances

===

## request flow

user requests app.dokedu.org

app.dokedu.org CNAME edge.zeitwork.com

using geodns we route to the closest region

eu-central-1

points to a hetzner L4 load balancer ip

hetzner load balancer balances to L7 edge proxy

the L7 edge proxy does tls termination

edge proxy load balances traffic to a node where the application is running

that is determined by the domain ie app.dokedu.org

we have a map of domain -> deployments -> list of vms running on nodes

as applications are running on multiple nodes

we route to one of the nodes to the specific microvm

each microvm has a ipv6

a worker node can run a node-proxy that accepts traffic and routes it to the right microvm

microvms shouldnt be able to see/interact with traffic from other vms

---

public api

```
ANY    /v1/auth/github

POST   /v1/projects
GET    /v1/projects
GET    /v1/projects/{id}

POST   /v1/waitlist

POST   /v1/webhook/github

// Deployment management
GET    /v1/deployments
POST   /v1/deployments
GET    /v1/deployments/{id}
GET    /v1/deployments/{id}/logs
PUT    /v1/deployments/{id}/status
```

management api

```
## instances

GET    /v1/instances
POST   /v1/instances
GET    /v1/instances/{id}
PUT    /v1/instances/{id}/state
DELETE /v1/instances/{id}


## image builder

GET    /v1/images
POST   /v1/images
GET    /v1/images/{id}
GET    /v1/images/{id}/download
DELETE /v1/images/{id}
```

node agent

```
// Health check
GET    /v1/health

// Instance management endpoints
POST   /v1/instances
GET    /v1/instances/{id}
PUT    /v1/instances/{id}/state
DELETE /v1/instances/{id}

// Node information
GET    /v1/node/info
GET    /v1/node/resources
```
