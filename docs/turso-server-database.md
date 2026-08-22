# Turso server database

## File adopt spike

**Outcome: adopt supported.**

Tested with `turso.tech/database/tursogo` v0.7.2 under
`CGO_ENABLED=0`. A database created and migrated by
`modernc.org/sqlite` v1.57.0 was reopened with `TursoSyncDb` using
`BootstrapIfEmpty=false` and an unreachable remote URL. `Connect`
succeeded and a previously inserted row in the `agents` table was readable.

The production migration runbook can therefore point `BACKUP_DB_PATH` at the
existing SQLite file, start the server with the `turso-sync` driver, and push
the adopted local state to the configured remote.
