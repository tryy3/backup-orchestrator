# Agent — Further Considerations

Items surfaced during the internal code review that are worth exploring but fall outside the immediate fix plan. These are not bugs — they are design decisions and hardening opportunities.

---

## 1. Hook Template Engine Safety

**Current state**: Hook commands use Go's `text/template` for variable expansion in `executor/hooks.go`. The `HookContext` struct only has string fields, so it's safe today.

**Risk**: `text/template` can call methods on the context struct. If methods are ever added to `HookContext` (or it's changed to embed a struct that has methods), those methods become callable from hook templates authored on the server side. This is a potential code execution vector.

**Options to explore**:
- Switch to a simpler `strings.Replacer` approach (e.g., `{{.PlanName}}` → literal replacement, no template logic).
- Keep `text/template` but add an explicit allowlist via a custom `template.FuncMap`.
- Document the constraint: "`HookContext` must remain a plain struct with exported string fields only."
- Consider `html/template` for auto-escaping if hook output ever appears in HTML contexts.

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

**Current state**: The scheduler tracks a single `currentJob` and reports it in heartbeats. `TriggerNow` fires a goroutine, so it's possible for a manual trigger and a scheduled backup to overlap. The `currentJob` tracking would overwrite the first job's status.

**Options to explore**:
- Decide on a policy: allow concurrent jobs, or queue/reject overlapping triggers.
- If allowing concurrency, change `currentJob` to a slice/map and report all running jobs.
- If serializing, add a job queue with a single worker goroutine.
- Consider per-repository locking — restic itself can't safely run concurrent operations on the same repo.

---

## 6. Sensitive Data in Log Buffer

**Current state**: The `BufferHandler` captures all log entries and ships them to the server as part of job reports. The `redact` package handles restic CLI args and env vars, but log messages from hooks or restic stderr could contain passwords, paths, or other sensitive data.

**Options to explore**:
- Add a `redact.LogEntry()` pass over the buffer before shipping.
- Scrub known patterns (passwords, tokens, keys) from log message text and attributes.
- Allow the server-side config to specify redaction patterns.
