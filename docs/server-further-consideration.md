# Server — Further Consideration

Items identified during the server code review that are deferred because they require cross-system changes (frontend modifications), are large in scope, or need a separate design discussion.

## Cross-System Changes (require frontend updates)

### Hide API key from Agent JSON responses
**Severity:** P0 — credential leak
**File:** `server/internal/database/agents.go`

The `Agent` struct has `json:"api_key,omitempty"`, so every `GET /agents` and `GET /agents/{id}` response returns the secret API key to the frontend. Should be changed to `json:"-"` or use a separate response DTO. Frontend currently reads this field and will need updating.

### Hide repository password from API responses
**Severity:** P0 — credential leak
**File:** `server/internal/database/repositories.go`

The `Repository.Password` field has `json:"password"`, exposing the plaintext restic password in every list/get response. Should be omitted or masked in responses. Frontend form handling will need adjustment.

### Move `jsonContentType` into `/api` route group
**Severity:** P1 — bug
**File:** `server/internal/api/router.go`

The `jsonContentType` middleware is applied globally via `r.Use(jsonContentType)`, which means the embedded frontend SPA handler (`r.NotFound(frontend.Handler())`) also gets `Content-Type: application/json` on all HTML/CSS/JS assets. This breaks the SPA when served from the binary in production. The middleware should be scoped to the `/api` route group only. Currently masked in development because Vite serves the frontend separately.

### Add REST API authentication
**Severity:** P0 — no auth
**File:** `server/internal/api/router.go`

No authentication middleware exists on any HTTP endpoint. Anyone with network access can approve agents, delete plans, trigger backups, modify settings. Needs auth middleware (at minimum static token or basic auth for v1). Frontend will need credential handling.

## Separate Design Discussions

### CORS hardening
**File:** `server/internal/api/router.go`

CORS `Allow-Methods` and `Allow-Headers` headers are set unconditionally for all requests regardless of whether the origin matches the whitelist. Only `Allow-Origin` is conditional. Should make all CORS headers conditional on the origin check and gate the allowed origins by environment (dev vs production). Deferred to avoid blocking development workflow.

### Snapshot cache review
**Files:** `server/internal/grpcserver/report.go`

Two related issues:
1. `snapshotCache` map grows without bounds — no eviction, no TTL, no size limit
2. The cache appears to be dead code — it's written to by `ReportSnapshots` but never read; `listSnapshotsHandler` sends real-time commands to agents instead

Needs a design decision: either wire the cache as a read-through layer (faster snapshot listings when agents are offline) or remove it entirely. Track as a separate issue.

### Structured logging migration
**Files:** all server packages

All logging uses `log.Printf` with no severity levels and no structured fields. Makes production debugging difficult. Migrating to `log/slog` would improve observability but is a broad change touching every file.

### Audit logging
Not currently tracked — no record of who approved agents, modified plans, or triggered backups. Important for production use but needs a design for what to log and where.

---

## Pass 2 — Additional Items

### Config push race condition
**Files:** `server/internal/configpush/resolver.go`, all API handlers

Many handlers fire `go resolver.PushConfigToAgent(agentID)` concurrently. Two rapid updates produce two concurrent pushes that each increment `config_version` independently. The agent may receive stale data with a higher version number. Needs a per-agent debounce or serialized push queue.

### `pushConfigForPlan` silently discards errors
**File:** `server/internal/api/hooks.go`

`pushConfigForPlan` returns silently on DB errors with no logging. Should at minimum `log.Printf` the error.

### `updateSettingsHandler` allows writing arbitrary keys
**File:** `server/internal/api/settings.go`

The handler iterates request body and writes every key/value pair with no key validation. Allows polluting the settings table. Not a SQL injection risk (parameterized queries), but a data integrity issue. Should validate keys against an allow-list.
