# Agent Internal Code Review — Fix Plan

Review of the `agent/` Go project internals. All fixes are internal — no structural or API changes.

**Status**: In Progress

---

## Phase 1: Critical Bugs & Data Races

- [x] **1.1 Data race on `Identity.APIKey` in StreamHandler**
  - `grpcclient/stream.go`: `handleApproval()` writes `s.identity.APIKey` from the recv goroutine while `sendHeartbeat()` reads it from the send goroutine with no synchronization.
  - In `main.go`, `configMu` protects the same field in `reportFn`/`onApproval`, but StreamHandler itself has no lock.
  - Fix: Add a `sync.RWMutex` to `StreamHandler` protecting all identity field reads/writes.

- [x] **1.2 Copy-paste bug: heartbeat sends OS/arch as ResticVersion**
  - `grpcclient/stream.go` `sendHeartbeat()`: `ResticVersion: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)` sends e.g. `"linux/amd64"` instead of a version string.
  - Compare `register.go` where `Os` uses that format and `ResticVersion` is `"0.17.3"`.
  - Fix: Set `ResticVersion: "0.17.3"` to match register.go.

- [x] **1.3 Double-close of gRPC client and database**
  - `cmd/agent/main.go`: Both resources are closed via `defer` near creation AND explicitly in the shutdown section at the bottom. This causes double-close.
  - Fix: Remove either the defers or the explicit calls.

- [x] **1.4 Goroutine leak in `StreamHandler.Run`**
  - `grpcclient/stream.go` `Run()`: `wg` is declared and incremented but never `Wait()`ed. When `Run` returns via the `select`, the other goroutine keeps running.
  - Fix: Create an internal derived context, cancel it when `Run` exits, then `wg.Wait()`.

---

## Phase 2: Robustness & Correctness

- [x] **2.1 Backoff never resets on successful reconnection**
  - `cmd/agent/main.go`: `backoff` starts at 1s, doubles up to 5min, but is never reset after a successful connection. If the stream runs for an hour then disconnects, the next backoff wait is still the doubled value from the previous disconnect.
  - Fix: Reset `backoff = time.Second` at the top of the reconnect loop before calling `Run`.

- [x] **2.2 No context cancellation for commands and scheduled backups**
  - `handleCommand()` passes `context.Background()` for ListSnapshots, BrowseSnapshot, TriggerRestore. Scheduler passes `context.Background()` to `ExecuteBackupJob`. On SIGTERM, restic subprocesses cannot be cancelled.
  - Fix: Thread the root cancellable context through the scheduler and command handler.

- [x] **2.3 `json.Marshal` errors silently ignored in `handleCommand`**
  - `cmd/agent/main.go`: `data, _ := json.Marshal(snapshots)` and `data, _ := json.Marshal(files)` discard marshal errors.
  - Fix: Check the error and return a failed `CommandResult`.

- [x] **2.4 No max retry / dead-letter for buffered reports**
  - `reporter/reporter.go` `flush()`: Failed reports are retried indefinitely with `IncrementAttempts` but no cap. Malformed or permanently rejected reports accumulate forever.
  - Fix: Add a max attempts constant (e.g., 10). Delete reports exceeding it.

- [x] **2.5 SQLite connection pool should be limited to 1**
  - `database/db.go` `Open()`: Default Go connection pool allows multiple connections. SQLite only supports one writer and can return "database is locked" under concurrent access.
  - Fix: Add `sqlDB.SetMaxOpenConns(1)` after opening.

---

## Phase 3: Minor Correctness & Hardening

- [x] **3.1 `BufferHandler.WithGroup` is a no-op**
  - `logging/buffer.go`: `WithGroup` returns `h` unchanged, violating the `slog.Handler` contract. Grouped attributes would be flattened.
  - Fix: Track group prefix and prepend it to attribute keys in `Handle`.

- [x] **3.2 Hardcoded version strings**
  - `grpcclient/register.go` and `grpcclient/stream.go`: Agent, restic, and rclone versions are hardcoded placeholders (`"0.1.0"`, `"0.17.3"`, `"1.68.0"`).
  - Fix: Make agent version a build-time variable via `-ldflags`. For restic/rclone, consider running `restic version` / `rclone version` at startup and caching.

- [x] **3.3 Fragile repo existence check in `EnsureRepo`**
  - `executor/restic.go` `EnsureRepo()`: Checks for repo existence by string-matching stderr output (`"unable to open repository"`, `"Is there a repository at the following location"`). Can break with restic version changes or non-English locales.
  - Fix: Attempt `init` unconditionally and treat "already initialized" as success.

- [x] **3.4 `MultiHandler.Handle` short-circuits on first error**
  - `logging/multi.go`: If the first handler in the list fails, subsequent handlers are skipped. For a multi-handler, all handlers should receive the record.
  - Fix: Call all handlers regardless; collect and return a joined error.

---

## Phase 4: Test Coverage

Only `redact` has tests. Priority targets by risk:

- [x] **4.1** `scheduler` — cron scheduling, UpdateSchedule, TriggerNow
- [x] **4.2** `executor/hooks.go` — template expansion, hook ordering, abort logic
- [x] **4.3** `logging/buffer.go` — entry capture, max entries cap, PlainText output
- [x] **4.4** `database/db.go` — migrations, CRUD operations
- [x] **4.5** `reporter/reporter.go` — buffering, flush cycle, max retries
- [x] **4.6** `identity` — Load/Save round-trip, missing file handling

---

## Verification Checklist

- [x] `cd agent && go vet ./... && go build ./...` passes after each phase
- [x] `go test -race ./...` confirms no data races (after Phase 1)
- [x] Reconnect behavior resets backoff (after 2.1)
- [x] `sched.Stop()` cancels in-progress backup jobs (after 2.2)
- [x] `go test -cover ./...` shows increased coverage (after Phase 4)
