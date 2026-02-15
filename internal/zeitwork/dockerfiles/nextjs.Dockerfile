FROM node:22-alpine AS base

FROM base AS deps
RUN apk add --no-cache libc6-compat
WORKDIR /app

COPY package.json yarn.lock* package-lock.json* pnpm-lock.yaml* bun.lock* .npmrc* ./
RUN \
  if [ -f bun.lock ]; then npm i -g bun && bun install --frozen-lockfile; \
  elif [ -f pnpm-lock.yaml ]; then corepack enable pnpm && pnpm i --frozen-lockfile; \
  elif [ -f yarn.lock ]; then yarn --frozen-lockfile; \
  elif [ -f package-lock.json ]; then npm ci; \
  else npm i; fi

FROM base AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED=1

# Ensure standalone output is enabled for Docker deployment
RUN config_file=""; \
    for f in next.config.ts next.config.mjs next.config.js; do \
      [ -f "$f" ] && config_file="$f" && break; \
    done; \
    if [ -n "$config_file" ] && ! grep -q "standalone" "$config_file"; then \
      node -e " \
        const fs = require('fs'); \
        let c = fs.readFileSync('$config_file', 'utf8'); \
        c = c.replace(/((?:const|let|var)\s+\w+(?:\s*:\s*\w+)?\s*=\s*\{)/, '\$1\n  output: \"standalone\",'); \
        fs.writeFileSync('$config_file', c); \
      "; \
    fi

RUN \
  if [ -f bun.lock ]; then npm i -g bun && bun run build; \
  elif [ -f pnpm-lock.yaml ]; then corepack enable pnpm && pnpm run build; \
  elif [ -f yarn.lock ]; then yarn run build; \
  else npm run build; fi

FROM base AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000

CMD ["node", "server.js"]
