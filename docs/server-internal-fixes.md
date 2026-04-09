# Server Internal Fixes

Scope: internal-only changes to the server Go module. No API contract changes, no frontend impact.

## 1. Add request body size limit middleware

**File:** `server/internal/api/router.go`

Add `http.MaxBytesReader` middleware (1 MB limit) to prevent memory exhaustion from oversized request bodies. Applied globally before route handlers.

## 2. Add gRPC graceful stop timeout

**File:** `server/cmd/server/main.go`

`grpcSrv.GracefulStop()` can block indefinitely if a stream stays open. Add a 15-second timeout with fallback to `grpcSrv.Stop()`.

## 3. Make `UpdateConfigVersion` atomic

**File:** `server/internal/database/agents.go`

The increment (`UPDATE ... SET config_version = config_version + 1`) and read (`SELECT config_version`) are two separate statements. Under concurrent config pushes for the same agent, one caller can read a version incremented by another. Wrap in a transaction.

## 4. Wrap `DeleteScript` in a transaction

**File:** `server/internal/database/scripts.go`

The "check references" query and `DELETE` are separate statements (TOCTOU race). A hook could reference the script between the check and the delete. Wrap both in a single transaction.

## 5. Fix N+1 query in `ListPlans`

**File:** `server/internal/database/plans.go`

Currently does a separate `SELECT repository_id` query per plan. Replace with a single batch query using `WHERE backup_plan_id IN (...)` after loading all plans.

## 6. Handle JSON unmarshal errors in `ListPlans`

**File:** `server/internal/database/plans.go`

In the `ListPlans` loop, `json.Unmarshal` errors for excludes, tags, and retention are silently discarded. Add error handling consistent with `GetPlan`.

## 7. Add pagination to `ListJobs`

**Files:** `server/internal/database/jobs.go`, `server/internal/api/jobs.go`

`ListJobs` returns all matching jobs with no `LIMIT`. Add optional `limit` (default 50, max 200) and `offset` query parameters. Backward-compatible — no params = default limit.

## 8. Fix silenced errors in API handlers

**Files:** `server/internal/api/agents.go`, `server/internal/api/repositories.go`

Several handlers discard errors from DB calls that follow the primary operation, then use potentially-nil results:
- `approveAgentHandler`: `agent, _ := db.GetAgent(id)` after approval
- `rejectAgentHandler`: same after rejection
- `updateRcloneHandler`: same after update
- `deleteRepositoryHandler`: `agentIDs, _ := db.AgentIDsUsingRepository(id)`

Add proper error logging for these cases.

## 9. Fix incorrect HTTP status codes

**Files:** `server/internal/api/agents.go`, `server/internal/api/plans.go`, `server/internal/api/snapshots.go`

- "agent not connected" returns `404` → should be `502` (server acts as gateway to agent)
- Approve/reject agent failure returns `400` → distinguish `404` (agent not found) from `409` (not in pending status)

## 10. Add input validation

**Files:** `server/internal/api/plans.go`, `server/internal/api/repositories.go`, `server/internal/api/hooks.go`

- Plan schedule: validate it has 5–6 space-separated fields (basic cron format check)
- Repository scope: validate against known values (`global`, `local`)
- Hook on_event: validate against known events (`pre_backup`, `post_backup`, `on_success`, `on_failure`, `pre_restore`, `post_restore`, `pre_forget`, `post_forget`)

## 11. Fix ignored `w.Write()` return values

**File:** `server/internal/api/snapshots.go`

Raw `w.Write()` calls ignore the error return. Assign to blank identifiers to acknowledge.

---

## Pass 2 — Second Review Findings

## 12. Add `busy_timeout` pragma

**File:** `server/internal/database/db.go`

WAL mode is enabled but there's no `PRAGMA busy_timeout`. Under concurrent writes (HTTP + gRPC), SQLite returns `SQLITE_BUSY` immediately. Add `PRAGMA busy_timeout=5000` to retry for 5 seconds.

## 13. Add connection pool configuration

**File:** `server/internal/database/db.go`

`database/sql` defaults have no connection limits. For SQLite with WAL, set `MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime`, `ConnMaxIdleTime`.

## 14. Add database `Close()` method

**File:** `server/internal/database/db.go`

No explicit `Close()` method on the `DB` wrapper. Server shutdown can't cleanly close the DB.

## 15. Propagate `context.Context` through database methods

**Files:** all `server/internal/database/*.go` + all callers

Every method uses `db.Exec`/`db.Query`/`db.QueryRow` instead of context variants. HTTP cancellation, gRPC deadlines, and shutdown signals don't reach the DB layer.

## 16. Add HTTP server timeouts

**File:** `server/cmd/server/main.go`

`http.Server` has no `ReadTimeout`, `WriteTimeout`, or `IdleTimeout`. Vulnerable to slowloris-style connection exhaustion.

## 17. Add event hub `Close()` method

**File:** `server/internal/events/hub.go`

No `Close()` method. WebSocket client channels aren't cleaned up on shutdown.

## 18. Add agent manager `Close()` method

**File:** `server/internal/agentmgr/manager.go`

No `Close()` or shutdown method. Connected agents' send channels survive until GC.

## 19. Wire shutdown orchestration in `main.go`

**File:** `server/cmd/server/main.go`

After gRPC + HTTP stop, add ordered shutdown: agent manager → event hub → database.

## 20. Add gRPC recovery interceptor

**File:** `server/internal/grpcserver/server.go`

A panic in any gRPC handler crashes the process. Switch to `ChainUnaryInterceptor`/`ChainStreamInterceptor` and add recovery middleware.

## 21. Use proper gRPC status code in `Register`

**File:** `server/internal/grpcserver/register.go`

Returns `fmt.Errorf` which maps to `codes.Unknown`. Use `status.Errorf(codes.Internal, ...)`.

## 22. Inject agent ID into stream auth context

**File:** `server/internal/grpcserver/auth.go`

Stream interceptor validates API key but doesn't enrich context with agent ID. Future stream RPCs calling `agentIDFromContext` get empty string.

## 23. Remove dead code `grpcserver.New()`

**File:** `server/internal/grpcserver/server.go`

`New()` is never called — only `NewGRPCServer()` is used. Remove to reduce confusion.

## 24. Cache agent status in `Connect` stream

**File:** `server/internal/grpcserver/connect.go`

Every received message triggers `s.db.GetAgent(agentID)` to re-check status. With 10s heartbeats and many agents, this is unnecessary load. Cache status and re-check periodically.

## 25. Batch repository loading in config resolver

**File:** `server/internal/configpush/resolver.go`

`PushConfigToAgent` calls `GetRepository(rid)` in a loop. Replace with single `SELECT ... WHERE id IN (...)`.
