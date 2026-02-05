ARG NODE_VERSION=23-alpine

# Use the official Node.js image
# See available tags at https://hub.docker.com/_/node
FROM node:${NODE_VERSION} AS base

WORKDIR /usr/src/app

# Install Bun
RUN apk update && apk add --no-cache bash curl unzip
RUN curl https://bun.sh/install | bash -s

ENV PATH="${PATH}:/root/.bun/bin"

# Install dependencies preserving workspace structure
FROM base AS install
RUN mkdir -p /temp/dev/apps/web /temp/dev/packages/database

# Copy all package.json files to preserve workspace structure
COPY package.json bun.lock* /temp/dev/
COPY apps/web/package.json /temp/dev/apps/web/
COPY packages/database/package.json /temp/dev/packages/database/

WORKDIR /temp/dev
RUN bun install --linker hoisted

# Build stage
FROM base AS prerelease
WORKDIR /usr/src/app

# Copy source code first
COPY . .

# Copy installed node_modules from install stage (hoisted to root)
COPY --from=install /temp/dev/node_modules node_modules

# Build the web app using Node (Bun has memory issues with large builds)
WORKDIR /usr/src/app/apps/web
RUN npx nuxt build

# Production stage
FROM base AS release
WORKDIR /usr/src/app

COPY --from=prerelease /usr/src/app/apps/web/.output .output
COPY --from=prerelease /usr/src/app/apps/web/package.json .

# Run the app
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
