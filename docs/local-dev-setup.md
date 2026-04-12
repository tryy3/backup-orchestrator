# Local Development Setup — Design Plan

**Issue:** [#39](https://github.com/tryy3/backup-orchestrator/issues/39)
**Status:** Planning

---

## Problem

The current workflow to run the project at all is:

```
just pod-start   # builds Docker images and starts containers
just pod-stop
just pod-restart
```

Every code change requires a full Docker image rebuild. This has several consequences:

- **Slow inner loop** — even a one-line Go change triggers a full `docker build`.
- **IDE tooling breakage** — debuggers, `go test`, language servers and hot module replacement all expect to run binaries locally, not inside containers.
- **No frontend HMR in practice** — Vite's hot module replacement is only useful when the dev server is running natively; it does not apply inside a built image.

---

## Current State

| Component | How it runs today | What we want |
|-----------|-------------------|--------------|
| `server` | Docker only | Native binary, hot restart |
| `agent` | Docker only | Native binary, hot restart |
| `frontend` | Docker (static embed) | `npm run dev` with HMR |

### What already works

- **Vite proxy** — `frontend/vite.config.ts` already proxies `/api` and WebSockets to `http://localhost:8080`, so `npm run dev` will work correctly against a locally-running server the moment one exists.
- **Environment variables** — both `server` and `agent` are fully config-driven through env vars with sensible defaults; no hardcoded Docker assumptions.
- **NixOS flake** — `rclone` and `restic` (required by the agent) are already available in the dev shell.

---

## Proposed Solution

### Overview

```
just dev
 ├── dev-server    →  air (Go hot restart)  on :8080 (HTTP) + :8443 (gRPC)
 ├── dev-agent     →  air (Go hot restart)  → connects to localhost:8443
 └── dev-frontend  →  npm run dev (Vite HMR) on :5173  → proxies /api to :8080
```

A single `just dev` target spawns all three in parallel (using `just --parallel` or a process manager — see the [open question](#open-question-process-management) below).

---

### 1. Frontend — `just dev-frontend`

**Complexity: trivial.**

The Vite config already has the `/api` proxy configured. The only missing piece is a `just` target:

```just
# Run frontend dev server with HMR (proxies /api to localhost:8080)
dev-frontend:
    cd frontend && npm run dev
```

No changes to `vite.config.ts` are needed.

---

### 2. Go — Hot Restart

This is the main open question and is discussed in detail in the section below. The short proposal is to use [**air**](https://github.com/air-verse/air) (`github.com/air-verse/air`), the de-facto standard Go hot-restart tool.

**What air does:** watches `.go` source files; on change it runs `go build` and restarts the binary. It is configured via a `.air.toml` file per binary.

**What we need:**

- `server/.air.toml` — watching `server/` source, building without the frontend embed (explained below).
- `agent/.air.toml` — watching `agent/` source.
- `just dev-server` and `just dev-agent` targets that invoke `air`.

---

### 3. Local env defaults — `.env.dev`

Both binaries need env vars to run locally. Rather than requiring developers to export variables manually, we check in a `.env.dev` file with safe defaults and `just` sources it before running.

Proposed `.env.dev` (committed, no secrets):

```dotenv
# server
BACKUP_DB_PATH=./tmp/server.db
BACKUP_HTTP_PORT=8080
BACKUP_GRPC_PORT=8443
# CORS: allow the Vite dev server (defaults to these two when unset)
BACKUP_ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000

# agent
BACKUP_SERVER_URL=localhost:8443
BACKUP_AGENT_NAME=dev-agent
BACKUP_DATA_DIR=./tmp/agent-data
```

`just` can source it with `set dotenv-load` or by using `export $(cat .env.dev)` in the recipe. The `tmp/` directory should be in `.gitignore`.

---

### 4. Docker Compose repositioned

`docker/docker-compose.yml` stays as-is but is reframed as an integration/staging environment. A comment should be added at the top of the file to clarify this, and the `README` should direct developers to `just dev` for daily work.

---

## Decisions

### Go — `air` ✅

Using `air` — de-facto standard Go hot-restart tool. The nixpkgs package exists but is slightly outdated, so `air` is installed via `go install` in the nix `shellHook`. The flake already exports `GOPATH=$PWD/.go` and puts `$GOPATH/bin` on `PATH`, so air lands in `.go/bin/air` and is available after `nix develop`. The install is skipped when air is already present.

### `air` installation: `go install` in shellHook ✅

```nix
shellHook = ''
  export GOPATH="$PWD/.go"
  export PATH="$GOPATH/bin:$PATH"
  if ! command -v air &>/dev/null; then
    echo "Installing air..."
    go install github.com/air-verse/air@latest
  fi
'';
```

---

### Go — Server embed in dev mode ✅

Using build-tag split (`//go:build !dev` / `//go:build dev`). `air` builds the server with `-tags dev`, which compiles `embed_dev.go` instead of `embed.go`. The dev handler redirects any frontend route to the Vite dev server on `:5173`.

### Process management: Zellij layout ✅

`just dev` launches a Zellij layout (`.zellij/dev.kdl`) that opens three tabs — one per process. This gives separate, scrollable, named panes for server, agent, and frontend without requiring tmux. When run inside an existing Zellij session, the layout opens as new tabs in that session.

---

## Proposed `justfile` additions (draft)

```just
# ── Dev ───────────────────────────────────────────────────────────────────────

# Run frontend dev server with Vite HMR (proxy → localhost:8080)
dev-frontend:
    cd frontend && npm run dev

# Run server with air hot restart (no frontend embed, uses .env.dev)
dev-server:
    cd server && air

# Run agent with air hot restart (uses .env.dev)
dev-agent:
    cd agent && air

# Start all dev processes in parallel
dev: dev-server dev-agent dev-frontend   # just --parallel if needed
```

---

## Files to create / modify

| File | Action | Notes |
|------|--------|-------|
| `docs/local-dev-setup.md` | Create | This document |
| `.env.dev` | Create | Default env vars for local dev |
| `.gitignore` | Update | Ignore `tmp/` and `.env.dev.local` |
| `flake.nix` | Update | Add `air` to `buildInputs` (needs research) |
| `server/.air.toml` | Create | air config for server binary |
| `agent/.air.toml` | Create | air config for agent binary |
| `server/internal/frontend/embed_dev.go` | Create | No-op embed for `-tags dev` builds |
| `server/internal/frontend/embed_prod.go` | Rename/tag | Add `//go:build !dev` constraint |
| `justfile` | Update | Add `dev-*` and `dev` targets |
| `docker/docker-compose.yml` | Update | Add comment clarifying staging-only scope |
| `README.md` | Update | "Local Development" section |

---

## Next Steps

1. **Discuss** the Go hot-restart approach (see open questions above) — specifically:
   - Is `air` acceptable, or prefer a different watcher?
   - Add `air` to nix flake vs `go install`?
   - Proceed with the build-tag approach for the frontend embed?
   - Use `just --parallel` for `just dev`, or introduce a Procfile + process manager?
2. Once agreed, implement in this order:
   1. `.env.dev` + `.gitignore` update
   2. `just dev-frontend` (no-risk, purely additive)
   3. `air` configs + `just dev-server` / `just dev-agent`
   4. Build-tag split for the frontend embed
   5. `just dev` combined target
   6. README / CONTRIBUTING update
