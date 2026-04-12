# Backup Orchestrator

Backup orchestration system built on [restic](https://restic.net/). A Go server exposes a REST + gRPC API, a lightweight Go agent runs on each host and executes backups, and a Vue 3 SPA provides the UI.

## Project structure

| Path | Description |
|------|-------------|
| `server/` | REST API (Chi) + gRPC server + SQLite database |
| `agent/` | gRPC client + restic CLI wrapper + cron scheduler |
| `frontend/` | Vue 3 + TypeScript + Vite + Pinia + Tailwind CSS |
| `proto/` | Protobuf definitions, generates Go code into both modules |
| `docs/` | Architecture, API specs, data models |

## Getting started

### Prerequisites

- Go 1.24+
- Node.js 22+
- [just](https://github.com/casey/just) — task runner
- [lefthook](https://github.com/evilmartians/lefthook) — pre-commit hooks
- [buf](https://buf.build/) — protobuf toolchain (only needed to regenerate protos)

### Install pre-commit hooks

```bash
lefthook install
```

> **Note:** If you get an error like `core.hooksPath is set locally to '...'`, run:
> ```bash
> lefthook install --reset-hooks-path
> ```
> This unsets the local `core.hooksPath` config and installs the hooks. See the lefthook [docs](https://github.com/evilmartians/lefthook) for details.

### Common tasks

Run `just` with no arguments to list all available recipes.

```bash
just build          # build server + agent (rebuilds frontend first)
just test           # run all tests (server + agent + frontend)
just fmt            # format all Go code
just vet            # go vet all Go modules
just lint           # golangci-lint on all Go modules
just proto-gen      # regenerate protobuf code
```

## Docker images

Three container images are published:

| Image | Contents | Use when |
|-------|----------|----------|
| `ghcr.io/tryy3/backup-orchestrator-server` | REST + gRPC server with embedded UI | Always — runs the central server |
| `ghcr.io/tryy3/backup-orchestrator-agent` | Agent + restic + rclone | Backing up files/directories — no database tools needed |
| `ghcr.io/tryy3/backup-orchestrator-agent-db` | Agent + restic + rclone + sqlite3 | Hooks require database CLI tools (e.g. `sqlite3 .dump` before a backup) |

Pick the agent image that matches your workload. The `-agent` image is the smallest; use
`-agent-db` when your pre/post-backup hooks need to interact with databases.

```bash
# Build all images locally
just docker-build

# Push multi-arch images (requires docker buildx + ghcr.io login)
just docker-push
```
