# Serialized Config Push + Push-on-Connect Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate concurrent config-push races with a per-agent coalescing queue, and push latest config when an approved agent connects — closing the race and offline-edit gap in [#47](https://github.com/tryy3/backup-orchestrator/issues/47) without proto/revision changes.

**Architecture:** Add an injectable `pushQueue` in `server/internal/configpush` that serializes builds per agent and coalesces bursts (`Request` → at most one in-flight + one follow-up). `Resolver.RequestPush` is the only production entry point; `PushConfigToAgent` remains the worker body. API handlers switch from `go PushConfigToAgent` to `RequestPush`. `grpcserver.Connect` calls `RequestPush` after `mgr.Register` for approved agents.

**Tech Stack:** Go 1.26, `sync`, `context`, `log/slog`, testify; existing `agentmgr` + SQLite resolver path unchanged.

**Spec:** [docs/superpowers/specs/2026-08-29-config-push-serialization-design.md](../specs/2026-08-29-config-push-serialization-design.md)

## Global Constraints

- No protobuf / agent protocol changes; keep `config_version` + `ConfigAck` as today.
- No revision/heartbeat sync (deferred).
- No durable dirty flag in SQLite; reconnect always requests a push.
- Prefer immediacy: **no debounce** in v1 (pending coalescing alone is enough).
- Worker must use `context.Background()` (or resolver-owned ctx), never the HTTP request context.
- Prefer root `just` recipes: `just test-server`, `just fmt`, `just vet`.
- Do not commit secrets.

## File map

| File | Responsibility |
|---|---|
| `server/internal/configpush/queue.go` | Per-agent inFlight/pending coalescing; `Request(agentID)` |
| `server/internal/configpush/queue_test.go` | Coalescing / single-flight / multi-agent tests with fake pusher |
| `server/internal/configpush/resolver.go` | Embed queue; add `RequestPush`; change `PushConfigToAllAgents` to `RequestPush` |
| `server/internal/api/plans.go` | Replace `go PushConfigToAgent` with `RequestPush` |
| `server/internal/api/hooks.go` | `pushConfigForPlan` → `RequestPush` (no nested `go`) |
| `server/internal/api/repositories.go` | Replace push helpers / goroutines with `RequestPush` |
| `server/internal/api/scripts.go` | Same |
| `server/internal/api/agents.go` | Same |
| `server/internal/api/settings.go` | `PushConfigToAllAgents` stays; it now queues internally |
| `server/internal/grpcserver/connect.go` | `RequestPush` after Register when approved |
| `server/internal/grpcserver/connect_push.go` | Tiny helper `requestConfigPushOnConnect` for testability |
| `server/internal/grpcserver/connect_push_test.go` | Approved vs pending/rejected push behavior |
| `docs/agent-server-design.md` | Document serialized push + push-on-connect |
| `docs/server-further-consideration.md` | Mark config push race item resolved / point to fix |

---

### Task 1: Per-agent push queue (TDD)

**Files:**
- Create: `server/internal/configpush/queue.go`
- Create: `server/internal/configpush/queue_test.go`

**Interfaces:**
- Consumes: nothing outside stdlib
- Produces:
  - `type pushFunc func(ctx context.Context, agentID string) error`
  - `type pushQueue struct` (unexported)
  - `func newPushQueue(push pushFunc) *pushQueue`
  - `func (q *pushQueue) Request(agentID string)` — non-blocking; starts worker if needed

- [ ] **Step 1: Write the failing tests**

Create `server/internal/configpush/queue_test.go`:

