# Agent Outbox Redesign

**Status**: Phases 1–3 implemented. Phase 4 pending.
**Scope**: Closes [#33](https://github.com/tryy3/backup-orchestrator/issues/33), [#104](https://github.com/tryy3/backup-orchestrator/issues/104), [#109](https://github.com/tryy3/backup-orchestrator/issues/109)

## Summary

Replace the agent's two persistent tables — `local_jobs` and `buffered_reports` — with a single **in-memory-first outbox** that spills to SQLite only when the server is unreachable. The agent's SQLite is treated as a transient delivery cache, not a permanent store. Send eagerly, ack-and-forget, prune aggressively. Bound the spill table by configurable TTL and row count.

The same outbox is also used for incremental job events (`job_started`, `status_changed`, `log_batch`, `job_completed`), subsuming the existing `LiveLogs` heartbeat stream. One mechanism handles both buffered final reports and live-event streaming.

## Goals (per issue)

- **#33**: SQLite acts as a delivery cache — rows are deleted after successful server sync; bounded fallback retention.
- **#104**: Drop payload duplication (`LogTail` vs `LogEntries`); paginate the flush so peak heap is bounded by batch size, not backlog size.
- **#109**: Bound `local_jobs` (eliminated entirely) and the new spill table by both TTL (default 7 days) and row count (default 5000).

## Architecture

```
executor / scheduler
        │
        │ outbox.Submit(item)
        ▼
┌────────────────────────────────────────────┐
│  In-memory queue  (cap: OUTBOX_MEMORY_MAX) │
│         (default 2000 items)               │
└──────────────┬─────────────────────────────┘
               │
               │  worker drains continuously
               ▼
        gRPC SubmitJobEvent / ReportJob
               │
       success │  failure
       ◀───────┴─────────▶
       drop              spill to SQLite (`outbox_spill`)
                              │  cap: OUTBOX_SPILL_MAX (default 20000)
                              │  ttl: OUTBOX_SPILL_RETENTION_DAYS (default 7d)
                              │  on overflow → drop oldest
                              ▼
                  drained on reconnect / ticker / next success
```

### Two-tier capacity policy

1. **Memory tier** (cap `OUTBOX_MEMORY_MAX`, default **2000** items)
   - Hot path: `Submit` writes here via non-blocking channel send.
   - On overflow → spill *that item* directly to SQLite. Executor never blocks.

2. **SQLite spill tier** (cap `OUTBOX_SPILL_MAX`, default **20 000** rows; TTL `OUTBOX_SPILL_RETENTION_DAYS`, default **7 days**)
   - Drained back into memory in pages of 50 by the worker on connection success and on the periodic ticker.
   - On row-cap overflow → **drop oldest** rows. Documented loss semantic.
   - Daily prune ticker enforces TTL and row cap, then runs `PRAGMA wal_checkpoint(TRUNCATE)`.

Rationale (per discussion): server outages long enough to fill 22 000 items are pathological. We prefer dropping oldest events over blocking an in-flight restic command.

## Phases

### Phase 1 — Drop duplication & deprecate field (partial fix for #104)

*Smallest non-throwaway slice. The pagination half of #104 is folded into Phase 2 because Phase 2 replaces the reporter entirely — paginating the soon-to-be-deleted code is throwaway.*

- [x] Stderr cap at executor (already in [agent/internal/executor/restic.go](agent/internal/executor/restic.go) — see `TestStreamRestic_StderrIsBounded`).
- [x] Stop populating `report.LogTail` in [agent/internal/executor/backup.go](agent/internal/executor/backup.go). Server already prefers `LogEntries`; the `LogTail` fallback in [server/internal/grpcserver/report.go](server/internal/grpcserver/report.go) ~L80 stays for backward compat with old agents.
- [x] Mark `JobReport.log_tail` (field 13) `[deprecated = true]` in [proto/backup/v1/backup.proto](proto/backup/v1/backup.proto). Wire-compatible. Server-side fallback path silenced with `//nolint:staticcheck` annotation.

### Phase 2 — In-memory outbox + SQLite spill (fixes #33 + #109)

*Depends on Phase 1.*

- [x] New package [agent/internal/outbox/](agent/internal/outbox/) with:
  - `Outbox` struct, `SubmitReport(ctx, report)`, `Run(ctx)` worker, `FlushNow()`.
  - In-memory bounded channel; on full → spill directly to SQLite.
  - Drain spill in pages of 50 on every successful send and on ticker.
  - Jittered exponential backoff on send failure.
  - Persists in-memory items to spill on shutdown so they survive restarts.
- [x] New SQLite table `outbox_spill (id, kind, payload BLOB, created_at, attempts, last_error)` with index on `created_at`. Replaces `buffered_reports` and `local_jobs` in one migration.
- [x] One-time migration: conditional `INSERT OR IGNORE INTO outbox_spill SELECT … FROM buffered_reports`; `DROP TABLE buffered_reports`; `DROP TABLE IF EXISTS local_jobs`. Idempotent (guarded by `sqlite_master` lookup via `tableExists`).
- [x] DB methods: `SpillEnqueue`, `SpillPage(limit, afterCreatedAt, afterID)`, `SpillDelete`, `SpillIncrementAttempts`, `SpillCount`, `SpillPruneByAge`, `SpillPruneByCount`, `SpillDeleteOldest`, `SpillCheckpoint`. Cursor uses `CAST(created_at AS TEXT)` for stable string ordering.
- [x] Rewire [agent/cmd/agent/main.go](agent/cmd/agent/main.go) `reportFn`: `outbox.SubmitReport(ctx, report)` replaces `InsertLocalJob` + `deliverReport` + `BufferReport`. Deleted the `reporter` package and the obsolete `report_test.go`.
- [x] Daily prune goroutine: TTL prune → row-cap prune → `PRAGMA wal_checkpoint(TRUNCATE)`. Run once at startup too.
- [x] **Folded from #104**: outbox spill drain is paged (50 rows at a time), each batch freed before the next. Heap-bound test in [agent/internal/outbox/outbox_test.go](agent/internal/outbox/outbox_test.go) (`TestDrain_BoundedHeap`) drains 500 spilled items × 4 KiB and asserts the worker delivers all of them without OOM.

### Phase 3 — Configuration

*Depends on Phase 2.*

- [x] Add to [agent/internal/config/config.go](agent/internal/config/config.go):

  | Env var | Default | Purpose |
  |---|---|---|
  | `OUTBOX_MEMORY_MAX` | `2000` | In-memory queue capacity (bootstrap-only — Go channels cannot be resized at runtime) |

- [x] All other tunables are server-pushed via `AgentConfig.outbox` and hot-applied
  by `Outbox.UpdateConfig`. They are configurable globally via the `settings` table
  (and per-agent via `agents.outbox_overrides`, mirroring `command_timeouts`):

  | Setting key | Default | Per-agent field | Purpose |
  |---|---|---|---|
  | `outbox_spill_max_rows` | `20000` | `spill_max_rows` | SQLite spill row cap; oldest dropped on overflow |
  | `outbox_spill_retention_seconds` | `604800` (7d) | `spill_retention_secs` | TTL for spill rows |
  | `outbox_flush_interval_seconds` | `60` | `flush_interval_secs` | Periodic spill drain interval |
  | `outbox_delivery_timeout_seconds` | `10` | `delivery_timeout_secs` | Per-RPC timeout |
  | `outbox_max_attempts` | `10` | `max_attempts` | Drop after N failed sends |

  REST endpoints: `GET/PUT /api/settings` for globals; `PUT /api/agents/{id}/outbox-overrides`
  for per-agent override (send `null` to clear). On change the server re-pushes the
  agent's `AgentConfig`, the agent calls `Outbox.UpdateConfig`, and the flush /
  prune tickers reset to the new intervals. `MemoryMax` stays as initialized at
  agent boot (env var) and is preserved across reloads.

- [x] Update [docs/agent-server-design.md](docs/agent-server-design.md), [docs/database-schema.md](docs/database-schema.md), mark [docs/agent-further-considerations.md](docs/agent-further-considerations.md) §2 resolved.

### Phase 4 — Incremental events through the same outbox

*Depends on Phase 2. The "everything through one path" goal.*

- [ ] Add to [proto/backup/v1/backup.proto](proto/backup/v1/backup.proto):
  ```protobuf
  message JobEvent {
    string agent_id = 1;
    string api_key = 2;
    string event_id = 3;
    google.protobuf.Timestamp emitted_at = 4;
    oneof event {
      JobStarted    started   = 10;
      JobStatus     status    = 11;
      LogBatch      logs      = 12;
      JobReport     completed = 13;
    }
  }
  rpc SubmitJobEvent(JobEvent) returns (JobReportAck);
  ```
- [ ] Keep `rpc ReportJob` for backward compat (old agents). New agents send `SubmitJobEvent`; on `Unimplemented`, fall back to `ReportJob` for `kind=job_report`.
- [ ] Outbox `kind` column carries `"job_report"` or `"job_event"`.
- [ ] Server-side: extend [server/internal/grpcserver/report.go](server/internal/grpcserver/report.go) to handle events incrementally — update job rows in place, broadcast events to [server/internal/events](server/internal/events).
- [ ] Subsume `LiveLogs` stream in [agent/internal/grpcclient/stream.go](agent/internal/grpcclient/stream.go) (~L173): emit `LogBatch` events through outbox instead. Keep the proto for one release for compat; remove later.

## Migration Plan

### Schema (agent SQLite)

In a single transaction inside `migrate()`:

```sql
CREATE TABLE IF NOT EXISTS outbox_spill (
  id          TEXT PRIMARY KEY,
  kind        TEXT NOT NULL,
  payload     BLOB NOT NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  attempts    INTEGER NOT NULL DEFAULT 0,
  last_error  TEXT
);
CREATE INDEX IF NOT EXISTS idx_outbox_spill_created_at ON outbox_spill(created_at);

-- Conditional, only if buffered_reports still exists:
INSERT OR IGNORE INTO outbox_spill (id, kind, payload, created_at, attempts, last_error)
SELECT id, 'job_report', CAST(payload AS BLOB), created_at, attempts, last_error
FROM buffered_reports;
DROP TABLE IF EXISTS buffered_reports;
DROP TABLE IF EXISTS local_jobs;
```

Idempotent: guarded by `IF EXISTS` / `IF NOT EXISTS` and `INSERT OR IGNORE`. No operator action required.

### Wire (agent ↔ server)

- `JobReport.log_tail` field 13 stays in proto (deprecated). Old agents continue to send it; server's existing fallback path reads it.
- `SubmitJobEvent` is additive. New agent + old server: outbox detects `Unimplemented`, falls back to `ReportJob` for `job_report` kind, drops standalone events with debug log (no worse than today).
- `LiveLogs` stream remains in proto for one release; agents stop using it once `SubmitJobEvent` is available on the server.

## Files Touched

**Agent**
- [agent/internal/database/db.go](agent/internal/database/db.go) — schema migration, new outbox methods
- [agent/internal/outbox/](agent/internal/outbox/) — new package (replaces `reporter`)
- [agent/internal/reporter/](agent/internal/reporter/) — deleted after rewire
- [agent/internal/executor/backup.go](agent/internal/executor/backup.go) — drop `LogTail` assignments
- [agent/cmd/agent/main.go](agent/cmd/agent/main.go) — rewire `reportFn`, add prune ticker
- [agent/internal/config/config.go](agent/internal/config/config.go) — 6 new env vars
- [agent/internal/grpcclient/reporter.go](agent/internal/grpcclient/reporter.go) — `SubmitJobEvent` (Phase 4)
- [agent/internal/grpcclient/stream.go](agent/internal/grpcclient/stream.go) — remove `LiveLogs` emission (Phase 4)

**Server (Phase 4)**
- [server/internal/grpcserver/report.go](server/internal/grpcserver/report.go) — add `SubmitJobEvent` handler
- [server/internal/events](server/internal/events) — broadcast incremental events

**Proto**
- [proto/backup/v1/backup.proto](proto/backup/v1/backup.proto) — deprecate `log_tail`; add `JobEvent` + RPC

**Tests rewritten / removed**
- [agent/internal/database/db_test.go](agent/internal/database/db_test.go) — drop `TestLocalJobs_*`, `TestBufferedReports_*`; add `TestOutboxSpill_*`
- [agent/internal/reporter/reporter_test.go](agent/internal/reporter/reporter_test.go) — replaced by `agent/internal/outbox/outbox_test.go`
- New: heap-bound flush test, spill prune tests, two-tier overflow tests

**Docs**
- [docs/database-schema.md](docs/database-schema.md), [docs/agent-server-design.md](docs/agent-server-design.md), [docs/agent-further-considerations.md](docs/agent-further-considerations.md), this file

## Verification

1. `cd agent && go test -race ./internal/database/... ./internal/outbox/... ./internal/executor/... ./cmd/agent/...`
2. `cd server && go test -race ./...`
3. Heap-bound test: 500 spilled reports drained, peak `HeapInuse` ≤ ~3× one batch.
4. Manual offline drill: stop server → run jobs → confirm rows in `outbox_spill`. Restart server → rows drain within one cycle; in-memory queue stays at 0.
5. Manual prune: synthetic-age rows deleted on next prune; disk reclaimed by `wal_checkpoint(TRUNCATE)`.
6. Manual two-tier overflow: simulate sustained server outage with high event rate; observe spill grows to `OUTBOX_SPILL_MAX_ROWS` then oldest entries are dropped (counter logged).
7. `just lint && just test` clean across all modules.

## Out of Scope

- Secret redaction in payload (separate concern, [docs/agent-further-considerations.md §5](docs/agent-further-considerations.md))
- Switching protojson → binary proto encoding (separate perf ticket)
- Server-side job retention (server is source of truth; owns its own retention)
