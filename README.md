# server-projects

Monorepo for projects running on the home server (`.72`).

## Layout

| Path        | What lives here                                                        |
| ----------- | ---------------------------------------------------------------------- |
| `apps/`     | Runnable applications. Each is a pnpm workspace package.               |
| `packages/` | Shared TypeScript libraries used by apps (added when first needed).    |
| `infra/`    | Docker & k3s deployment configs. Not a pnpm workspace member.          |

## Apps

- **`apps/dashboard`** — server monitoring dashboard for iPad (Vite + React + TS).

## Toolchain

- Node `>=20` (see `.node-version`)
- pnpm `>=9` (workspaces)

## Getting started

```sh
pnpm install          # install all workspace deps
pnpm dev              # run the dashboard dev server
pnpm build            # build the dashboard
```

Run a specific package directly:

```sh
pnpm --filter dashboard dev
```

## Adding a new app

```sh
mkdir apps/<name> && cd apps/<name>
# scaffold (e.g. pnpm create vite .), set "name" in package.json
```

It becomes a workspace member automatically — no root config change needed.
