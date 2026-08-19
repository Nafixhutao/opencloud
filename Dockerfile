# syntax=docker/dockerfile:1
# Multi-stage build for the Next.js dashboard/BFF. Two runnable targets:
#   - runner (default): standalone production server on :3000
#   - auth-migrate: one-shot Better Auth migration (npm run auth:migrate)
# See docs/INFRASTRUCTURE.md §2 and docs/DEPLOYMENT.md §4.

FROM node:22-alpine AS deps
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci

# One-shot Better Auth migration target (Compose `auth-migrate` service).
# Needs full node_modules + TS sources: jiti runs scripts/migrate-auth.ts,
# which drives Better Auth's programmatic migration API (ADR 0006).
FROM deps AS auth-migrate
COPY --chown=node:node tsconfig.json ./
COPY --chown=node:node lib ./lib
COPY --chown=node:node scripts ./scripts
USER node
CMD ["npm", "run", "auth:migrate"]

FROM deps AS build
COPY . .
# Build-time placeholders: lib/auth.ts fails fast when these are unset, and
# `next build` loads that module. Nothing connects during the build (pg.Pool
# is lazy); real values come from the runtime env (docker-compose.yml).
ENV ENV=build MAIL_PROVIDER=log
# Opt-in for the preview container: inlines the react-grab script into the
# production bundle (app/layout.tsx gates on it). Dev-only tooling — leave
# "false" for any real deployment.
ARG NEXT_PUBLIC_REACT_GRAB=false
ENV NEXT_PUBLIC_REACT_GRAB=${NEXT_PUBLIC_REACT_GRAB}
RUN DATABASE_URL=postgres://build:build@localhost:5432/build \
    BETTER_AUTH_SECRET=build-time-placeholder-secret-never-used-at-runtime \
    BETTER_AUTH_URL=http://localhost:3000 \
    npm run build

# Minimal runtime: `output: 'standalone'` (next.config.ts) emits server.js plus
# a pruned node_modules; static assets and public/ must be copied in beside it.
FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production PORT=3000 HOSTNAME=0.0.0.0
COPY --from=build --chown=node:node /src/.next/standalone ./
COPY --from=build --chown=node:node /src/.next/static ./.next/static
COPY --from=build --chown=node:node /src/public ./public

USER node
EXPOSE 3000
CMD ["node", "server.js"]
