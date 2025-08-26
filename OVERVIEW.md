# Zeitwork Platform Overview

## Vision

Zeitwork aims to be a seamless, open-source Platform-as-a-Service (PaaS) that democratizes application deployment. From first principles, the platform is built to abstract away infrastructure complexities, enabling developers to focus on code while providing automatic, global, and secure deployments. In its early stages, Zeitwork prioritizes simplicity, transparency, and scalability, allowing any Docker-based application to be deployed with a simple `git push` without managing servers, orchestration, or scaling.

The core philosophy is to combine the ease of serverless platforms with the isolation and performance of microVMs, ensuring zero vendor lock-in through open-source code and self-hosting options.

## Key Goals in Early Stages

- **Zero-Configuration Deployments**: Connect a GitHub repo, push code with a Dockerfile, and get instant global deployments.
- **Global Availability**: Automatic distribution across multiple regions with GeoDNS routing for low-latency access.
- **Security and Isolation**: Use Firecracker microVMs for strong isolation between applications.
- **Open Source Ethos**: Full transparency in how apps are built, deployed, and run, with community-driven development.
- **Developer-Centric**: Target startups and teams needing reliable, scalable hosting without ops overhead.

## High-Level Architecture

Zeitwork is a distributed system for running containerized apps in microVMs:

1. **Operator**: Central control plane managing state in PostgreSQL, handling builds, deployments, and orchestration.
2. **Node Agent**: Runs on compute nodes, manages local Firecracker VMs, and reports health.
3. **Load Balancer**: Distributes traffic with algorithms like round-robin and health checks.
4. **Edge Proxy**: Handles SSL termination, rate limiting, and routing to instances.

### Deployment Flow

1. GitHub push triggers webhook to Operator.
2. Operator builds/optimizes container image for Firecracker.
3. Deploys minimum 3 instances per region (9 globally).
4. Assigns versioned URL; custom domains route via CNAME to edge.
5. Traffic flows through GeoDNS → Load Balancer → Edge Proxy → Node Agent → VM.

## Technology Stack

- **Backend**: Go for services, PostgreSQL for storage (with sqlc for queries).
- **Virtualization**: Firecracker microVMs on KVM-enabled Linux.
- **Database Management**: Drizzle ORM in TypeScript for schema.
- **Frontend**: Nuxt.js/Vue.js web app.
- **CLI**: Go-based tool for setup, deploy, status, etc.
- **Build/Tools**: Makefile for building, systemd for services.

## Current State and Future

In early development, Zeitwork focuses on core deployment functionality, multi-region support, and basic management. Future expansions may include auto-scaling, advanced monitoring, and more integrations, always prioritizing simplicity and openness.
