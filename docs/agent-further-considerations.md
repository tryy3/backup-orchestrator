# Agent — Further Considerations

Items surfaced during the internal code review that are worth exploring but fall outside the immediate fix plan. These are not bugs — they are design decisions and hardening opportunities.

---

## 1. Hook Template Engine Safety

**Resolution**: Replaced `text/template` with `strings.Replacer` in `executor/hooks.go`. The `expandTemplate` function now performs literal `{{.FieldName}}` substitution over the fixed set of `HookContext` fields. Unknown placeholders are left unchanged rather than causing an error. This eliminates the method-call risk entirely and makes the allowed variable set explicit and auditable.

---

## 2. Local Job History Cleanup

**Current state**: The `local_jobs` table in the agent's SQLite database grows unbounded. Every completed backup job inserts a row, and nothing ever deletes them.

**Impact**: Over time (months/years of scheduled backups), the table will grow to thousands or tens of thousands of rows. SQLite handles this fine performance-wise, but it's unnecessary disk usage and makes queries slower.

**Options to explore**:
- Periodic cleanup: keep last N jobs (e.g., 1000) or last N days (e.g., 90 days).
- Run cleanup after each backup job or on a separate timer.
- Add a `PRAGMA auto_vacuum` or `VACUUM` on startup if rows were deleted.
- Make the retention configurable via the agent config pushed from the server.

---

## 3. Structured Shutdown Ordering

**Current state**: In `main.go`, shutdown happens via:
1. `cancel()` — cancels root context
2. `sched.Stop()` — waits for cron jobs to finish
3. `grpcClient.Close()` — closes gRPC connection (also deferred earlier)
4. `db.Close()` — closes database (also deferred earlier)

**Concern**: After fixing the double-close (Phase 1.3), the ordering should be deliberate:
- The scheduler may still be flushing a report when gRPC is closed.
- The reporter flush goroutine may still be writing to the DB when it's closed.

**Options to explore**:
- Define a clear shutdown sequence: cancel context → wait for scheduler → wait for reporter → close gRPC → close DB.
- Use an `errgroup` or explicit `sync.WaitGroup` to coordinate goroutine shutdown before closing resources.
- Add a short grace period (e.g., 5 seconds) for in-flight operations.

---

## 4. Restic/Rclone Binary Discovery & Validation

**Current state**: The agent assumes `restic` and `rclone` are on `PATH`. If they're missing or the wrong version, the first backup job fails with an opaque exec error.

**Options to explore**:
- On startup, run `restic version` and `rclone version`, parse the output, and log warnings/errors if: (a) the binary is missing, (b) the version is below a minimum supported version.
- Cache the detected versions and send them in register/heartbeat messages (replaces the hardcoded placeholders from Phase 3.2).
- Fail-fast at agent startup if critical binaries are missing.

---

## 5. Concurrent Backup Job Execution

**Status**: Resolved. See the concurrency policy below.

### Policy

- **Different backup plans may run concurrently.** The agent tracks running
  jobs in a per-plan map keyed by plan ID, so two plans can execute at the
  same time without interfering with each other's bookkeeping.
- **The same plan must not run concurrently.** If a trigger (manual or
  scheduled) arrives while a job for that plan is already running, the new
  trigger is **aborted immediately** — no queue, no waiting. The rejection is
  reported to the server as a `JobReport` with `status = "aborted"` so the
  operator can see it in job history.

This matches the common failure mode where a user manually re-triggers a
plan that the cron scheduler has already started (or clicks "backup now"
multiple times in quick succession).

### Implementation notes

- `agent/internal/scheduler/scheduler.go` uses `currentJobs map[string]*JobStatus`
  (plan ID → running job). A `tryStartJob` helper acquires the per-plan slot
  atomically before a goroutine is launched for either the scheduled cron
  callback or a `TriggerNow` request.
- Heartbeat `CurrentJob` (proto `RunningJob`) currently models a single
  running job. When multiple plans are running concurrently, the
  earliest-started one is reported in the heartbeat. The JobReport at
  completion is the authoritative source of truth for each run.
- Server-side `storeJobReport` treats `aborted` reports specially: they
  always create a new job row instead of replacing an existing
  `planned`/`running` job, so the genuinely running job's row is not
  overwritten by the rejection.

### restic repository locking

restic uses file-based locks inside the repository itself:

- `restic backup` acquires a **non-exclusive (shared)** lock, so multiple
  concurrent `backup` operations against the same repository are safe —
  including from different machines.
- `forget`, `prune`, and `check` acquire an **exclusive** lock and will fail
  fast (or wait, depending on flags) if other operations hold a lock.

Because restic handles concurrency correctly at the repository level,
**no agent-level per-repository lock is needed.** If a restic operation
fails because the repository is locked by another process, the error
surfaces as a normal backup failure and it is up to the user to schedule
overlapping routines so that they don't fight for exclusive operations.

---

## 6. Sensitive Data in Log Buffer

**Current state**: The `BufferHandler` captures all log entries and ships them to the server as part of job reports. The `redact` package handles restic CLI args and env vars, but log messages from hooks or restic stderr could contain passwords, paths, or other sensitive data.

**Options to explore**:
- Add a `redact.LogEntry()` pass over the buffer before shipping.
- Scrub known patterns (passwords, tokens, keys) from log message text and attributes.
- Allow the server-side config to specify redaction patterns.
