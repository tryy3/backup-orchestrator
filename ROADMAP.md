# Backup Orchestrator — MVP v1 Roadmap

Progress tracker for the MVP v1 implementation. Check off tasks as they're completed.

---

## Phase 0: Foundation

### 0.1 — Proto + Codegen
- [ ] Create `proto/backup/v1/backup.proto`
- [ ] Set up `buf.yaml` + `buf.gen.yaml`
- [ ] Generate Go code into server and agent modules
- [ ] Add `Makefile` with `proto-gen` target

### 0.2 — Go Module Scaffolding
- [ ] `server/go.mod` + minimal `main.go`
- [ ] `agent/go.mod` + minimal `main.go`
- [ ] Install core dependencies (grpc, sqlite, chi, cron)

### 0.3 — Frontend Scaffolding
- [ ] Vue 3 + TypeScript + Vite + Pinia + Vue Router + Tailwind CSS
- [ ] `vite.config.ts` with `/api` proxy
- [ ] `src/types/api.ts` — TypeScript API types
- [ ] `src/api/client.ts` — typed HTTP client stub

### 0.4 — Project Setup
- [ ] `.gitignore`
- [ ] `CLAUDE.md` project conventions

---

## Phase 1: Parallel Implementation

### Stream A: Backend (Server)

#### A1 — SQLite Database Layer
- [ ] `server/internal/database/db.go` — SQLite connection (WAL mode, foreign keys)
- [ ] `server/internal/database/migrations.go` — Schema from docs
- [ ] `server/internal/database/repositories.go` — CRUD
- [ ] `server/internal/database/agents.go` — CRUD
- [ ] `server/internal/database/plans.go` — CRUD + join table
- [ ] `server/internal/database/scripts.go` — CRUD
- [ ] `server/internal/database/hooks.go` — CRUD + reorder
- [ ] `server/internal/database/jobs.go` — CRUD + results
- [ ] `server/internal/database/settings.go` — Get/Set

#### A2 — REST API
- [ ] `server/internal/api/router.go` — Chi router, CORS, middleware
- [ ] `server/internal/api/agents.go` — Agent endpoints
- [ ] `server/internal/api/repositories.go` — Repository CRUD
- [ ] `server/internal/api/scripts.go` — Script CRUD
- [ ] `server/internal/api/plans.go` — Plan CRUD + trigger
- [ ] `server/internal/api/hooks.go` — Hook CRUD + reorder
- [ ] `server/internal/api/jobs.go` — Job listing + detail
- [ ] `server/internal/api/snapshots.go` — Snapshot list/browse/restore
- [ ] `server/internal/api/settings.go` — Settings GET/PUT

#### A3 — Agent Manager
- [ ] `server/internal/agentmgr/manager.go` — In-memory connected agent registry

#### A4 — gRPC Server
- [ ] `server/internal/grpcserver/server.go` — gRPC server setup
- [ ] `server/internal/grpcserver/auth.go` — API key interceptors
- [ ] `server/internal/grpcserver/register.go` — Register RPC
- [ ] `server/internal/grpcserver/connect.go` — Bidirectional stream
- [ ] `server/internal/grpcserver/report.go` — ReportJob + ReportSnapshots

#### A5 — Config Push
- [ ] `server/internal/configpush/resolver.go` — Assemble + push AgentConfig

#### A6 — Server Wiring
- [ ] `server/internal/config/config.go` — Env var config
- [ ] `server/cmd/server/main.go` — Full wiring + graceful shutdown

### Stream B: Agent

#### B1 — Bootstrap + Identity + Local DB
- [ ] `agent/internal/config/config.go` — Env var config
- [ ] `agent/internal/identity/identity.go` — Load/save identity.json
- [ ] `agent/internal/database/db.go` — Agent SQLite (buffered_reports, local_jobs)
- [ ] `agent/internal/localconfig/store.go` — Persist/load AgentConfig

#### B2 — Restic Executor
- [ ] `agent/internal/executor/restic.go` — Backup, Forget, Prune, Snapshots, ListFiles, Restore, InitRepo
- [ ] `agent/internal/executor/rclone.go` — Write rclone.conf, env vars

