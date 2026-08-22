# Turso server database

Optional Turso backends for the **server** only. The agent keeps its on-disk SQLite `state.db` for outbox spill; that database is not migrated and does not use Turso.

## Drivers

| Driver | Engine | Process uses | Typical use |
|---|---|---|---|
| `sqlite` (default) | `modernc.org/sqlite` | Local `BACKUP_DB_PATH` | Default, unit tests, simple deploys |
| `turso` | `tursogo-serverless` | Cloud only | Shared test server, no local disk |
| `turso-sync` | `tursogo` `NewTursoSyncDb` | Local file; Turso is replica | Production |

`BACKUP_DB_DRIVER` unset or empty means `sqlite`. Existing SQLite deployments need no new env vars.

## Environment variables

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

- **`sqlite`:** `BACKUP_DB_PATH` required (existing default path is fine). URL and token are ignored.
- **`turso`:** `BACKUP_DB_URL` and `BACKUP_DB_AUTH_TOKEN` required. Path is not used for the connection. `BACKUP_ENCRYPTION_KEY` is required (see below).
- **`turso-sync`:** `BACKUP_DB_PATH`, `BACKUP_DB_URL`, and `BACKUP_DB_AUTH_TOKEN` required. Invalid `BACKUP_DB_SYNC_INTERVAL` fails startup.

## Encryption key

`BACKUP_ENCRYPTION_KEY` is a 64-character hex string (32 bytes) used for application-level ciphertext (repo passwords, rclone configs). It stays host-local; Turso stores encrypted values only.

Resolution order:

| Driver | Order |
|---|---|
| `sqlite`, `turso-sync` | `BACKUP_ENCRYPTION_KEY` env → `<db_dir>/encryption.key` → auto-generate and persist |
| `turso` | `BACKUP_ENCRYPTION_KEY` env only — **required**; no key file beside a local db and no auto-generate |

Losing the key makes stored repository credentials unrecoverable.

## Startup behavior

The HTTP and gRPC request path never calls Turso in `turso-sync`. All queries use `database/sql` against the local file. Sync is a background loop (`Pull` then `Push` every `BACKUP_DB_SYNC_INTERVAL`; best-effort `Push` on shutdown).

| Driver | Remote | Local file | Result |
|---|---|---|---|
| `sqlite` | n/a | create or open | Current behavior |
| `turso` | reachable | n/a | Open remote, run migrations |
| `turso` | down | n/a | Refuse to start |
| `turso-sync` | reachable | empty or existing | `Pull()`, migrate locally, `Push()` |
| `turso-sync` | down | existing DB | Start, warn, retry sync |
| `turso-sync` | down | missing or empty | Refuse to start |

An empty local file plus a down remote must not start: that would create a divergent empty database.

Sync errors are logged and recorded as status; they do not fail API or gRPC handlers.

## One writer

v1 supports **one server writer** per cloud database. Do not run `turso-sync` and `turso` (or two `turso-sync` processes) against the same Turso Database at the same time.

## Migration: `sqlite` → `turso-sync`

v1 is a **manual maintenance window**. No automatic rewrite on first connect. Agent `state.db` is **not** migrated.

### File adopt (supported)

Tested with `turso.tech/database/tursogo` v0.7.2 under `CGO_ENABLED=0`. A database created and migrated by `modernc.org/sqlite` v1.57.0 was reopened with `TursoSyncDb` using `BootstrapIfEmpty=false` and an unreachable remote URL. `Connect` succeeded and a previously inserted row in the `agents` table was readable.

**Procedure:**

1. Create an empty Turso Database (Turso Database engine, not a legacy libSQL-only database).
2. Stop the server.
3. Set `BACKUP_DB_DRIVER=turso-sync`, keep `BACKUP_DB_PATH` pointing at the existing SQLite file, and set `BACKUP_DB_URL` and `BACKUP_DB_AUTH_TOKEN`.
4. Start the server. The first successful `Push()` uploads local state to Turso — that is the cutover.
5. Leave the previous `server.db` copy offline as rollback until agents reconnect and at least one job report is stored.
6. Verify: UI data present, agents connected, one backup report stored.

### Other switches

| From → to | Procedure |
|---|---|
| `sqlite` → `turso` | Import file into Turso, switch driver to `turso` |
| `turso` → `turso-sync` | New local path, start sync, `Pull()` bootstraps, local becomes primary |
| `turso-sync` → `turso` | `Push()` until caught up, stop, switch to remote-only |
| `turso-sync` → `sqlite` | Only if the local file opens as plain SQLite; otherwise export from Turso |
| Rollback | Restore previous driver + files/env. Never start two writers on the same cloud DB |

## Integration test

Default `just test-server` stays sqlite-only. A gated integration test exercises migrate + agent round-trip on a non-sqlite backend when env is set.

**Local `tursodb` sync server:**

```bash
# Terminal 1 — local Turso sync server (install tursodb separately)
tursodb :memory: --sync-server 127.0.0.1:18080

# Terminal 2 — run the gated test
cd server
BACKUP_TEST_TURSO_URL=http://127.0.0.1:18080 \
BACKUP_TEST_TURSO_DRIVER=turso-sync \
  go test ./internal/database/ -count=1 -run TestTursoIntegration
```

Optional: set `BACKUP_TEST_TURSO_AUTH_TOKEN` when the remote requires a token. Set `BACKUP_TEST_TURSO_DRIVER=turso` to exercise remote-only instead of sync.

Without `BACKUP_TEST_TURSO_URL`, the test skips.

## See also

- [database-schema.md](database-schema.md) — shared schema SQL
- [superpowers/specs/2026-08-23-turso-server-database-design.md](superpowers/specs/2026-08-23-turso-server-database-design.md) — design record
