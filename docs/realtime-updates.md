# Real-Time WebSocket Updates — Implementation Tracker

Tracks progress for GitHub issues [#2](https://github.com/tryy3/backup-orchestrator/issues/2) and [#10](https://github.com/tryy3/backup-orchestrator/issues/10).

## Overview

Add WebSocket push from server to frontend so job status, progress, and agent events update live — no polling needed. Delivered incrementally across 3 phases.

**Decisions**:
- **Transport**: WebSocket (bidirectional, allows future subscriptions)
- **Server lib**: `nhooyr.io/websocket` (idiomatic Go, well-maintained)
- **Frontend**: Native `WebSocket` API (no library needed)
- **Event format**: JSON `{ "type": "job.created", "payload": {...} }`
- **Broadcast model**: All connected browsers receive all events (no per-topic filtering in v1)
- **Hub**: In-memory, no event persistence. Clients do a full fetch on reconnect.
- **Auth**: None on WS endpoint (matches current REST API). Token auth can be added later.

---

## Phase 1: WebSocket Core Infrastructure

| Done | Task | Files |
|------|------|-------|
| [x] | Create server-side event hub (pub/sub with Go channels) | `server/internal/events/hub.go` |
| [x] | Add `nhooyr.io/websocket` dependency | `server/go.mod` |
| [x] | Create WebSocket upgrade handler (`GET /api/ws`) | `server/internal/api/websocket.go` |
| [x] | Wire Hub into Chi router + add `/api/ws` route | `server/internal/api/router.go` |
| [x] | Wire Hub into server startup, pass to API + gRPC | `server/cmd/server/main.go` |
| [x] | Create frontend WebSocket service (auto-reconnect, event dispatch) | `frontend/src/api/websocket.ts` |
| [x] | Initialize WebSocket connection in App.vue | `frontend/src/App.vue` |

**Verification**: Browser connects with `101 Switching Protocols`. Auto-reconnect works on server restart. No console errors during idle connection.

---

## Phase 2: Job Status & Progress Events

| Done | Task | Files |
|------|------|-------|
| [x] | Add `CreatePlannedJob` DB method (new `planned` status) | `server/internal/database/jobs.go` |
| [x] | Create planned job on plan trigger, return job ID, broadcast `job.created` | `server/internal/api/plans.go` |
| [x] | Track current job per agent in Manager | `server/internal/agentmgr/manager.go` |
| [x] | Emit `job.started` + `job.progress` from heartbeats | `server/internal/grpcserver/connect.go` |
| [x] | Emit `job.completed` on job report | `server/internal/grpcserver/report.go` |
| [x] | Add WebSocket event types to frontend | `frontend/src/types/api.ts` |
| [x] | Update jobs store to handle WS events | `frontend/src/stores/jobs.ts` |
| [x] | Update plans store trigger to return job ID | `frontend/src/stores/plans.ts` |
| [x] | Add live progress bar in JobDetailView | `frontend/src/views/JobDetailView.vue` |
| [x] | Add live job updates in DashboardView | `frontend/src/views/DashboardView.vue` |

**Verification**: Trigger backup → job appears instantly as `planned` → transitions to `running` with progress bar → completes with results. Works across multiple browser tabs.

---

## Phase 3: Agent Status Events

| Done | Task | Files |
|------|------|-------|
| [x] | Emit `agent.connected` / `agent.disconnected` from gRPC stream | `server/internal/grpcserver/connect.go` |
| [x] | Emit `agent.heartbeat` with timestamp | `server/internal/grpcserver/connect.go` |
| [x] | Emit `agent.registered` on new enrollment | `server/internal/grpcserver/register.go` |
| [x] | Update agents store to handle agent events | `frontend/src/stores/agents.ts` |
| [x] | Live heartbeat timer in AgentDetailView | `frontend/src/views/AgentDetailView.vue` |
| [x] | Live agent health in DashboardView | `frontend/src/views/DashboardView.vue` |

**Verification**: Connect/disconnect agent → status changes in UI immediately. "Last heartbeat" updates without refresh.

---

## Architecture

```
┌─────────┐  heartbeat/report   ┌─────────┐  WebSocket   ┌──────────┐
│  Agent   │ ─────gRPC─────────→│  Server  │────push─────→│ Frontend │
│  (restic)│                    │  (Go)    │              │  (Vue)   │
└─────────┘                    └─────────┘              └──────────┘
                                    │                        │
                              events.Hub               ws service
                              (broadcast)          (auto-reconnect)
```

### Event Types

| Event | Trigger | Payload |
|-------|---------|---------|
| `job.created` | Plan triggered (manual/cron) | `{ job: Job }` |
| `job.started` | First heartbeat with `current_job` | `{ job_id, agent_id, plan_id, plan_name, started_at }` |
| `job.progress` | Each heartbeat during backup | `{ agent_id, plan_id, plan_name, progress_percent }` |
| `job.completed` | Job report received | `{ job: Job }` |
| `agent.connected` | Agent gRPC stream opens | `{ agent_id, hostname }` |
| `agent.disconnected` | Agent gRPC stream closes | `{ agent_id }` |
| `agent.heartbeat` | Each agent heartbeat | `{ agent_id, timestamp }` |
| `agent.registered` | New agent enrolls | `{ agent: Agent }` |

### New Job Status Lifecycle

```
planned → running → success | partial | failed
```
