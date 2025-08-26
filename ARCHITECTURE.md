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

app.dokedu.org CNAME edge.zeitwork-dns.com

using geodns we route to the closest region

eu-central-1

requests hits operator node with L4 load balancer

that load balancer sends traffic to a L7 edge proxy on the operator node

the L7 edge proxy does tls termination

edge proxy load balances traffic to a node where the application is running

that is determined by the domain ie app.dokedu.org

as applications are running on multiple nodes

we route to one of the nodes to the specific microvm

each microvm has a ipv6

the traffic from an edge proxy to the worker-node with the firecracker vms need to be encrypted

a worker node can run a node-proxy that accepts traffic and routes it to the right microvm

microvms shouldnt be able to see/interact with traffic from other vms

---

tech considerations

- we write more or less everything in go besides the web apps that are written using nuxt/typescript
- instead of inlining bash scripts in code we store them as bash script files and embed them into the go binary

---

public api

api.zeitwork.com -> CNAME edge.zeitwork-dns.com -> eu-central-1 -> ipv4 operator node 1 -> load balancer L4 -> edge proxy L7

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
PUT    /v1/deployments/{id}/status
```

image builder (internal)

```
// Image management
GET    /v1/images
POST   /v1/images
GET    /v1/images/{id}
GET    /v1/images/{id}/download
DELETE /v1/images/{id}
```

operator api (internal)

NODE_ID=""
NODE_IP=""
NODE_TYPE="operator"
NODE_REGION="eu-central-1"
DATABASE_URL=""

```
// Node management
GET    /v1/nodes
POST   /v1/nodes
GET    /v1/nodes/{id}
DELETE /v1/nodes/{id}
PUT    /v1/nodes/{id}/state

// Instance management
GET    /v1/instances
POST   /v1/instances
GET    /v1/instances/{id}
PUT    /v1/instances/{id}/state
DELETE /v1/instances/{id}
```

node agent api (internal)

NODE_ID=""
NODE_IP="10.0.2.42"
NODE_JWT=""
NODE_TYPE="worker"
NODE_REGION="eu-central-1"
OPERATOR_URL="eu-central-1.zeitwork-dns.com" (for all the nodes in that datacenter)

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
