# Dependencies stage - use Bun for workspace installation
FROM oven/bun:1.3-alpine AS deps

WORKDIR /usr/src/app

# Copy all files (node_modules excluded by .dockerignore)
COPY . .

# Install all workspace dependencies with Bun
RUN bun install

# Build stage - use Node.js for building
FROM node:alpine AS build

WORKDIR /usr/src/app

# Copy source and installed dependencies
COPY --from=deps /usr/src/app .

# Run postinstall and build with Node
RUN cd apps/web && npm run postinstall
RUN cd apps/web && npm run build

# Production stage - use Node.js for runtime
FROM node:alpine AS release

WORKDIR /usr/src/app

# Copy built application
COPY --from=build /usr/src/app/apps/web/.output ./.output

# Run the app
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
