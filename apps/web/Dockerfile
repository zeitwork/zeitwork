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
FROM base AS install
RUN mkdir -p /temp/dev
COPY package.json /temp/dev/
RUN cd /temp/dev && bun install

# Copy dependencies and source code
FROM base AS prerelease
COPY --from=install /temp/dev/node_modules node_modules
COPY . .

# Build the project
RUN bun run build

# Prepare the final production image
FROM base AS release
COPY --from=install /temp/dev/node_modules node_modules
COPY --from=prerelease /usr/src/app/.output .output
COPY --from=prerelease /usr/src/app/package.json .

# Run the app
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
