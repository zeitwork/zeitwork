# Use the official Node.js image
# See available tags at https://hub.docker.com/r/oven/bun/tags
FROM oven/bun:1-alpine AS base

# Install Node.js and npm
RUN apk add --no-cache nodejs npm

WORKDIR /usr/src/app

# Install dependencies in a separate layer for better caching
FROM base AS install
RUN mkdir -p /temp/dev
COPY package.json bun.lock /temp/dev/
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
COPY --from=prerelease /usr/src/app/bun.lock .

# Run the app
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
