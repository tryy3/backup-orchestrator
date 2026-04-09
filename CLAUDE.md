# Backup Orchestrator — Project Conventions

## Overview
Backup orchestration system using restic. Go server + agent communicating via gRPC, Vue.js SPA frontend.

## Project Structure
- `server/` — Go module: REST API (Chi) + gRPC server + SQLite database
- `agent/` — Go module: gRPC client + restic CLI wrapper + cron scheduler
- `frontend/` — Vue 3 + TypeScript + Vite + Pinia + Tailwind CSS
- `proto/` — Protobuf definitions, generates into both Go modules
- `docs/` — Design documentation (architecture, API specs, data models)

## Task Runner (just)

The project uses [just](https://github.com/casey/just) as the task runner. Run `just` with no arguments to list all available recipes.

```bash
# Tests
just test              # run all tests (server + agent + frontend)
just test-server       # go test -race ./... in server/
just test-agent        # go test -race ./... in agent/
just test-frontend     # vitest in frontend/
just test-cover        # tests + HTML coverage reports

# Formatting & vet
just fmt               # gofmt -l -w on all Go modules
just vet               # go vet ./... on all Go modules

# Linting (requires golangci-lint)
just lint              # golangci-lint run on all Go modules

# Build
just build             # build server + agent binaries (frontend first)
just build-server      # build server (rebuilds frontend)
just build-server-only # build server without rebuilding frontend
just build-agent       # build agent binary
just build-frontend    # build frontend (copies dist into server)

# Proto
just proto-gen         # regenerate proto files (buf generate)
just proto-lint        # lint proto files (buf lint)
just proto-breaking    # check for breaking changes against main

# Cleanup
just clean             # remove build artefacts

# Docker
just docker-build      # build images locally
just docker-push       # push multi-arch images to ghcr.io (prompts for confirmation)
just docker-login github-user=<user>  # log in to ghcr.io
```

## Pre-commit Hooks (lefthook)

The project uses [lefthook](https://github.com/evilmartians/lefthook) for fast pre-commit hooks.

```bash
lefthook install   # register hooks after first clone
```

Hooks run `gofmt` (format check) and `go vet` on staged Go files. They are intentionally lightweight — heavy checks live in CI.

## Go Conventions
- **No ORM** — raw SQL with `database/sql`, methods on `*DB` receiver
- **SQLite driver** — `modernc.org/sqlite` (pure Go, no CGO)
- **HTTP router** — `go-chi/chi/v5`
- **Error handling** — return errors, don't panic. Wrap with `fmt.Errorf("context: %w", err)`
- **UUIDs** — `github.com/google/uuid` for all entity IDs
- **JSON fields** — stored as TEXT in SQLite, marshal/unmarshal in Go code
- **Naming** — standard Go: exported PascalCase, unexported camelCase
- **Package structure** — `internal/` for all non-main packages

## Frontend Conventions
- **State management** — Pinia stores in `src/stores/`
- **API calls** — all through `src/api/client.ts`, never direct fetch in components
- **Types** — shared API types in `src/types/api.ts`
- **Components** — SFC with `<script setup lang="ts">`
- **Styling** — Tailwind CSS utility classes

## Key Design Decisions
- Agent connects outbound to server (server is the only open port)
- Server pushes config to agents over gRPC bidirectional stream
- Agent operates independently with last-known config when server is unreachable
- Scripts are resolved to inline commands server-side before pushing to agents
- Multi-repo backups run independently (not restic copy) per repository sequentially
- Hooks run once per job, not once per repository

## Documentation Reference
- `docs/architecture-overview.md` — System components and data flows
- `docs/grpc-api.md` — Proto definitions + REST API endpoints
- `docs/database-schema.md` — Server + agent SQLite schemas
- `docs/data-models.md` — Entity relationships and field details
- `docs/hooks-design.md` — Hook lifecycle and composition model
- `docs/multi-repo-strategy.md` — Independent backup strategy
- `docs/agent-server-design.md` — Communication patterns and enrollment
- `docs/open-questions.md` — Decided questions and version scope
