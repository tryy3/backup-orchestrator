# Backup Orchestrator — MVP v1 Roadmap

Progress tracker for the MVP v1 implementation. Check off tasks as they're completed.

---

## Phase 0: Foundation

### 0.1 — Proto + Codegen
- [x] Create `proto/backup/v1/backup.proto`
- [x] Set up `buf.yaml` + `buf.gen.yaml`
- [x] Generate Go code into server and agent modules
- [x] Add `Makefile` with `proto-gen` target

### 0.2 — Go Module Scaffolding
- [x] `server/go.mod` + minimal `main.go`
- [x] `agent/go.mod` + minimal `main.go`
- [x] Install core dependencies (grpc, sqlite, chi, cron)

### 0.3 — Frontend Scaffolding
- [x] Vue 3 + TypeScript + Vite + Pinia + Vue Router + Tailwind CSS
- [x] `vite.config.ts` with `/api` proxy
- [x] `src/types/api.ts` — TypeScript API types
- [x] `src/api/client.ts` — typed HTTP client stub

### 0.4 — Project Setup
- [x] `.gitignore`
- [x] `CLAUDE.md` project conventions

---

## Phase 1: Parallel Implementation

### Stream A: Backend (Server)

#### A1 — SQLite Database Layer
- [x] `server/internal/database/db.go` — SQLite connection (WAL mode, foreign keys)
- [x] `server/internal/database/migrations.go` — Schema from docs
- [x] `server/internal/database/repositories.go` — CRUD
- [x] `server/internal/database/agents.go` — CRUD
- [x] `server/internal/database/plans.go` — CRUD + join table
- [x] `server/internal/database/scripts.go` — CRUD
- [x] `server/internal/database/hooks.go` — CRUD + reorder
- [x] `server/internal/database/jobs.go` — CRUD + results
- [x] `server/internal/database/settings.go` — Get/Set

#### A2 — REST API
- [x] `server/internal/api/router.go` — Chi router, CORS, middleware
- [x] `server/internal/api/agents.go` — Agent endpoints
- [x] `server/internal/api/repositories.go` — Repository CRUD
- [x] `server/internal/api/scripts.go` — Script CRUD
- [x] `server/internal/api/plans.go` — Plan CRUD + trigger
- [x] `server/internal/api/hooks.go` — Hook CRUD + reorder
- [x] `server/internal/api/jobs.go` — Job listing + detail
- [x] `server/internal/api/snapshots.go` — Snapshot list/browse/restore
- [x] `server/internal/api/settings.go` — Settings GET/PUT

#### A3 — Agent Manager
- [x] `server/internal/agentmgr/manager.go` — In-memory connected agent registry

#### A4 — gRPC Server
- [x] `server/internal/grpcserver/server.go` — gRPC server setup
- [x] `server/internal/grpcserver/auth.go` — API key interceptors
- [x] `server/internal/grpcserver/register.go` — Register RPC
- [x] `server/internal/grpcserver/connect.go` — Bidirectional stream
- [x] `server/internal/grpcserver/report.go` — ReportJob + ReportSnapshots

#### A5 — Config Push
- [x] `server/internal/configpush/resolver.go` — Assemble + push AgentConfig

#### A6 — Server Wiring
- [x] `server/internal/config/config.go` — Env var config
- [x] `server/cmd/server/main.go` — Full wiring + graceful shutdown

### Stream B: Agent

#### B1 — Bootstrap + Identity + Local DB
- [x] `agent/internal/config/config.go` — Env var config
- [x] `agent/internal/identity/identity.go` — Load/save identity.json
- [x] `agent/internal/database/db.go` — Agent SQLite (buffered_reports, local_jobs)
- [x] `agent/internal/localconfig/store.go` — Persist/load AgentConfig

#### B2 — Restic Executor
- [x] `agent/internal/executor/restic.go` — Backup, Forget, Prune, Snapshots, ListFiles, Restore, InitRepo
- [x] `agent/internal/executor/rclone.go` — Write rclone.conf, env vars

#### B3 — Hook Executor
- [x] `agent/internal/executor/hooks.go` — Command runner with timeout + template vars

