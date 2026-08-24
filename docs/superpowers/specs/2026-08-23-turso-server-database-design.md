# Turso as an optional server database

**Date:** 2026-08-23  
**Status:** Approved for implementation planning  
**Issue:** [#231](https://github.com/tryy3/backup-orchestrator/issues/231)  
**Scope:** Server database backends only

## Summary

Add two optional Turso backends for the Go server while keeping today’s embedded SQLite file as the default. Production is expected to use **Turso Sync**: a local database file remains the process’s source of truth, and Turso Cloud is a replica via explicit `Push()` / `Pull()`. Development and simple test servers can use local SQLite or remote-only Turso.

The agent keeps its on-disk SQLite `state.db` (outbox spill only). Agent last-known config stays in `config.json` / `identity.json`. That is intentional: spill exists because the server is unreachable, and backups must not depend on Turso.

## Goals

- Support three server backends: `sqlite` (default), `turso` (remote-only), `turso-sync` (local file + sync).
- Existing SQLite deployments keep working with no new required env vars.
- If Turso Cloud is unreachable and the process is already running on `turso-sync` with a local file, the control plane keeps full read/write (job reports, heartbeats, config).
- Document a maintenance-window migration from a production SQLite file to `turso-sync`.
- Exercise at least one non-SQLite backend in automated tests.

## Non-goals

- Agent database on Turso (outbox, config, or identity).
- Removing SQLite support.
- In-process automatic migration on first Turso connect.
- Multi-instance server HA / two writers on the same Sync database.
- A `db export` / `db import` CLI in v1 (add later if dump/import is error-prone).
- Classic libSQL embedded replicas (`go-libsql`, CGO).

## Backend model

| Driver | Engine | Process uses | Turso down (process up) | Typical use |
|---|---|---|---|---|
| `sqlite` | `modernc.org/sqlite` | Local `BACKUP_DB_PATH` | N/A | Default, unit tests, simple deploys |
| `turso` | `tursogo-serverless` (Turso Database, remote) | Cloud only | Control plane errors; agents still back up | Shared test server, no local disk |
| `turso-sync` | `tursogo` `NewTursoSyncDb` | Local file; Turso is replica | Full local read/write; sync retries | Production |

`BACKUP_DB_DRIVER` unset or empty means `sqlite`.

Remote-only and sync must target the **same cloud engine** (Turso Database, not classic libSQL via `libsql-client-go`). Otherwise a database created for sync cannot be opened remote-only and the reverse. Local `sqlite` stays on `modernc` so the default path does not take a new engine.

v1 is one server writer. Do not run a `turso-sync` process and a `turso` process against the same cloud database at the same time.

## Configuration

```bash
# Always valid. Default driver is sqlite when BACKUP_DB_DRIVER is unset.
BACKUP_DB_PATH=/var/lib/backup-orchestrator/server.db

# sqlite | turso | turso-sync
BACKUP_DB_DRIVER=sqlite

# Required for turso and turso-sync
BACKUP_DB_URL=...
BACKUP_DB_AUTH_TOKEN=...

# turso-sync only. Default 30s.
BACKUP_DB_SYNC_INTERVAL=30s
```

Validation:

- `sqlite`: `BACKUP_DB_PATH` required (existing default path is fine). URL/token ignored.
- `turso`: URL and token required. Path not used for the connection. Encryption key must come from `BACKUP_ENCRYPTION_KEY` (no `encryption.key` beside a local db file).
- `turso-sync`: path, URL, and token required. Invalid interval fails startup.

`BACKUP_ENCRYPTION_KEY` remains host-local. Turso stores application-level ciphertext only (repo passwords, rclone configs). Key resolution for `sqlite` and `turso-sync` stays: env var, then `<db_dir>/encryption.key`, then auto-generate. For `turso`, env var is required; do not auto-generate a key that is not persisted anywhere durable.

## Startup, sync loop, and failure modes

The HTTP and gRPC request path never calls Turso in `turso-sync`. All queries use `database/sql` against the local file. Sync is a background loop.

### Startup

| Driver | Remote | Local file | Result |
|---|---|---|---|
| `sqlite` | n/a | create or open | Current behavior |
| `turso` | reachable | n/a | Open remote, run migrations |
| `turso` | down | n/a | Refuse to start |
| `turso-sync` | reachable | empty or existing | `Pull()`, migrate locally, `Push()` |
| `turso-sync` | down | existing DB | Start, warn, retry sync |
| `turso-sync` | down | missing or empty | Refuse to start |

An empty local file plus a down remote must not start: that would create a divergent empty database.

SQLite PRAGMAs (`journal_mode=WAL`, `busy_timeout`, `foreign_keys`) apply to the `sqlite` backend only. Remote-only skips them. Sync applies only pragmas the local Turso engine accepts. Additive migrations that use `PRAGMA table_info` must go through a helper that works on all three backends (or skip the check when the dialect has no pragma and rely on `CREATE` / guarded `ALTER`).

### Sync loop (`turso-sync` only)

- Every `BACKUP_DB_SYNC_INTERVAL` (default 30s): `Pull()` then `Push()`.
- On `Close()` / process shutdown: best-effort `Push()`.
- Sync errors are logged and recorded as last-success / last-error timestamps. They must not fail API or gRPC handlers.
- A later health/UI surface can read those timestamps; v1 logging is enough.

### Durability

- Unsynced local writes are as durable as today’s SQLite file. A crash before `Push()` loses nothing locally; Turso is behind until the next successful push.
- A new host with no local file and no reachable Turso cannot start.

## Code structure

Keep the existing `*database.DB` query API. Introduce a thin open layer; do not split CRUD per backend.

- `server/internal/config`: add `Driver`, `DBURL`, `AuthToken`, `SyncInterval`; keep `DBPath` and `EncryptionKey`.
- `server/internal/database`: `New` takes options (not only path + key). Open the selected backend, migrate, start the sync loop when driver is `turso-sync`.
- `server/cmd/server`: pass full config; `Close()` owns best-effort push.

Suggested files (names may shift slightly):

- `server/internal/database/backend.go` — driver selection and open
- `server/internal/database/sync.go` — pull/push loop and status
- Existing `db.go`, `migrations.go`, and entity files stay the CRUD/migration home

Connection pool: keep current sqlite pool settings for `sqlite`. Remote-only should use a smaller pool appropriate for HTTP/WebSocket (set explicitly in the open path; do not reuse the sqlite 25-open default blindly). Sync uses the local engine’s constraints (treat like a local file: conservative open/idle counts).

## Migration runbook

v1 is a **manual maintenance window**. No automatic rewrite on first connect. Agent `state.db` is not migrated.

**Before cutover:** stop the server. Keep the old `server.db` and encryption key until agents reconnect and at least one job report is stored.

### Production path: `sqlite` → `turso-sync`

1. Create an empty Turso Database (Turso Database engine, not a legacy libSQL-only database).
2. Stop the server.
3. **Preferred, if the spike confirms it:** point `BACKUP_DB_DRIVER=turso-sync` at the existing `BACKUP_DB_PATH`, add URL + token, start. First successful `Push()` is the cutover.
4. **Fallback, if tursogo cannot adopt a `modernc` SQLite file:** import the file into Turso (`turso db import` or dump/apply), set `BACKUP_DB_PATH` to a new empty local path, start `turso-sync` so `Pull()` bootstraps the file.
5. Leave the previous `server.db` offline as rollback.
6. Verify: UI data present, agents connected, one backup report stored.

The adopt-versus-import check is an implementation spike. Docs must state the one supported method after that check; operators do not guess.

### Other switches

| From → to | Procedure |
|---|---|
| `sqlite` → `turso` | Import file into Turso, switch driver to `turso` |
| `turso` → `turso-sync` | New local path, start sync, `Pull()` bootstraps, local becomes primary |
| `turso-sync` → `turso` | `Push()` until caught up, stop, switch to remote-only |
| `turso-sync` → `sqlite` | Only if the local file opens as plain SQLite; otherwise export from Turso and open as `sqlite` |
| Rollback | Restore previous driver + files/env. Never start two writers on the same cloud DB |

### Development

Default `.env.dev` stays `sqlite`. Add a commented block for `turso` and `turso-sync` against a throwaway cloud database.

## Testing and CI

- Default `just test-server` / `go test -race ./...` remains temp-file `sqlite`. No Turso credentials required.
- New unit tests: config validation per driver; startup matrix (refuse empty local + down remote; start with existing local + down remote); request path does not fail when sync errors.
- One integration test: migrate + insert a core row (agent) + read back on a non-sqlite backend. Runs when URL/token (or a local Turso sync server) is available; otherwise skip.
- Current GitHub `test-server` job stays sqlite-only. A gated CI step or later job may run the integration test when secrets or a local sync server exist. v1 acceptance: sqlite always green, plus at least one automated non-sqlite path (local tursogo sync server is enough; Turso Cloud is not required in CI).

## Documentation updates (implementation)

- `docs/database-schema.md` — note three server backends; agent schema unchanged.
- New short ops section or page: env vars, startup table, migration runbook, encryption-key warning.
- `.env.dev` — optional Turso examples.
- This spec remains the design record for #231.

## Acceptance criteria

- Existing SQLite installs work with no new required env vars.
- Server starts against remote-only Turso with documented URL + token.
- Server starts in `turso-sync` and continues read/write if Turso drops after a local file exists.
- `turso-sync` refuses to start with an empty/missing local file when Turso is down.
- Migrations apply on sqlite and on the exercised Turso path.
- Migration runbook exists for sqlite → turso-sync.
- At least one automated test exercises a non-sqlite backend.
- Agent outbox / `state.db` behavior unchanged.

## Risks

- **File-format adopt:** tursogo may not open an existing `modernc` SQLite file. Fallback is import + `Pull()` bootstrap.
- **SQL dialect:** Turso Database is SQLite-compatible but not identical. `PRAGMA table_info` and datetime defaults need a compatibility audit during implementation.
- **Driver maturity:** `tursogo` / `tursogo-serverless` are newer than `modernc`. Pin versions; keep sqlite as the default and CI baseline.
- **Split brain:** two writers on one cloud DB. Documented as unsupported in v1.
- **Key loss:** ciphertext in Turso is unreadable without the host key. Remote-only must not auto-generate an ephemeral key.
