# API Service Dockerfile
# Background worker service for deployment reconciliation

FROM oven/bun:1.3-alpine AS base

WORKDIR /usr/src/app

# Install dependencies in a separate layer for better caching
FROM base AS deps
WORKDIR /usr/src/app
COPY packages/database/package.json packages/database/package.json
COPY apps/api/package.json apps/api/package.json
RUN cd packages/database && bun install
RUN cd apps/api && bun install

# Copy dependencies and source code
FROM base AS release
WORKDIR /usr/src/app
COPY --from=deps /usr/src/app/packages/database/node_modules ./packages/database/node_modules
COPY --from=deps /usr/src/app/apps/api/node_modules ./apps/api/node_modules
COPY packages/database ./packages/database
COPY apps/api ./apps/api

# Create non-root user
RUN addgroup -S zeitwork && adduser -S zeitwork -G zeitwork && \
    chown -R zeitwork:zeitwork /usr/src/app

# Switch to non-root user
USER zeitwork

# Health check - checks if the process is running
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ps aux | grep -v grep | grep "bun" || exit 1

# Run the API service
WORKDIR /usr/src/app/apps/api
CMD ["bun", "run", "start"]
