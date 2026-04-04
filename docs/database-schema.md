# Database Schema (SQLite)

Server-side database. The agent uses a separate smaller SQLite database for local state.

## Server Database

```sql
-- ============================================================
-- Repositories
-- ============================================================
CREATE TABLE repositories (
    id          TEXT PRIMARY KEY,  -- UUID
    name        TEXT NOT NULL UNIQUE,
    scope       TEXT NOT NULL,     -- "local" | "global"
    agent_id    TEXT,              -- set when scope="local", NULL when scope="global"
    type        TEXT NOT NULL,     -- "local", "rclone", "sftp", "s3", "b2", "rest", "azure", "gs"
    path        TEXT NOT NULL,     -- restic --repo value
    password    TEXT NOT NULL,     -- repo password (plain text for MVP)
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
);

-- ============================================================
-- Agents
-- ============================================================
CREATE TABLE agents (
    id              TEXT PRIMARY KEY,  -- UUID
    name            TEXT NOT NULL,
    hostname        TEXT NOT NULL,
    os              TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',  -- pending, approved, rejected
    api_key         TEXT,              -- issued on approval
    agent_version   TEXT,
    restic_version  TEXT,
    rclone_version  TEXT,
    rclone_config   TEXT,              -- raw INI text
    last_heartbeat  DATETIME,
    last_job_at     DATETIME,
    config_version  INTEGER NOT NULL DEFAULT 0,
    config_applied_at DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ============================================================
-- Global settings
-- ============================================================
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL  -- JSON-encoded
);

-- Stores:
--   "default_retention" -> JSON of RetentionPolicy

-- ============================================================
-- Scripts (reusable hook definitions)
-- ============================================================
CREATE TABLE scripts (
    id          TEXT PRIMARY KEY,  -- UUID
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL DEFAULT 'command',  -- "command" for MVP
    command     TEXT NOT NULL,
    timeout     INTEGER NOT NULL DEFAULT 60,  -- seconds
    on_error    TEXT NOT NULL DEFAULT 'continue',  -- "abort" | "continue"
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ============================================================
-- Backup plans
-- ============================================================
CREATE TABLE backup_plans (
    id                  TEXT PRIMARY KEY,  -- UUID
    name                TEXT NOT NULL,
    agent_id            TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    paths               TEXT NOT NULL,     -- JSON array: ["/mnt/backup/home"]
    excludes            TEXT,              -- JSON array: ["*.tmp", ".cache"]
    tags                TEXT,              -- JSON array: ["daily", "important"]
    schedule            TEXT NOT NULL,     -- cron expression
    forget_after_backup BOOLEAN NOT NULL DEFAULT 1,
    prune_after_forget  BOOLEAN NOT NULL DEFAULT 1,
    prune_schedule      TEXT,              -- cron expression, if prune is separate
    retention           TEXT,              -- JSON of RetentionPolicy, NULL = use global
    enabled             BOOLEAN NOT NULL DEFAULT 1,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, agent_id)
);

-- Many-to-many: backup plan -> repositories
CREATE TABLE backup_plan_repositories (
    backup_plan_id  TEXT NOT NULL REFERENCES backup_plans(id) ON DELETE CASCADE,
    repository_id   TEXT NOT NULL REFERENCES repositories(id) ON DELETE RESTRICT,
    PRIMARY KEY (backup_plan_id, repository_id)
);

-- ============================================================
-- Plan hooks (ordered list per plan — composition model)
-- ============================================================
CREATE TABLE plan_hooks (
    id              TEXT PRIMARY KEY,  -- UUID
    backup_plan_id  TEXT NOT NULL REFERENCES backup_plans(id) ON DELETE CASCADE,
    on_event        TEXT NOT NULL,     -- "pre_backup", "post_backup", "on_success", "on_failure", etc.
    sort_order      INTEGER NOT NULL DEFAULT 0,

    -- Either references a script OR defines inline command (one or the other)
    script_id       TEXT REFERENCES scripts(id) ON DELETE RESTRICT,  -- NULL for inline

    -- Inline hook fields (NULL when using script_id)
    type            TEXT,              -- "command"
    command         TEXT,

    -- Overrides (NULL = use script defaults or system defaults)
    timeout         INTEGER,           -- seconds
    on_error        TEXT,              -- "abort" | "continue"

    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Ensure either script_id or command is set, not both
    CHECK (
        (script_id IS NOT NULL AND command IS NULL) OR
        (script_id IS NULL AND command IS NOT NULL)
    )
);

CREATE INDEX idx_plan_hooks_plan_id ON plan_hooks(backup_plan_id);

-- ============================================================
-- Jobs (history)
-- ============================================================
CREATE TABLE jobs (
    id          TEXT PRIMARY KEY,  -- UUID
    agent_id    TEXT NOT NULL REFERENCES agents(id),
    plan_id     TEXT REFERENCES backup_plans(id) ON DELETE SET NULL,
    plan_name   TEXT NOT NULL,     -- denormalized
    type        TEXT NOT NULL,     -- "backup", "forget", "prune", "restore"
    trigger     TEXT NOT NULL,     -- "scheduled", "manual"
    status      TEXT NOT NULL,     -- "running", "success", "partial", "failed"
    started_at  DATETIME NOT NULL,
    finished_at DATETIME,
    log_tail    TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_jobs_agent_id ON jobs(agent_id);
CREATE INDEX idx_jobs_plan_id ON jobs(plan_id);
CREATE INDEX idx_jobs_started_at ON jobs(started_at);
CREATE INDEX idx_jobs_status ON jobs(status);

-- Per-repository results within a job
CREATE TABLE job_repository_results (
    id              TEXT PRIMARY KEY,
    job_id          TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    repository_id   TEXT NOT NULL REFERENCES repositories(id),
    repository_name TEXT NOT NULL,     -- denormalized
    status          TEXT NOT NULL,     -- "success", "failed", "skipped"
    snapshot_id     TEXT,
    error           TEXT,
    files_new       INTEGER,
    files_changed   INTEGER,
    files_unmodified INTEGER,
    bytes_added     INTEGER,
    total_bytes     INTEGER,
    duration_ms     INTEGER
);

CREATE INDEX idx_job_repo_results_job_id ON job_repository_results(job_id);

-- Hook results within a job
CREATE TABLE job_hook_results (
    id          TEXT PRIMARY KEY,
    job_id      TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    hook_name   TEXT NOT NULL,
    phase       TEXT NOT NULL,     -- "pre_backup", "post_backup", etc.
    status      TEXT NOT NULL,     -- "success", "failed", "skipped"
    error       TEXT,
    duration_ms INTEGER
);

CREATE INDEX idx_job_hook_results_job_id ON job_hook_results(job_id);
```

## Agent Database

Smaller SQLite database on each agent. Stores buffered reports and local job history.

```sql
-- ============================================================
-- Buffered job reports (unsent to server)
-- ============================================================
CREATE TABLE buffered_reports (
    id          TEXT PRIMARY KEY,
    payload     TEXT NOT NULL,     -- JSON-encoded JobReport
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT
);

-- ============================================================
-- Local job history (agent keeps its own record)
-- ============================================================
CREATE TABLE local_jobs (
    id          TEXT PRIMARY KEY,
    plan_name   TEXT NOT NULL,
    type        TEXT NOT NULL,
    status      TEXT NOT NULL,
    started_at  DATETIME NOT NULL,
    finished_at DATETIME,
    log_tail    TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_local_jobs_started_at ON local_jobs(started_at);
```
