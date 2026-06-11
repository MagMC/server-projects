# syntax=docker/dockerfile:1
# Build context is the repo root (see infra/docker/docker-compose.yml) so the
# pnpm workspace can resolve. Multi-stage: build static assets, then serve via nginx.

FROM node:20-alpine AS build
WORKDIR /repo
RUN corepack enable

# Copy only manifests + lockfile first for better layer caching on dep installs.
COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY apps/dashboard/package.json apps/dashboard/
RUN pnpm install --frozen-lockfile --filter dashboard...

# Build the dashboard.
COPY apps/dashboard apps/dashboard
RUN pnpm --filter dashboard build

FROM nginx:alpine AS runtime
COPY apps/dashboard/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/dashboard/dist /usr/share/nginx/html
EXPOSE 80
