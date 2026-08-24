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

`BACKUP_ENCRYPTION_KEY` is a 64-character hex string (32 bytes) used for **application-level** ciphertext of repository passwords and rclone configs. The key stays host-local (or is supplied via env for remote-only `turso`).

Only those credential fields are encrypted today. Other control-plane data in the database — including `agents.api_key`, script/hook `command` bodies, paths, and schedules — remains **plaintext** in SQLite and in Turso. Treat `BACKUP_DB_AUTH_TOKEN` as tier-0: possession is equivalent to read/write of the control plane (see [#245](https://github.com/tryy3/backup-orchestrator/issues/245) for expanding at-rest encryption).

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

## Docker / Alpine runtime

`turso-sync` loads a native musl library (`libturso_sync_sdk_kit.so`) that dynamically links `libgcc_s.so.1`. The published server image installs Alpine’s `libgcc` package for this. If you build a custom minimal image and use `BACKUP_DB_DRIVER=turso-sync`, install `libgcc` (for example `apk add libgcc`) or startup panics with “Error loading shared library libgcc_s.so.1”.

Remote-only `turso` (`tursogo-serverless`) does not need that native library.

## One writer

v1 supports **one server writer** per cloud database. Do not run `turso-sync` and `turso` (or two `turso-sync` processes) against the same Turso Database at the same time.

## Migration: `sqlite` → `turso-sync`

v1 is a **manual maintenance window**. Agent `state.db` is **not** migrated.

Plain modernc SQLite files **cannot** be pointed at Turso Sync directly (missing
CDC history). Creating schema locally in a sync file and `Push()`ing has also
triggered Turso engine panics on reopen (`negative root page and a positive
root page`). The supported cutover is:

1. Import rows into Turso **remote-only**
2. Bootstrap a **new empty** local sync file with `Pull()`

### Procedure

1. Create an empty Turso Database (`turso db create <name> --tursodb`), or reuse
   one you are willing to overwrite. Re-runs against a non-empty remote require
   `-force` (the migrator drops app tables remotely).
2. Stop the server.
3. Back up the SQLite file and encryption key:

   ```bash
   cp "$BACKUP_DB_PATH" "${BACKUP_DB_PATH}.pre-turso"
   ```

4. Run the migrator (reads `BACKUP_DB_URL` / `BACKUP_DB_AUTH_TOKEN` from `.env`
   only — do not pass the token on the CLI):

   ```bash
   mv tmp/server.db tmp/server.db.pre-turso
   just migrate-turso-sync tmp/server.db.pre-turso tmp/server.db
   # Re-run / overwrite remote app tables:
   # just migrate-turso-sync tmp/server.db.pre-turso tmp/server.db force
   ```

   Paths may be relative to the repo root or absolute.
5. Set `.env`:

   ```bash
   BACKUP_DB_DRIVER=turso-sync
   BACKUP_DB_PATH=/absolute/path/to/tmp/server.db   # the NEW file from step 4
   BACKUP_DB_URL=...
   BACKUP_DB_AUTH_TOKEN=...
   BACKUP_ENCRYPTION_KEY=...   # same key as before (ciphertext is copied as-is)
   ```

6. Start the server. Verify UI data, agents, and a job report.
7. Keep `*.pre-turso` offline until you are satisfied; then you may archive it.

### If turso-sync panics on open

Quarantine all sync sidecars (`server.db`, `-wal`, `-shm`, `-changes`, `-info`,
`-log`, `-wal-revert`), restore `*.pre-turso` as plain `sqlite`, then re-run the
migrator against a **fresh empty** destination path (and preferably a clean
Turso database if the previous Push left bad remote state).

### Other switches

| From → to | Procedure |
|---|---|
| `sqlite` → `turso` | Migrator step 1 only (remote import), then set `BACKUP_DB_DRIVER=turso` |
| `turso` → `turso-sync` | New empty local path, start sync, `Pull()` bootstraps |
| `turso-sync` → `turso` | `Push()` until caught up, stop, switch to remote-only |
| `turso-sync` → `sqlite` | Only if the local file opens as plain SQLite; otherwise export from Turso |
| Rollback | Restore previous driver + `*.pre-turso` files/env. Never start two writers on the same cloud DB |

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
- [#245](https://github.com/tryy3/backup-orchestrator/issues/245) — expand at-rest encryption for control-plane fields
