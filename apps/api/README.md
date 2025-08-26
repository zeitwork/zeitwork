public api

api.zeitwork.com

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

image builder

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
OPERATOR_URL="eu-central-1.zeitwork-dns.com:8080" (for all the nodes in that datacenter)

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