#### B4 — Backup Job Orchestrator
- [x] `agent/internal/executor/backup.go` — Full multi-repo + hooks lifecycle

#### B5 — gRPC Client
- [x] `agent/internal/grpcclient/client.go` — Connection management
- [x] `agent/internal/grpcclient/register.go` — Registration flow
- [x] `agent/internal/grpcclient/stream.go` — Connect stream (heartbeat, config, commands)
- [x] `agent/internal/grpcclient/reporter.go` — ReportJob + ReportSnapshots

#### B6 — Scheduler
- [x] `agent/internal/scheduler/scheduler.go` — Cron scheduler + manual trigger

#### B7 — Report Buffer Flusher
- [x] `agent/internal/reporter/reporter.go` — Background flush of buffered reports

#### B8 — Agent Wiring
- [x] `agent/cmd/agent/main.go` — Full wiring + graceful shutdown

### Stream C: Frontend

#### C1 — Layout + Navigation
- [x] `AppLayout.vue` — Sidebar + main content
- [x] `Sidebar.vue` — Navigation links
- [x] `Header.vue` — Top bar
- [x] `router/index.ts` — All routes

#### C2 — Common Components
- [x] `DataTable.vue` — Sortable, filterable table
- [x] `ConfirmDialog.vue` — Destructive action modal
- [x] `StatusBadge.vue` — Colored status badge
- [x] `EmptyState.vue` + `LoadingSpinner.vue`

#### C3 — Pinia Stores
- [x] `stores/agents.ts`
- [x] `stores/repositories.ts`
- [x] `stores/plans.ts`
- [x] `stores/scripts.ts`
- [x] `stores/jobs.ts`
- [x] `stores/snapshots.ts`
- [x] `stores/settings.ts`

#### C4 — Dashboard
- [x] `DashboardView.vue` — Agent status cards, recent jobs, quick actions

#### C5 — Agents Views
- [x] `AgentsView.vue` — Agent list table
- [x] `AgentDetailView.vue` — Detail + rclone config + plans + jobs

#### C6 — Repositories Views
- [x] `RepositoriesView.vue` — Repo list with scope filter
- [x] `RepositoryFormView.vue` — Create/edit form

#### C7 — Scripts Views
- [x] `ScriptsView.vue` — Script list
- [x] `ScriptFormView.vue` — Create/edit form

#### C8 — Backup Plans Views
- [x] `PlansView.vue` — Plan list by agent
- [x] `PlanFormView.vue` — Full plan form (paths, repos, schedule, retention)
- [x] `PlanDetailView.vue` — Detail + hooks + trigger + jobs
- [x] `HookEditor.vue` — Hook list editor
- [x] `RetentionEditor.vue` — Retention policy fields
- [x] `RepositoryPicker.vue` — Multi-select grouped by scope

#### C9 — Jobs Views
- [x] `JobsView.vue` — Job list with filters
- [x] `JobDetailView.vue` — Full detail with repo/hook results

#### C10 — Snapshots + Restore
- [x] `SnapshotsView.vue` — Browse snapshots + trigger restore

#### C11 — Settings
- [x] `SettingsView.vue` — Global retention defaults

---

## Phase 2: Integration + Docker

### 2.1 — Config Push Wiring
- [x] Wire REST mutations to trigger config push to affected agents
- [x] Handle fan-out for global resources (scripts, repos, settings)

### 2.2 — Embed Frontend
- [x] Go embed for `frontend/dist/` in server binary
- [x] SPA fallback routing

### 2.3 — Docker
- [x] `docker/Dockerfile.server` — Multi-stage build
- [x] `docker/Dockerfile.agent` — Multi-stage build + restic + rclone
- [x] `docker/docker-compose.yml` — Server + demo agent

### 2.4 — End-to-End Verification
- [ ] Agent enrollment flow
- [ ] Config push flow
- [ ] Scheduled backup execution
- [ ] Manual trigger from UI
- [ ] Restore flow
- [ ] Offline resilience (buffered reports)
- [ ] Hook execution and reporting