```go
package configpush

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushQueue_SingleFlightPerAgent(t *testing.T) {
	t.Parallel()

	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	var calls atomic.Int32

	started := make(chan struct{})
	release := make(chan struct{})

	q := newPushQueue(func(ctx context.Context, agentID string) error {
		calls.Add(1)
		c := concurrent.Add(1)
		for {
			old := maxConcurrent.Load()
			if c <= old || maxConcurrent.CompareAndSwap(old, c) {
				break
			}
		}
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		concurrent.Add(-1)
		return nil
	})

	q.Request("agent-1")
	q.Request("agent-1")
	q.Request("agent-1")

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("push never started")
	}

	// Still held in first push; more Requests only set pending.
	q.Request("agent-1")
	assert.Equal(t, int32(1), maxConcurrent.Load())

	close(release)

	require.Eventually(t, func() bool {
		return calls.Load() == 2 // one in-flight + one coalesced follow-up
	}, 2*time.Second, 10*time.Millisecond)

	assert.Equal(t, int32(1), maxConcurrent.Load(), "never run two pushes concurrently for same agent")
}

func TestPushQueue_DifferentAgentsRunInParallel(t *testing.T) {
	t.Parallel()

	var gate sync.WaitGroup
	gate.Add(2)
	entered := make(chan string, 2)

	q := newPushQueue(func(ctx context.Context, agentID string) error {
		entered <- agentID
		gate.Done()
		gate.Wait() // both must enter before either finishes
		return nil
	})

	q.Request("a")
	q.Request("b")

	require.Eventually(t, func() bool {
		return len(entered) == 2
	}, 2*time.Second, 10*time.Millisecond)
}

func TestPushQueue_PushErrorStillClearsInFlight(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	q := newPushQueue(func(ctx context.Context, agentID string) error {
		calls.Add(1)
		return errors.New("push failed")
	})

	q.Request("agent-1")
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, 5*time.Millisecond)

	q.Request("agent-1")
	require.Eventually(t, func() bool { return calls.Load() == 2 }, time.Second, 5*time.Millisecond)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/configpush/ -run TestPushQueue -count=1`

Expected: FAIL — `newPushQueue` undefined.

- [ ] **Step 3: Implement the queue**

Create `server/internal/configpush/queue.go`:

```go
package configpush

import (
	"context"
	"log/slog"
	"sync"
)

// pushFunc builds and sends config for one agent. Used so tests can inject a fake.
type pushFunc func(ctx context.Context, agentID string) error

type agentPushState struct {
	inFlight bool
	pending  bool
}

// pushQueue serializes config pushes per agent and coalesces bursts.
type pushQueue struct {
	mu     sync.Mutex
	agents map[string]*agentPushState
	push   pushFunc
}

func newPushQueue(push pushFunc) *pushQueue {
	return &pushQueue{
		agents: make(map[string]*agentPushState),
		push:   push,
	}
}

// Request schedules a config push for agentID. Non-blocking.
func (q *pushQueue) Request(agentID string) {
	q.mu.Lock()
	st := q.agents[agentID]
	if st == nil {
		st = &agentPushState{}
		q.agents[agentID] = st
	}
	if st.inFlight {
		st.pending = true
		q.mu.Unlock()
		return
	}
	st.inFlight = true
	q.mu.Unlock()

	go q.worker(agentID)
}

func (q *pushQueue) worker(agentID string) {
	for {
		q.mu.Lock()
		st := q.agents[agentID]
		st.pending = false
		q.mu.Unlock()

		if err := q.push(context.Background(), agentID); err != nil {
			slog.Error("config push failed", "agent_id", agentID, "error", err)
		}

		q.mu.Lock()
		st = q.agents[agentID]
		if st.pending {
			q.mu.Unlock()
			continue
		}
		st.inFlight = false
		q.mu.Unlock()
		return
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/configpush/ -run TestPushQueue -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/configpush/queue.go server/internal/configpush/queue_test.go
git commit -m "$(cat <<'EOF'
feat(server): add per-agent coalescing config push queue

EOF
)"
```

---

### Task 2: Wire queue into Resolver

**Files:**
- Modify: `server/internal/configpush/resolver.go`

**Interfaces:**
- Consumes: `newPushQueue`, `pushQueue.Request` from Task 1
- Produces:
  - `func (r *Resolver) RequestPush(agentID string)`
  - `PushConfigToAllAgents` uses `RequestPush` for each approved agent (keep `IsOnline` filter to avoid useless workers for offline fleet on settings updates; Connect covers offline→online)

- [ ] **Step 1: Extend Resolver struct and New**

In `resolver.go`, change:

