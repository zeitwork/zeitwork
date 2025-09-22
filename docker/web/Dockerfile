ARG NODE_VERSION=23-alpine
ARG BUN_VERSION=1.2.19

# Use the official Node.js image
# See available tags at https://hub.docker.com/_/node
FROM node:${NODE_VERSION} AS base

WORKDIR /usr/src/app

# Install Bun in the specified version
RUN apk update && apk add --no-cache bash curl unzip
RUN curl https://bun.sh/install | bash -s

ENV PATH="${PATH}:/root/.bun/bin"

# Install dependencies in a separate layer for better caching
FROM base AS deps
WORKDIR /usr/src/app
COPY bun.lock package.json ./
COPY apps/web/package.json apps/web/package.json
COPY packages/database/package.json packages/database/package.json
RUN bun install --ignore-scripts

# Copy dependencies and source code
FROM base AS build
WORKDIR /usr/src/app
COPY --from=deps /usr/src/app/node_modules ./node_modules
COPY apps/web ./apps/web
COPY packages/database ./packages/database
# Build the project inside the workspace
RUN bun --cwd ./apps/web run postinstall
RUN bun --cwd ./apps/web build

# Prepare the final production image
FROM base AS release
WORKDIR /usr/src/app
COPY --from=build /usr/src/app/apps/web/.output ./.output

# Run the app
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
