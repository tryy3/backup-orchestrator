# Backup Orchestrator

Backup orchestration system built on [restic](https://restic.net/). A central Go server manages configuration and history; lightweight Go agents run on each host and execute backups autonomously. A Vue 3 SPA provides the management UI.

[![Go version](https://img.shields.io/github/go-mod/go-version/tryy3/backup-orchestrator/main?filename=server%2Fgo.mod&logo=go&logoColor=white)](https://go.dev/)
[![CI](https://github.com/tryy3/backup-orchestrator/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/tryy3/backup-orchestrator/actions/workflows/ci.yml)
[![Build and Push](https://github.com/tryy3/backup-orchestrator/actions/workflows/build-push.yml/badge.svg?branch=main)](https://github.com/tryy3/backup-orchestrator/actions/workflows/build-push.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## Features

**Backup engine**
- [restic](https://restic.net/)-based backups to one or more repositories per plan
- rclone integration for cloud and remote backends (config pushed from server)
- Retention policies — global defaults with per-plan overrides (all restic `keep-*` flags)
- Auto-forget after backup; optional separate prune schedule
- Automatic snapshot tagging (`agent:`, `plan:`, `trigger:`)

**Hooks & scripts**
- Pre/post lifecycle hooks on four events: `pre_backup`, `post_backup`, `on_success`, `on_failure`
- Hooks can be inline shell commands or references to a reusable server-side Script library
- Template variables available in scripts (`{{.PlanName}}`, `{{.Status}}`, `{{.SnapshotID}}`, …)

**Agent autonomy**
- Agents connect *outbound* to the server — no inbound ports required on agent hosts
- Cron scheduler runs on the agent, independent of server availability
- Agents operate on their last-known local config when the server is unreachable
- Job reports are buffered in an on-agent SQLite database and replayed on reconnect

**UI**
- Dashboard with fleet-wide agent status and recent job history
- Full plan management — paths, schedule, retention, hooks
- Snapshot browser and restore trigger
- Reusable script library

---

## Architecture

```
Browser ──► Server :8080 (HTTP / REST API + embedded Vue SPA)
                │
               :8443 (gRPC — agents connect outbound)
                │
         ┌──────┴──────┐
       Agent          Agent  (one per host)
      + restic        + restic
      + rclone        + rclone
```

- **Server** — single Go binary: REST API (Chi), gRPC server, SQLite, embedded Vue SPA. The only host that needs an open inbound port.
- **Agent** — Go binary on each backup host: wraps the restic/rclone CLIs, runs a cron scheduler, streams results back to server.
- Config and credentials are pushed server → agent over a persistent bidirectional gRPC stream. Agents never hold credentials until approved.

---

## Quick Start (Docker)

### 1. Create a `docker-compose.yml`

```yaml
services:
  server:
    image: ghcr.io/tryy3/backup-orchestrator-server:latest
    ports:
      - "8080:8080"   # Web UI + REST API
      - "8443:8443"   # gRPC (agents connect here)
    volumes:
      - server-data:/data
    environment:
      BACKUP_DB_PATH: /data/server.db

  agent:
    image: ghcr.io/tryy3/backup-orchestrator-agent:latest
    depends_on: [server]
    volumes:
      - agent-data:/data
      - /path/to/backup:/mnt/backup:ro   # mount the paths you want to back up
    environment:
      BACKUP_SERVER_URL: server:8443
      BACKUP_AGENT_NAME: my-agent
      BACKUP_DATA_DIR: /data

volumes:
  server-data:
  agent-data:
```

> Use `ghcr.io/tryy3/backup-orchestrator-agent-db` instead if your hook scripts need database CLI tools (e.g. `sqlite3`).

### 2. Start the stack

```bash
docker compose up -d
```

### 3. Approve the agent

1. Open **http://localhost:8080** in your browser.
2. The agent appears on the dashboard as **Pending**.
3. Click **Approve** — the agent receives its API key and begins accepting config.

### 4. Create your first backup plan

In the UI: **Plans → New Plan** — choose paths, target repositories, a cron schedule, and (optionally) retention rules and hooks.

---

## Manual / Binary Install

### Prerequisites

- Go 1.26+
- Node.js 22+
- [just](https://github.com/casey/just) — task runner

### Build from source

```bash
git clone https://github.com/tryy3/backup-orchestrator.git
cd backup-orchestrator
just build
```

Outputs: `bin/server` and `bin/agent` (no CGO required).

### Run

```bash
# Server
BACKUP_DB_PATH=/var/lib/backup-orchestrator/server.db ./bin/server

# Agent (on the host to back up)
BACKUP_SERVER_URL=<server-host>:8443 ./bin/agent
```

---

## Configuration Reference

### Server

| Variable | Default | Description |
|---|---|---|
| `BACKUP_DB_PATH` | `/var/lib/backup-orchestrator/server.db` | SQLite database path |
| `BACKUP_HTTP_PORT` | `8080` | HTTP port — serves the web UI and REST API |
| `BACKUP_GRPC_PORT` | `8443` | gRPC port — agents connect here |
| `BACKUP_ALLOWED_ORIGINS` | `http://localhost:5173,http://localhost:3000` | Comma-separated CORS allowed origins |
| `BACKUP_ENCRYPTION_KEY` | *(auto-generated)* | 64-char hex AES-256 key for secrets at rest |

**Encryption key resolution order:** `BACKUP_ENCRYPTION_KEY` env var → `encryption.key` file next to the database → auto-generate and persist.

> **Production note:** Set `BACKUP_ENCRYPTION_KEY` explicitly or back up the auto-generated `encryption.key` file. Losing it makes stored repository credentials unrecoverable.

### Agent

| Variable | Default | Required | Description |
|---|---|:---:|---|
| `BACKUP_SERVER_URL` | — | ✓ | Server address, e.g. `backup-server:8443` |
| `BACKUP_AGENT_NAME` | *(hostname)* | | Agent display name shown in the UI |
| `BACKUP_DATA_DIR` | `/var/lib/backup-orchestrator` | | Directory for agent identity, local config, and job DB |

---

## UI Overview

> Screenshots coming soon.

| Page | Purpose |
|---|---|
| **Dashboard** | Fleet overview — agent status cards, recent job history |
| **Agents** | Agent list; per-agent detail with rclone config editor |
| **Plans** | Backup plan list; create/edit paths, schedule, retention, and hooks |
| **Repositories** | Repository definitions; scope: `local` (one agent) or `global` (any agent) |
| **Scripts** | Reusable hook script library with template variable support |
| **Jobs** | Job history with per-repository and per-hook result breakdown |
| **Snapshots** | Browse snapshot contents and trigger restores |
| **Settings** | Global retention policy defaults |

---

## Docker Images

| Image | Contents | Use when |
|---|---|---|
| `ghcr.io/tryy3/backup-orchestrator-server` | REST + gRPC server with embedded UI | Always — runs the central server |
| `ghcr.io/tryy3/backup-orchestrator-agent` | Agent + restic + rclone | Backing up files/directories |
| `ghcr.io/tryy3/backup-orchestrator-agent-db` | Agent + restic + rclone + sqlite3 | Hook scripts need database CLI tools |

```bash
just docker-build   # build all images locally
just docker-push    # push multi-arch images to ghcr.io (prompts for confirmation)
```

---

## Project Structure

| Path | Description |
|---|---|
| `server/` | REST API (Chi) + gRPC server + SQLite database |
| `agent/` | gRPC client + restic CLI wrapper + cron scheduler |
| `frontend/` | Vue 3 + TypeScript + Vite + Pinia + Tailwind CSS |
| `proto/` | Protobuf definitions, generates Go code into both modules |
| `docs/` | Architecture, API specs, data models |

---

## Development

Run `just` with no arguments to list all available recipes.

```bash
just build          # build server + agent (rebuilds frontend first)
just test           # run all tests (server + agent + frontend)
just fmt            # format all Go code
just vet            # go vet all Go modules
just lint           # golangci-lint on all Go modules
just proto-gen      # regenerate protobuf code
```

**Pre-commit hooks** — install with [lefthook](https://github.com/evilmartians/lefthook):

```bash
lefthook install
```

If you see `core.hooksPath is set locally`, run `lefthook install --reset-hooks-path` instead.

---

## License

[MIT](LICENSE)