```go
// Resolver builds and pushes config to agents.
type Resolver struct {
	db    *database.DB
	mgr   *agentmgr.Manager
	queue *pushQueue
}

// New creates a new config resolver.
func New(db *database.DB, mgr *agentmgr.Manager) *Resolver {
	r := &Resolver{db: db, mgr: mgr}
	r.queue = newPushQueue(r.PushConfigToAgent)
	return r
}

// RequestPush schedules a non-blocking, coalesced config push for agentID.
func (r *Resolver) RequestPush(agentID string) {
	r.queue.Request(agentID)
}
```

- [ ] **Step 2: Update PushConfigToAllAgents**

Replace the body loop so it queues instead of calling `PushConfigToAgent` inline (which would bypass coalescing if someone also `RequestPush`es concurrently):

```go
// PushConfigToAllAgents schedules a config push for every connected approved agent.
func (r *Resolver) PushConfigToAllAgents(ctx context.Context) {
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		slog.Error("failed to list agents for config push", "error", err)
		return
	}

	for _, a := range agents {
		if a.Status == "approved" && r.mgr.IsOnline(a.ID) {
			r.RequestPush(a.ID)
		}
	}
}
```

Note: `ctx` remains on the signature for call-site compatibility but the worker uses `context.Background()` inside `PushConfigToAgent` via the queue.

- [ ] **Step 3: Run existing resolver tests**

Run: `cd server && go test ./internal/configpush/ -count=1`

Expected: PASS (existing tests still call `PushConfigToAgent` directly).

- [ ] **Step 4: Commit**

```bash
git add server/internal/configpush/resolver.go
git commit -m "$(cat <<'EOF'
feat(server): expose RequestPush on config resolver

EOF
)"
```

---

### Task 3: Migrate API call sites to RequestPush

**Files:**
- Modify: `server/internal/api/plans.go`
- Modify: `server/internal/api/hooks.go`
- Modify: `server/internal/api/repositories.go`
- Modify: `server/internal/api/scripts.go`
- Modify: `server/internal/api/agents.go`
- Modify: `server/internal/api/settings.go` (only if it still wraps incorrectly; `go resolver.PushConfigToAllAgents` is OK — that method now queues)

**Interfaces:**
- Consumes: `(*configpush.Resolver).RequestPush(agentID string)`
- Produces: no new API types; handlers no longer spawn their own push goroutines for single-agent pushes

- [ ] **Step 1: Update plans.go**

Replace each:

```go
go func() {
    if err := resolver.PushConfigToAgent(context.Background(), p.AgentID); err != nil {
        slog.Error(...)
    }
}()
```

with:

```go
resolver.RequestPush(p.AgentID)
```

Apply for create / update / delete plan handlers (all three sites). Remove unused `context` import only if no longer needed in that file.

- [ ] **Step 2: Update hooks.go `pushConfigForPlan`**

Replace the whole helper with:

```go
// pushConfigForPlan looks up the plan's agent and schedules a coalesced config push.
func pushConfigForPlan(ctx context.Context, db *database.DB, resolver *configpush.Resolver, planID string) {
	plan, err := db.GetPlan(ctx, planID)
	if err != nil {
		slog.Error("failed to get plan for config push", "plan_id", planID, "error", err)
		return
	}
	if plan == nil {
		return
	}
	resolver.RequestPush(plan.AgentID)
}
```

- [ ] **Step 3: Update repositories.go**

- Create/delete agent-scoped repo: `resolver.RequestPush(*repo.AgentID)` / `resolver.RequestPush(agentID)` instead of `go PushConfigToAgent`.
- `pushConfigToAgentsUsingRepo`: for each agent ID, `resolver.RequestPush(agentID)` (drop per-agent error logging on push — queue logs failures). Keep DB lookup error logging.

Replace the body of `pushConfigToAgentsUsingRepo` with:

```go
func pushConfigToAgentsUsingRepo(ctx context.Context, db *database.DB, resolver *configpush.Resolver, repoID string) {
	agentIDs, err := db.AgentIDsUsingRepository(ctx, repoID)
	if err != nil {
		slog.Error("error finding agents for repo", "repo_id", repoID, "error", err)
		return
	}
	for _, agentID := range agentIDs {
		resolver.RequestPush(agentID)
	}
}
```

