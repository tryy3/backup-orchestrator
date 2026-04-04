# Data Models

Core entities and their relationships. These drive the database schema, gRPC messages, and API responses.

## Entity Relationship Overview

```
Server stores:
  Scripts (reusable) ────────────────────────────┐
  Repositories (local + global) ──────────┐      │
  Agents ──────────────────────────────┐  │      │
  BackupPlans ─────────────────────┐   │  │      │
                                   │   │  │      │
  BackupPlan ──────────────────────┼───┼──┤      │
    - assigned to 1 Agent          │   │  │      │
    - targets Repository(s)        │   │  │      │
    - has PlanHooks ───────────────┼───┼──┼──────┤
        (inline or references a Script)          │
    - has RetentionPolicy (or inherits global)   │
                                   │             │
  Jobs ────────────────────────────┘
    - execution of a BackupPlan
    - has JobRepositoryResults (one per repo)
    - has HookResults
```

## Repositories

Repositories have a **scope**: local (bound to one agent) or global (usable by any agent).

```go
type Repository struct {
    ID        string    // UUID
    Name      string    // unique, human-friendly: "local-nas", "s3-offsite"

    // Scope
    Scope     string    // "local" | "global"
    AgentID   string    // set when Scope="local", empty when Scope="global"

    // Restic repo connection
    Type      string    // "local" | "rclone" | "sftp" | "s3" | "b2" | "rest" | "azure" | "gs"
    Path      string    // restic --repo value, e.g. "/mnt/nas/backups", "rclone:s3-backup:bucket/path"
    Password  string    // restic repo password (plain text for MVP)

    // For rclone-backed repos, the rclone remote name is embedded in Path
    // e.g. "rclone:myremote:bucket/path" — rclone config is per-agent

    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**Usage rules:**
- `scope: "local"` + `agent_id` set → only plans on that agent can use this repo
- `scope: "global"` + `agent_id` empty → any agent's plans can use this repo
- When creating a backup plan in the UI, the repo picker shows: agent's local repos + all global repos

**Prune considerations for shared repos:**
- When a global repo is used by multiple agents, prune should NOT run after each backup (multiple agents pruning concurrently is problematic)
- For shared repos, prune should be scheduled separately — either a dedicated cron per repo, or coordinated from the server
- Local repos are safe to prune after each backup (single writer)

## Agents

A registered host running the backup agent.

```go
type Agent struct {
    ID        string    // UUID, assigned at enrollment
    Name      string    // display name, defaults to hostname
    Hostname  string    // reported by agent
    OS        string    // e.g. "linux/amd64"

    Status    string    // "pending" | "approved" | "rejected" | "offline"
    APIKey    string    // issued at approval, used for auth

    AgentVersion  string  // e.g. "0.1.0"
    ResticVersion string  // e.g. "0.17.3"
    RcloneVersion string  // e.g. "1.68.0"

    // rclone config for this agent (raw INI text)
    RcloneConfig  string

    // Last known state
    LastHeartbeat time.Time
    LastJobAt     time.Time

    // Config versioning
    ConfigVersion   int       // latest config version pushed
    ConfigAppliedAt time.Time // when agent last confirmed config

    CreatedAt time.Time
    UpdatedAt time.Time
}
```

## Backup Plans

Defines what to back up, where, and when. 1-to-1 with an agent.

```go
type BackupPlan struct {
    ID        string    // UUID
    Name      string    // unique per agent: "daily-home", "database"
    AgentID   string    // which agent runs this plan

    // What to back up
    Paths     []string  // ["/mnt/backup/home", "/mnt/backup/etc"]
    Excludes  []string  // ["*.tmp", ".cache", "node_modules"]
    Tags      []string  // custom user tags applied to snapshots

    // Where to back up
    RepositoryIDs []string  // references Repository.ID, backs up to all listed

    // When to back up
    Schedule  string    // cron expression: "0 2 * * *"

    // Retention (nil = use global defaults)
    Retention *RetentionPolicy

    // Forget/prune behavior
    ForgetAfterBackup bool    // default: true
    PruneAfterForget  bool    // default: true
    PruneSchedule     string  // separate cron, only if PruneAfterForget is false

    // Hooks for this plan (composition — not inheritance)
    Hooks     []PlanHook

    Enabled   bool
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

## Snapshot Tags

Restic snapshots support tags (`--tag`). We use two layers:

### Automatic tags (v1)

Applied by the agent at backup time, always present. Not stored in config — generated at runtime.

| Tag                         | Example                  | Purpose                              |
|-----------------------------|--------------------------|--------------------------------------|
| `agent:<agent-name>`        | `agent:webserver-01`     | Which host created this snapshot     |
| `plan:<plan-name>`          | `plan:database-daily`    | Which backup plan                    |
| `trigger:<type>`            | `trigger:scheduled`      | Was it scheduled or manual           |

This gives us filtering like `restic snapshots --tag agent:webserver-01` to find all snapshots from a host, even in a shared global repo with multiple agents writing to it.

### Custom tags (v2)

User-defined tags at multiple levels, merged at backup time:

| Level        | Example                          | Applies to                          |
|--------------|----------------------------------|-------------------------------------|
| Global       | `env:production`                 | All snapshots from all agents       |
| Agent        | `location:eu-west`               | All snapshots from this agent       |
| Repository   | `storage:offsite`                | All snapshots going to this repo    |
| Plan         | `type:database`, `critical:yes`  | Snapshots from this plan            |

At backup time, tags merge: `global + agent + repo + plan + automatic`. Plan-level tags take precedence if there are conflicts.

For v1, only automatic tags + plan-level custom tags (the `Tags` field on `BackupPlan`). The global/agent/repo tag levels are v2.

## Retention Policy

```go
type RetentionPolicy struct {
    KeepLast    int  // --keep-last
    KeepHourly  int  // --keep-hourly
    KeepDaily   int  // --keep-daily
    KeepWeekly  int  // --keep-weekly
    KeepMonthly int  // --keep-monthly
    KeepYearly  int  // --keep-yearly
}
```

Global defaults are stored as a server-level setting. Per-plan retention overrides the global.

```go
type GlobalSettings struct {
    DefaultRetention RetentionPolicy
}
```

## Scripts & Hooks

### Design Philosophy: Composition over Inheritance

Instead of global hooks that are force-inherited by all plans, we use **reusable scripts** that plans opt into.

- **Scripts** are reusable building blocks defined at the server level (e.g., "ping healthcheck", "notify discord")
- **Plan hooks** compose from scripts + inline commands — each plan explicitly chooses what it needs
- Editing a script updates it everywhere it's referenced
- No forced inheritance — a plan only runs hooks it explicitly includes

### Scripts (reusable definitions)

```go
type Script struct {
    ID        string    // UUID
    Name      string    // unique: "healthcheck-start", "notify-discord", "dump-postgres"

    // What it does
    Type      string    // "command" for MVP (later: "webhook", "notification")
    Command   string    // shell command to execute

    // Behavior defaults (can be overridden per plan hook)
    Timeout   int       // seconds, default 60
    OnError   string    // "abort" | "continue"

    CreatedAt time.Time
    UpdatedAt time.Time
}
```

Scripts are server-global — any plan on any agent can reference them.

### Plan Hooks (per backup plan)

Each hook on a plan is either an inline command or a reference to a script.

```go
type PlanHook struct {
    ID          string    // UUID
    PlanID      string

    // When to run
    OnEvent     string    // "pre_backup", "post_backup", "on_success", "on_failure"
    SortOrder   int       // execution order within same event

    // What to run — one of:
    ScriptID    string    // reference to Script.ID (use script's command/type)
    // OR inline:
    Type        string    // "command" (only set for inline hooks)
    Command     string    // shell command (only set for inline hooks)

    // Behavior (overrides script defaults if set)
    Timeout     *int      // nil = use script default or 60s
    OnError     *string   // nil = use script default or "continue"
}
```

### Example: How it solves the healthcheck + database dump problem

```
Scripts (defined once, server-global):
  ┌─────────────────────────────────────────────────────┐
  │ "healthcheck-start"  │ command: curl hc-ping.com/id/start │
  │ "healthcheck-done"   │ command: curl hc-ping.com/id      │
  │ "notify-discord"     │ command: curl discord-webhook ...   │
  └─────────────────────────────────────────────────────┘

Plan: "daily-home" (no database)
  hooks:
    ├─ pre_backup:  script → "healthcheck-start"
    ├─ on_success:  script → "healthcheck-done"
    └─ on_failure:  script → "notify-discord"

Plan: "database-daily" (needs dump + same notifications)
  hooks:
    ├─ pre_backup:  inline → "pg_dumpall > /tmp/dump.sql"   ← plan-specific
    ├─ pre_backup:  script → "healthcheck-start"             ← reused
    ├─ post_backup: inline → "rm -f /tmp/dump.sql"           ← plan-specific
    ├─ on_success:  script → "healthcheck-done"              ← reused
    └─ on_failure:  script → "notify-discord"                ← reused
```

No duplication. The healthcheck and discord scripts are defined once. The database plan adds its dump/cleanup inline. Each plan is explicit about what it runs.

### UI Workflow

When adding a hook to a plan in the dashboard:

```
Add Hook to "database-daily"
  ┌──────────────────────────────────────────┐
  │ Event: [pre_backup ▾]                    │
  │                                          │
  │ Source: ○ Use existing script             │
  │           [healthcheck-start ▾]          │
  │                                          │
  │         ● Inline command                 │
  │           [pg_dumpall > /tmp/dump.sql  ] │
  │                                          │
  │ On error: [abort ▾]                      │
  │ Timeout:  [300] seconds                  │
  │                                     [Add]│
  └──────────────────────────────────────────┘
```

## Jobs

A job is a single execution of a backup plan (or a manual trigger like restore).

```go
type Job struct {
    ID        string    // UUID
    AgentID   string
    PlanID    string    // nil for manual operations (e.g. ad-hoc restore)
    PlanName  string    // denormalized for easy display

    Type      string    // "backup" | "forget" | "prune" | "restore"
    Trigger   string    // "scheduled" | "manual"

    Status    string    // "running" | "success" | "partial" | "failed"

    StartedAt  time.Time
    FinishedAt time.Time

    // Per-repository results
    RepositoryResults []JobRepositoryResult

    // Hook execution results
    HookResults []JobHookResult

    // Truncated log output from restic
    LogTail   string
}

type JobRepositoryResult struct {
    RepositoryID   string
    RepositoryName string  // denormalized
    Status         string  // "success" | "failed" | "skipped"
    SnapshotID     string  // short restic snapshot ID (on success)
    Error          string  // error message (on failure)

    // Stats from restic (backup jobs)
    FilesNew       int
    FilesChanged   int
    FilesUnmodified int
    BytesAdded     int64
    TotalBytes     int64
    Duration       time.Duration
}

type JobHookResult struct {
    HookName   string
    Phase      string  // "pre_backup", "post_backup", "on_success", "on_failure"
    Status     string  // "success" | "failed" | "skipped"
    Error      string
    Duration   time.Duration
}
```

## Config Pushed to Agent

When the server pushes config to an agent, it assembles this structure. Scripts referenced by plan hooks are resolved and embedded so the agent doesn't need to query the server for script contents.

```go
type AgentConfig struct {
    ConfigVersion int

    // Repositories this agent needs (its local repos + any global repos its plans use)
    Repositories []Repository

    // Backup plans assigned to this agent
    // Each plan's hooks have scripts resolved inline
    BackupPlans []BackupPlan

    // Global retention defaults
    DefaultRetention RetentionPolicy

    // rclone config (raw INI text)
    RcloneConfig string
}
```

When resolving config for an agent, the server:
1. Collects all plans for the agent
2. For each plan hook that references a script, embeds the script's command/type into the hook
3. Collects all repositories referenced by those plans (local + global)
4. Bundles everything into AgentConfig

The agent doesn't know about scripts vs inline — it just sees a flat list of hooks per plan, each with a command to execute.

## Summary of Relationships

```
Script        1 ──── * PlanHook.ScriptID           (script reused by many hooks)
Repository    1 ──── * BackupPlan.RepositoryIDs    (plan targets multiple repos)
Agent         1 ──── * BackupPlan                  (agent has multiple plans)
Agent         1 ──── * Repository (scope=local)    (agent owns local repos)
BackupPlan    1 ──── * PlanHook                    (plan has ordered hooks)
BackupPlan    1 ──── * Job                         (plan produces jobs over time)
Agent         1 ──── * Job                         (agent produces jobs)
Job           1 ──── * JobRepositoryResult         (job has result per repo)
Job           1 ──── * JobHookResult               (job has result per hook)
```
