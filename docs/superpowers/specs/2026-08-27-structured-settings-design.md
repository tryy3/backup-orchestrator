# Structured settings model

**Date:** 2026-08-27  
**Status:** Approved for implementation planning  
**Issue:** [#48](https://github.com/tryy3/backup-orchestrator/issues/48)  
**Scope:** Server settings registry + API validation + frontend field errors

## Summary

Replace the free-form / allow-list-only settings path with a typed registry in `server/internal/settings`. The registry enumerates every valid key with type, default, and constraints. The HTTP API validates the full request before any write, returns all field errors on failure, and serves fully resolved settings (stored value or default) on GET and successful PUT. The frontend surfaces those errors globally and per setting. Settings versioning is explicitly out of scope.

## Goals

- Single server-side source of truth for known keys, defaults, and validation rules.
- Reject unknown keys and invalid values with a collected `errors` array; never write on validation failure.
- `GET /api/settings` (and successful `PUT`) return every known key with a resolved value.
- `configpush` uses registry defaults instead of hardcoded literals.
- Frontend shows all validation errors, including inline per field when the key maps to a control.
- Keep the SQLite `settings` table as key/value TEXT (no schema migration).

## Non-goals

- Settings versioning or key migration metadata.
- Audit log table / authenticated “who changed settings”.
- Configuration hierarchy across ENV / agent / repo ([#155](https://github.com/tryy3/backup-orchestrator/issues/155)).
- Agent-side settings storage model changes (agents keep consuming pushed config).
- Changing other API endpoints’ `{ "error": "..." }` shape.

## Current state

- `server/internal/api/settings.go` already allow-lists keys via `settingsKeys` / `allowedSettings` and rejects unknown keys with `400`.
- Values are still stored as raw JSON strings with no type or range checks.
- Defaults live in `frontend/src/types/api.ts` (`SETTINGS_DEFAULTS`) and as hardcoded fallbacks in `configpush/resolver.go`.
- `database.GetSetting` / `SetSetting` are dumb storage and remain so.
- Server logging already uses `log/slog` in the API package.

## Architecture

New package: `server/internal/settings`.

Each registry entry defines:

| Field | Purpose |
|---|---|
| Key | Stable string (e.g. `heartbeat_interval_seconds`) |
| Default | JSON-serializable default value |
| Validate | Parse `json.RawMessage` and enforce type + constraints |

Public surface (names may vary slightly in implementation):

- `Validate(input map[string]json.RawMessage) []FieldError` — no DB I/O; collect **all** errors (unknown key, bad type, failed constraint).
- `LoadResolved(ctx, db) (map[string]json.RawMessage, error)` — for every registry key: use stored value if present and parseable under the entry’s rules; else default. Corrupt stored values fall back to default and may `slog.Warn`.
- `Apply(ctx, db, input) error` — write keys after validation has succeeded (handler validates first, then applies).
- Helpers for defaults used by `configpush` (e.g. typed getters or `DefaultJSON(key)`).

`database.GetSetting` / `SetSetting` stay unchanged.

`api/settings.go` becomes a thin adapter:

1. Decode body to `map[string]json.RawMessage`.
2. Call `Validate`; if any errors → `slog.Warn` with count/details → `400` with `{ "errors": [...] }` → return (no writes).
3. Else `Apply`, push config to agents, return full `LoadResolved` result (same shape as GET).

`configpush/resolver.go` reads defaults from `internal/settings` instead of literals such as `30` / `60`.

## API contract

### GET `/api/settings`

Returns a complete JSON object: every registry key is present. Value is the stored override when valid, otherwise the registry default.

### PUT `/api/settings`

Partial update: only keys present in the body are validated and written.

Flow:

1. Decode body.
2. Validate all entries; collect every `FieldError`.
3. If errors exist → log → `400` → no DB writes.
4. Else write each key → push config → `200` with full resolved settings object.

### Error body (settings validation only)

```json
{
  "errors": [
    { "key": "heartbeat_interval_seconds", "message": "must be at least 5" },
    { "key": "bogus", "message": "unknown setting" }
  ]
}
```

No top-level `"error"` string on this response. Other endpoints keep `{ "error": "..." }`.

### FieldError

```go
type FieldError struct {
    Key     string `json:"key"`
    Message string `json:"message"`
}
```

## Validation rules

Constraints align with the current Settings UI `min` attributes and existing defaults:

| Key | Type | Constraints (summary) | Default (matches today) |
|---|---|---|---|
| `default_retention` | object | Non-negative ints for `keep_last`, `keep_hourly`, `keep_daily`, `keep_weekly`, `keep_monthly`, `keep_yearly` | `{ "keep_last": 5, "keep_hourly": 0, "keep_daily": 0, "keep_weekly": 0, "keep_monthly": 0, "keep_yearly": 0 }` (matches Settings UI form default) |
| `heartbeat_interval_seconds` | number (int) | ≥ 5 | 30 |
| `agent_offline_threshold_seconds` | number (int) | ≥ 30 | 300 |
| `job_history_days` | number (int) | ≥ 1 | 30 |
| `health_threshold_failing` | number | \> 0 and ≤ 1 | 0.9 |
| `health_threshold_warning` | number | \> 0 and ≤ 1 | 0.99 |
| `max_heatmap_runs` | number (int) | ≥ 5 | 30 |
| `default_hook_timeout_seconds` | number (int) | ≥ 5 | 60 |
| `file_browser_blocked_paths` | string array | Each path non-empty string | `/proc`, `/sys`, `/dev`, `/run/credentials`, `/selinux`, `/cgroup` |
| `command_timeout_backup_seconds` | number (int) | ≥ 60 | 86400 |
| `command_timeout_restore_seconds` | number (int) | ≥ 60 | 86400 |
| `command_timeout_list_snapshots_seconds` | number (int) | ≥ 5 | 300 |
| `command_timeout_browse_snapshot_seconds` | number (int) | ≥ 5 | 300 |
| `command_timeout_browse_filesystem_seconds` | number (int) | ≥ 1 | 30 |
| `command_timeout_default_seconds` | number (int) | ≥ 5 | 300 |
| `outbox_spill_max_rows` | number (int) | ≥ 100 | 20000 |
| `outbox_spill_retention_seconds` | number (int) | ≥ 60 | 604800 (7 days) |
| `outbox_flush_interval_seconds` | number (int) | ≥ 1 | 60 |
| `outbox_delivery_timeout_seconds` | number (int) | ≥ 1 | 10 |
| `outbox_max_attempts` | number (int) | ≥ 1 | 10 |

JSON numbers that are not integers where an int is required are rejected. Wrong JSON types are rejected with a clear per-key message. Unknown keys produce `unknown setting`.

Cross-field rules (e.g. failing threshold vs warning threshold ordering) are **out of scope for v1**; each key is validated independently.

## Frontend

- Extend settings update error handling to parse `{ errors: [{ key, message }] }` into structured field errors (other API errors remain string messages).
- Settings store holds `fieldErrors` (map or list) from the last failed save; clear on successful save and on fetch.
- After successful PUT, replace store state with the returned full settings object.
- `resolved` simplifies: prefer server payload (GET is complete). Keep a thin local `SETTINGS_DEFAULTS` fallback for pre-fetch paint and for `agents.ts` offline threshold before settings are loaded.
- `SettingsView.vue`: show a top-level list of all errors **and** inline text under each control whose key appears in `fieldErrors`. Show all errors; no truncation in v1.
- Align TypeScript `Settings` / `SETTINGS_DEFAULTS` with the registry (same keys and default values). Prefer required fields on `Settings` once GET always returns the full object.

## Testing

**Server**

- Registry unit tests: valid values, invalid type/range, unknown keys, multiple errors collected in one `Validate` call.
- `LoadResolved`: empty DB → all defaults; stored override wins; corrupt stored value → default.
- HTTP handler tests: invalid PUT → `400` + `errors[]` and no DB writes; valid PUT → `200` + full body; GET returns complete key set.

**Frontend**

- Client/store: parse `errors` into `fieldErrors`.
- SettingsView: light coverage that field errors render when existing test patterns make that inexpensive.

## Documentation

- Note in API or database docs that settings keys are defined and validated in `server/internal/settings` (allow-listed, typed, defaulted). No versioning scheme.

## Rollout / compatibility

- Existing DBs keep working: unknown junk keys already in the table are ignored by GET (only registry keys are returned); they are not deleted in v1.
- Clients that only send known keys continue to work; clients sending unknown keys get `400` with `errors` (already true for unknown keys today; now also for bad values).
- PUT success response changes from `{ "status": "updated" }` to the full resolved settings object — frontend store must use that (and can refetch if needed). This is an intentional small API improvement for this endpoint.

## Success criteria

- Unknown or invalid settings never hit the DB.
- Validation failures return every problem in `errors[]` and are logged.
- GET always returns a complete, default-filled settings object.
- Resolver defaults come from the registry.
- Settings UI shows global and per-field errors after a failed save.
- #48 can be closed without claiming versioning support.
