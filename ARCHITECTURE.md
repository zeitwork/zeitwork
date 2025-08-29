how to bootstrap a new zeitwork platform

the bootstrap script also needs to recover an installation, ie if the database already has data, try to recover to that state

- provide a few domains ie \*.zeitwork.com, \*.zeitwork.app and \*.zeitwork-dns.com
- generate the ssl certs for all the dns entries
- set up a s3 bucket using minio or provide an existing s3 bucket
- set up a new postgres database on planetscale or provide existing one
- run the migrations against that database
- generate a ssh key or use existing one
- create 6 nodes with that ssh key having root access
- run the setup script providing the database credentials, the ssh key, and the ips of the nodes, and the s3 bucket credentials
- install the operator binary on 3 of these nodes, they become operators and they self register
- install the node-agent binary on 3 of these nodes, they become worker nodes and self register
- we then spin up a image builder. it is responsible for building code with docker inside of a firecracker vm and exports it as a firecracker vm
- if we dont have a firecracker vm image for building with docker we need to set that up
- we store firecracker vm images inside of a s3 bucket

---

user flow

1. signs up with github
2. creates a new project and connects a github repo to it
3. we auto build and deploy the application using the dockerfile in the project
4. we spin up at least 3 microvms for each deployment
5. we wait 5 minutes before shutting down the old deployment instances

---

request flow

app.dokedu.org CNAME edge.zeitwork.com

using geodns we route to the closest region

eu-central-1

points to a hetzner L4 load balancer

hetzner load balancer balances to L7 edge proxy that is envoy

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
