# API Service Dockerfile
# Background worker service for deployment reconciliation

FROM oven/bun:1.3-alpine

WORKDIR /usr/src/app

# Copy workspace configuration and package files
COPY package.json ./
COPY bun.lock ./
COPY packages/database/package.json ./packages/database/package.json
COPY apps/api/package.json ./apps/api/package.json

# Install all dependencies at workspace root (Bun 1.3 handles workspaces)
RUN bun install

# Copy source code (node_modules excluded by .dockerignore)
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
