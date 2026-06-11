# infra

Deployment configs for projects in this monorepo, running on the home server (`.72`).

- `docker/` — Docker Compose files / Dockerfiles for the apps here.
- `k3s/` — Kubernetes manifests for k3s.

This directory is **not** a pnpm workspace member (no `package.json`); it holds plain
config files only.

> Note: the unrelated `/media/plex-drive/docker/` and `/media/plex-drive/k3s/` on this
> machine are legacy from the old RPi4 server (`.159`). Don't confuse them with this folder.