Keep `go pushConfigToAgentsUsingRepo(...)` on the update handler so HTTP returns quickly.

For `deleteRepositoryHandler`, replace the per-agent `go PushConfigToAgent` loop with:

```go
for _, agentID := range agentIDs {
	resolver.RequestPush(agentID)
}
```

- [ ] **Step 4: Update scripts.go**

Replace the body of `pushConfigToAgentsUsingScript` with:

```go
func pushConfigToAgentsUsingScript(ctx context.Context, db *database.DB, resolver *configpush.Resolver, scriptID string) {
	agentIDs, err := db.AgentIDsUsingScript(ctx, scriptID)
	if err != nil {
		slog.Error("error finding agents for script", "script_id", scriptID, "error", err)
		return
	}
	for _, agentID := range agentIDs {
		resolver.RequestPush(agentID)
	}
}
```

Keep any outer `go pushConfigToAgentsUsingScript(...)` on the update handler.

- [ ] **Step 5: Update agents.go**

Replace all four `go func() { PushConfigToAgent ... }()` blocks (approve, rclone, command timeouts, outbox overrides) with `resolver.RequestPush(id)`.

- [ ] **Step 6: Verify settings.go**

Keep:

```go
go resolver.PushConfigToAllAgents(context.Background())
```

No change required if Task 2 updated `PushConfigToAllAgents`.

- [ ] **Step 7: Grep for leftover direct production pushes**

Run: `rg 'PushConfigToAgent' server/internal/api server/internal/grpcserver`

Expected: no matches in `api/` or `grpcserver/` (tests under `configpush/` may still call it).

Run: `rg 'go resolver\.Push' server/`

Expected: only `settings.go` → `PushConfigToAllAgents` (and possibly repo/script helper `go` wrappers that only schedule lookups).

- [ ] **Step 8: Run API + configpush tests**

Run: `cd server && go test ./internal/api/ ./internal/configpush/ -count=1`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add server/internal/api/plans.go server/internal/api/hooks.go \
  server/internal/api/repositories.go server/internal/api/scripts.go \
  server/internal/api/agents.go
git commit -m "$(cat <<'EOF'
refactor(server): route config pushes through RequestPush

EOF
)"
```

---

### Task 4: Push config on Connect for approved agents

**Files:**
- Create: `server/internal/grpcserver/connect_push.go`
- Create: `server/internal/grpcserver/connect_push_test.go`
- Modify: `server/internal/grpcserver/connect.go`

**Interfaces:**
- Consumes: `(*configpush.Resolver).RequestPush`
- Produces:
  - `func requestConfigPushOnConnect(resolver *configpush.Resolver, agentID, status string)`
  - Connect calls it **after** `s.mgr.Register(agentID, sendCh)`

- [ ] **Step 1: Write failing helper tests**

Create `server/internal/grpcserver/connect_push_test.go`:

```go
package grpcserver

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tryy3/backup-orchestrator/server/internal/agentmgr"
	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
)

