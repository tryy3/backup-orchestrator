# Hooks Design

## Design Philosophy: Composition over Inheritance

The problem with global hooks + inheritance (autorestic model): the moment a plan needs one extra hook, you have to redeclare all the inherited hooks too. Config gets duplicated and messy.

Our approach: **reusable scripts** + **explicit composition per plan**.

- **Scripts** are reusable definitions (server-global): "ping healthcheck", "notify discord", etc.
- **Plan hooks** are an ordered list per backup plan. Each hook either references a script or defines an inline command.
- Each plan explicitly opts into the hooks it needs. No forced inheritance.
- Editing a script updates behavior everywhere it's used.

## Hook Lifecycle Points

```
backup job triggered
  |
  v
PRE_BACKUP hooks (ordered)
  |  (e.g., dump database, ping healthcheck start)
  v
restic backup (per repository, sequentially)
  |
  v
POST_BACKUP hooks (ordered)
  |  (e.g., cleanup temp files, restart service)
  v
ON_SUCCESS hooks  -or-  ON_FAILURE hooks
  |  (e.g., notify Discord, ping healthcheck done)
  v
done
```

### All Hook Events

| Event          | Fires When                                   | Use Cases                                |
|----------------|----------------------------------------------|------------------------------------------|
| `pre_backup`   | Before restic backup starts                  | DB dump, stop service, ping healthcheck  |
| `post_backup`  | After backup completes (success or failure)  | Restart service, cleanup temp files      |
| `on_success`   | After successful backup                      | Notification, healthcheck ping           |
| `on_failure`   | After failed backup                          | Alert, healthcheck fail ping             |
| `pre_restore`  | Before restic restore starts                 | Stop service, create safety backup       |
| `post_restore` | After restore completes                      | Start service, verify data integrity     |
| `pre_forget`   | Before restic forget/prune                   | (rarely used)                            |
| `post_forget`  | After forget/prune completes                 | Notification, repo size check            |

## Scripts (Reusable Definitions)

Scripts are defined at the server level and can be used by any plan on any agent.

```
Scripts:
  "healthcheck-start"   → curl -s https://hc-ping.com/uuid/start
  "healthcheck-done"    → curl -s https://hc-ping.com/uuid
  "healthcheck-fail"    → curl -s https://hc-ping.com/uuid/fail
  "notify-discord"      → curl -s -X POST -H "Content-Type: application/json" -d '...' https://discord.com/api/webhooks/...
  "dump-postgres"       → pg_dumpall -U postgres > /tmp/pg_backup.sql
  "cleanup-postgres"    → rm -f /tmp/pg_backup.sql
```

For MVP, scripts are `type: "command"` only. Later we can add `type: "webhook"` and `type: "notification"` with structured config.

### Template Variables in Scripts

Script commands can use Go template variables:

| Variable        | Description                              |
|-----------------|------------------------------------------|
| `{{.PlanName}}` | Name of the backup plan                  |
| `{{.Hostname}}` | Agent hostname                           |
| `{{.Status}}`   | "success", "partial", "failed"           |
| `{{.Duration}}` | Human-readable duration                  |
| `{{.BytesAdded}}`| Bytes added in this snapshot            |
| `{{.FilesNew}}` | Count of new files                       |
| `{{.FilesChanged}}`| Count of changed files                |
| `{{.SnapshotID}}`| Restic snapshot ID (short)              |
| `{{.Error}}`    | Error message (if failed)                |
| `{{.StartedAt}}`| Job start time                           |
| `{{.FinishedAt}}`| Job end time                            |

Note: pre_backup hooks only have `.PlanName` and `.Hostname` available (backup hasn't run yet).

## Plan Hooks (Per Backup Plan)

Each backup plan has an ordered list of hooks. Each hook is either:
- A **reference to a script** (by name/ID)
- An **inline command** (written directly on the plan)

Both can have per-hook overrides for timeout and on_error behavior.

### Example Configuration

```
Plan: "daily-home"
  hooks:
    1. [pre_backup]  script: "healthcheck-start"
    2. [on_success]  script: "healthcheck-done"
    3. [on_failure]  script: "healthcheck-fail"
    4. [on_failure]  script: "notify-discord"

Plan: "database-daily"
  hooks:
    1. [pre_backup]  inline: "pg_dumpall -U postgres > /tmp/pg_backup.sql"  (on_error: abort)
    2. [pre_backup]  script: "healthcheck-start"
    3. [post_backup] inline: "rm -f /tmp/pg_backup.sql"
    4. [on_success]  script: "healthcheck-done"
    5. [on_failure]  script: "healthcheck-fail"
    6. [on_failure]  script: "notify-discord"

Plan: "media-weekly"
  hooks:
    1. [on_failure]  script: "notify-discord"
    # no healthcheck needed for this one
```

Key points:
- `daily-home` and `database-daily` both use healthcheck + discord, no duplication in script definitions
- `database-daily` adds its own dump/cleanup inline
- `media-weekly` only uses discord notifications — no forced inheritance of healthcheck hooks
- Changing the discord webhook URL means editing one script, not every plan

## Execution Rules

1. Hooks within the same event run sequentially in `sort_order`
2. If a `pre_*` hook with `on_error: "abort"` fails, the operation is cancelled and `on_failure` hooks run
3. `post_*` hooks always run (regardless of success/failure)
4. `on_success` / `on_failure` run after `post_*` hooks, based on the aggregate job result
5. Timeout defaults to 60 seconds per hook
6. Hooks run once per job, not once per repository

## What the Agent Sees

The server resolves script references before pushing config. The agent receives a flat list of hooks per plan:

```json
{
  "plan_name": "database-daily",
  "hooks": [
    { "event": "pre_backup", "command": "pg_dumpall -U postgres > /tmp/pg_backup.sql", "on_error": "abort", "timeout": 300 },
    { "event": "pre_backup", "command": "curl -s https://hc-ping.com/uuid/start", "on_error": "continue", "timeout": 60 },
    { "event": "post_backup", "command": "rm -f /tmp/pg_backup.sql", "on_error": "continue", "timeout": 60 },
    { "event": "on_success", "command": "curl -s https://hc-ping.com/uuid", "on_error": "continue", "timeout": 60 },
    { "event": "on_failure", "command": "curl -s https://hc-ping.com/uuid/fail", "on_error": "continue", "timeout": 60 },
    { "event": "on_failure", "command": "curl -s -X POST ...", "on_error": "continue", "timeout": 60 }
  ]
}
```

The agent doesn't know or care about scripts vs inline. It just executes commands in order.
