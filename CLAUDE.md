# Backup Orchestrator — Project Conventions

## Overview
Backup orchestration system using restic. Go server + agent communicating via gRPC, Vue.js SPA frontend.

## Project Structure
- `server/` — Go module: REST API (Chi) + gRPC server + SQLite database
- `agent/` — Go module: gRPC client + restic CLI wrapper + cron scheduler
- `frontend/` — Vue 3 + TypeScript + Vite + Pinia + Tailwind CSS
- `proto/` — Protobuf definitions, generates into both Go modules
- `docs/` — Design documentation (architecture, API specs, data models)

## Build Commands
```bash
# Proto generation
make proto-gen

# Server
cd server && go build -o ../bin/server ./cmd/server

# Agent
cd agent && go build -o ../bin/agent ./cmd/agent

# Frontend
cd frontend && npm run dev    # dev server with hot reload
cd frontend && npm run build  # production build to dist/
```

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