func TestRequestConfigPushOnConnect_ApprovedSchedulesPush(t *testing.T) {
	t.Parallel()
	db, err := database.New(filepath.Join(t.TempDir(), "t.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	ctx := t.Context()
	require.NoError(t, db.CreateAgent(ctx, &database.Agent{
		ID: "agent-1", Name: "a", Hostname: "h", Status: "approved",
	}))

	mgr := agentmgr.New()
	sendCh := make(chan *backupv1.ServerMessage, 1)
	mgr.Register("agent-1", sendCh)
	t.Cleanup(func() { mgr.Unregister("agent-1") })

	resolver := configpush.New(db, mgr)
	requestConfigPushOnConnect(resolver, "agent-1", "approved")

	select {
	case msg := <-sendCh:
		require.NotNil(t, msg.GetConfig())
	case <-time.After(2 * time.Second):
		t.Fatal("expected config push for approved agent")
	}
}

func TestRequestConfigPushOnConnect_PendingDoesNotPush(t *testing.T) {
	t.Parallel()
	db, err := database.New(filepath.Join(t.TempDir(), "t.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mgr := agentmgr.New()
	sendCh := make(chan *backupv1.ServerMessage, 1)
	mgr.Register("agent-1", sendCh)
	t.Cleanup(func() { mgr.Unregister("agent-1") })

	resolver := configpush.New(db, mgr)
	requestConfigPushOnConnect(resolver, "agent-1", "pending")

	select {
	case <-sendCh:
		t.Fatal("pending agent must not receive config")
	case <-time.After(100 * time.Millisecond):
	}
}
```

If `t.Context()` is unavailable on the project's Go version in CI, use `context.Background()` instead.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/grpcserver/ -run TestRequestConfigPushOnConnect -count=1`

Expected: FAIL — `requestConfigPushOnConnect` undefined.

- [ ] **Step 3: Implement helper**

Create `server/internal/grpcserver/connect_push.go`:

```go
package grpcserver

import "github.com/tryy3/backup-orchestrator/server/internal/configpush"

// requestConfigPushOnConnect schedules a config push after an agent connects.
// Must be called only after agentmgr.Register so IsOnline is true.
func requestConfigPushOnConnect(resolver *configpush.Resolver, agentID, status string) {
	if status != "approved" {
		return
	}
	resolver.RequestPush(agentID)
}
```

- [ ] **Step 4: Call from Connect**

In `connect.go`, immediately after `s.mgr.Register(agentID, sendCh)` and the deferred Unregister, add:

```go
	requestConfigPushOnConnect(s.resolver, agentID, agent.Status)
```

Place it **before** starting send/recv goroutines is fine: `RequestPush` is async and `Send` only needs the agent registered (already true). Prefer calling it right after Register / logging so ordering vs first heartbeat does not matter.

Do **not** call it before `Register`.

- [ ] **Step 5: Run tests**

Run: `cd server && go test ./internal/grpcserver/ ./internal/configpush/ -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/internal/grpcserver/connect.go \
  server/internal/grpcserver/connect_push.go \
  server/internal/grpcserver/connect_push_test.go
git commit -m "$(cat <<'EOF'
feat(server): push latest config when approved agent connects

EOF
)"
```

---

### Task 5: Docs + consideration

**Files:**
- Modify: `docs/agent-server-design.md` (Config Push section)
- Modify: `docs/server-further-consideration.md` (mark race item done)

- [ ] **Step 1: Update agent-server-design.md**

Replace / extend section **3. Config Push** to state:

- Server schedules pushes via a per-agent coalescing queue (`RequestPush`); at most one build/send in flight per agent.
- Config is also pushed when an **approved** agent connects (after stream register), so offline edits apply on reconnect.
- Agent still persists config locally and acks with `config_version`.

Keep the existing message shape examples.

- [ ] **Step 2: Update server-further-consideration.md**

Change the **Config push race condition** item to note it is addressed by serialized `RequestPush` + push-on-connect (link the spec path), or strike through / mark resolved per that doc’s existing style.

- [ ] **Step 3: Full server test + fmt/vet**

Run:

```bash
just test-server
just fmt
just vet
```

Expected: all PASS; fmt makes no lingering diffs (or commit fmt if it rewrites).

- [ ] **Step 4: Final leftover grep**

Run: `rg 'go resolver\.PushConfigToAgent|go func\(\) \{\s*if err := resolver\.PushConfigToAgent' server/`

Expected: no matches.

- [ ] **Step 5: Commit**

```bash
git add docs/agent-server-design.md docs/server-further-consideration.md
git commit -m "$(cat <<'EOF'
docs: document serialized config push and reconnect delivery

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
|---|---|
| Per-agent serialized / coalesced push | Task 1–2 |
| `RequestPush` public entry | Task 2 |
| `PushConfigToAgent` as worker body | Task 2 |
| `PushConfigToAllAgents` uses queue | Task 2 |
| No debounce required | Task 1 (none) |
| Offline no-op send unchanged | unchanged `PushConfigToAgent` |
| Push on Connect after Register for approved | Task 4 |
| Migrate API call sites | Task 3 |
| Coalescing unit tests | Task 1 |
| Connect approved vs pending tests | Task 4 |
| No proto / revision work | Global Constraints |
| Docs | Task 5 |
