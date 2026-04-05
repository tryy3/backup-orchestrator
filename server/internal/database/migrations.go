package database

import "fmt"

const migrationSQL = `
-- ============================================================
-- Agents (must be created before repositories due to FK)
-- ============================================================
CREATE TABLE IF NOT EXISTS agents (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    hostname        TEXT NOT NULL,
    os              TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
    api_key         TEXT,
    agent_version   TEXT,
    restic_version  TEXT,
    rclone_version  TEXT,
    rclone_config   TEXT,
    last_heartbeat  DATETIME,
    last_job_at     DATETIME,
    config_version  INTEGER NOT NULL DEFAULT 0,
    config_applied_at DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ============================================================
-- Repositories
-- ============================================================
CREATE TABLE IF NOT EXISTS repositories (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    scope       TEXT NOT NULL,
    agent_id    TEXT,
    type        TEXT NOT NULL,
    path        TEXT NOT NULL,
    password    TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
);

-- ============================================================
-- Global settings
-- ============================================================
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- ============================================================
-- Scripts (reusable hook definitions)
-- ============================================================
CREATE TABLE IF NOT EXISTS scripts (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL DEFAULT 'command',
    command     TEXT NOT NULL,
    timeout     INTEGER NOT NULL DEFAULT 60,
    on_error    TEXT NOT NULL DEFAULT 'continue',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ============================================================
-- Backup plans
-- ============================================================
CREATE TABLE IF NOT EXISTS backup_plans (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    agent_id            TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    paths               TEXT NOT NULL,
    excludes            TEXT,
    tags                TEXT,
    schedule            TEXT NOT NULL,
    forget_after_backup BOOLEAN NOT NULL DEFAULT 1,
    prune_after_forget  BOOLEAN NOT NULL DEFAULT 1,
    prune_schedule      TEXT,
    retention           TEXT,
    enabled             BOOLEAN NOT NULL DEFAULT 1,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, agent_id)
);

-- Many-to-many: backup plan -> repositories
CREATE TABLE IF NOT EXISTS backup_plan_repositories (
    backup_plan_id  TEXT NOT NULL REFERENCES backup_plans(id) ON DELETE CASCADE,
    repository_id   TEXT NOT NULL REFERENCES repositories(id) ON DELETE RESTRICT,
    PRIMARY KEY (backup_plan_id, repository_id)
);

-- ============================================================
-- Plan hooks (ordered list per plan)
-- ============================================================
CREATE TABLE IF NOT EXISTS plan_hooks (
    id              TEXT PRIMARY KEY,
    backup_plan_id  TEXT NOT NULL REFERENCES backup_plans(id) ON DELETE CASCADE,
    on_event        TEXT NOT NULL,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    script_id       TEXT REFERENCES scripts(id) ON DELETE RESTRICT,
    type            TEXT,
    command         TEXT,
    timeout         INTEGER,
    on_error        TEXT,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (
        (script_id IS NOT NULL AND command IS NULL) OR
        (script_id IS NULL AND command IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_plan_hooks_plan_id ON plan_hooks(backup_plan_id);

-- ============================================================
-- Jobs (history)
-- ============================================================
CREATE TABLE IF NOT EXISTS jobs (
    id          TEXT PRIMARY KEY,
    agent_id    TEXT NOT NULL REFERENCES agents(id),
    plan_id     TEXT REFERENCES backup_plans(id) ON DELETE SET NULL,
    plan_name   TEXT NOT NULL,
    type        TEXT NOT NULL,
    trigger     TEXT NOT NULL,
    status      TEXT NOT NULL,
    started_at  DATETIME NOT NULL,
    finished_at DATETIME,
    log_tail    TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_jobs_agent_id ON jobs(agent_id);
CREATE INDEX IF NOT EXISTS idx_jobs_plan_id ON jobs(plan_id);
CREATE INDEX IF NOT EXISTS idx_jobs_started_at ON jobs(started_at);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);

-- Per-repository results within a job
CREATE TABLE IF NOT EXISTS job_repository_results (
    id              TEXT PRIMARY KEY,
    job_id          TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    repository_id   TEXT NOT NULL REFERENCES repositories(id),
    repository_name TEXT NOT NULL,
    status          TEXT NOT NULL,
    snapshot_id     TEXT,
    error           TEXT,
    files_new       INTEGER,
    files_changed   INTEGER,
    files_unmodified INTEGER,
    bytes_added     INTEGER,
    total_bytes     INTEGER,
    duration_ms     INTEGER
);

CREATE INDEX IF NOT EXISTS idx_job_repo_results_job_id ON job_repository_results(job_id);

-- Hook results within a job
CREATE TABLE IF NOT EXISTS job_hook_results (
    id          TEXT PRIMARY KEY,
    job_id      TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    hook_name   TEXT NOT NULL,
    phase       TEXT NOT NULL,
    status      TEXT NOT NULL,
    error       TEXT,
    duration_ms INTEGER
);

CREATE INDEX IF NOT EXISTS idx_job_hook_results_job_id ON job_hook_results(job_id);
`

// migrate runs all DDL statements to create tables if they don't exist.
func (db *DB) migrate() error {
	if _, err := db.Exec(migrationSQL); err != nil {
		return fmt.Errorf("execute migration SQL: %w", err)
	}
	return nil
}
