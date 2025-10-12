ARG NODE_VERSION=23-alpine

# Use the official Node.js image for the final stage
FROM node:${NODE_VERSION} AS node-base

# Build stage using Bun
FROM oven/bun:1.3-alpine AS build

WORKDIR /usr/src/app

# Copy workspace configuration and package files
COPY package.json ./
COPY bun.lock ./
COPY apps/web/package.json ./apps/web/package.json
COPY packages/database/package.json ./packages/database/package.json

# Install all dependencies at workspace root (Bun 1.3 handles workspaces)
RUN bun install --ignore-scripts

# Copy source code (node_modules excluded by .dockerignore)
COPY apps/web ./apps/web
COPY packages/database ./packages/database

# Build the Nuxt application
RUN bun --cwd ./apps/web run postinstall
RUN bun --cwd ./apps/web build

# Production stage with Node.js
FROM node-base AS release

WORKDIR /usr/src/app

# Copy built application
COPY --from=build /usr/src/app/apps/web/.output ./.output

# Run the app
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]