#### B3 — Hook Executor
- [ ] `agent/internal/executor/hooks.go` — Command runner with timeout + template vars

#### B4 — Backup Job Orchestrator
- [ ] `agent/internal/executor/backup.go` — Full multi-repo + hooks lifecycle

#### B5 — gRPC Client
- [ ] `agent/internal/grpcclient/client.go` — Connection management
- [ ] `agent/internal/grpcclient/register.go` — Registration flow
- [ ] `agent/internal/grpcclient/stream.go` — Connect stream (heartbeat, config, commands)
- [ ] `agent/internal/grpcclient/reporter.go` — ReportJob + ReportSnapshots

#### B6 — Scheduler
- [ ] `agent/internal/scheduler/scheduler.go` — Cron scheduler + manual trigger

#### B7 — Report Buffer Flusher
- [ ] `agent/internal/reporter/reporter.go` — Background flush of buffered reports

#### B8 — Agent Wiring
- [ ] `agent/cmd/agent/main.go` — Full wiring + graceful shutdown

### Stream C: Frontend

#### C1 — Layout + Navigation
- [ ] `AppLayout.vue` — Sidebar + main content
- [ ] `Sidebar.vue` — Navigation links
- [ ] `Header.vue` — Top bar
- [ ] `router/index.ts` — All routes

#### C2 — Common Components
- [ ] `DataTable.vue` — Sortable, filterable table
- [ ] `ConfirmDialog.vue` — Destructive action modal
- [ ] `StatusBadge.vue` — Colored status badge
- [ ] `EmptyState.vue` + `LoadingSpinner.vue`

#### C3 — Pinia Stores
- [ ] `stores/agents.ts`
- [ ] `stores/repositories.ts`
- [ ] `stores/plans.ts`
- [ ] `stores/scripts.ts`
- [ ] `stores/jobs.ts`
- [ ] `stores/snapshots.ts`
- [ ] `stores/settings.ts`

#### C4 — Dashboard
- [ ] `DashboardView.vue` — Agent status cards, recent jobs, quick actions

#### C5 — Agents Views
- [ ] `AgentsView.vue` — Agent list table
- [ ] `AgentDetailView.vue` — Detail + rclone config + plans + jobs

#### C6 — Repositories Views
- [ ] `RepositoriesView.vue` — Repo list with scope filter
- [ ] `RepositoryFormView.vue` — Create/edit form

#### C7 — Scripts Views
- [ ] `ScriptsView.vue` — Script list
- [ ] `ScriptFormView.vue` — Create/edit form

#### C8 — Backup Plans Views
- [ ] `PlansView.vue` — Plan list by agent
- [ ] `PlanFormView.vue` — Full plan form (paths, repos, schedule, retention)
- [ ] `PlanDetailView.vue` — Detail + hooks + trigger + jobs
- [ ] `HookEditor.vue` — Hook list editor
- [ ] `RetentionEditor.vue` — Retention policy fields
- [ ] `RepositoryPicker.vue` — Multi-select grouped by scope

#### C9 — Jobs Views
- [ ] `JobsView.vue` — Job list with filters
- [ ] `JobDetailView.vue` — Full detail with repo/hook results

#### C10 — Snapshots + Restore
- [ ] `SnapshotsView.vue` — Browse snapshots + trigger restore

#### C11 — Settings
- [ ] `SettingsView.vue` — Global retention defaults

---

## Phase 2: Integration + Docker

### 2.1 — Config Push Wiring
- [ ] Wire REST mutations to trigger config push to affected agents
- [ ] Handle fan-out for global resources (scripts, repos, settings)

### 2.2 — Embed Frontend
- [ ] Go embed for `frontend/dist/` in server binary
- [ ] SPA fallback routing

### 2.3 — Docker
- [ ] `docker/Dockerfile.server` — Multi-stage build
- [ ] `docker/Dockerfile.agent` — Multi-stage build + restic + rclone
- [ ] `docker/docker-compose.yml` — Server + demo agent

### 2.4 — End-to-End Verification
- [ ] Agent enrollment flow
- [ ] Config push flow
- [ ] Scheduled backup execution
- [ ] Manual trigger from UI
- [ ] Restore flow
- [ ] Offline resilience (buffered reports)
- [ ] Hook execution and reporting
