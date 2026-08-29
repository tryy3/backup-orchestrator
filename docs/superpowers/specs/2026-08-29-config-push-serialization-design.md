# Serialized config push + push-on-connect

**Date:** 2026-08-29  
**Status:** Approved for implementation planning  
**Issue:** [#47](https://github.com/tryy3/backup-orchestrator/issues/47)  
**Scope:** Server-side config push concurrency + reconnect delivery (no proto / agent protocol changes)

## Summary

Replace fire-and-forget concurrent `PushConfigToAgent` goroutines with a **per-agent serialized push queue** that coalesces bursts into at most one in-flight build and one follow-up. Keep near-immediate delivery when the agent is online. When the agent is offline, entity writes already persist in SQLite; **push the latest resolved config as soon as the agent reconnects** (Connect register path). Revision-based heartbeat sync and desired-vs-applied observability are explicitly deferred.

## Goals

- Eliminate the race where two concurrent pushes build different snapshots and assign versions such that stale config can overwrite fresher config.
- Keep delivery as fast as practical when the agent is online (sub-second; no heartbeat-interval delay).
- Ensure offline config edits apply on reconnect without requiring another API write while the agent is online.
- Preserve existing `config_version` / `ConfigAck` protocol and agent apply behavior.

## Non-goals

- Heartbeat `config_revision` field or other proto changes ([#47](https://github.com/tryy3/backup-orchestrator/issues/47) “proposed approach” deferred).
- Desired-vs-applied revision observability in the UI or API.
- Per-agent full revision history table.
- Changing how `Command` messages are sent (trigger backup, queries, etc. stay on `agentmgr.SendCommand`).
- Agent-side “ignore older version” hardening (optional follow-up; not required once server serializes pushes).

## Problem (current state)

`PushConfigToAgent` builds a full config snapshot from the DB, then increments `config_version`, then sends. Many API handlers invoke it via `go resolver.PushConfigToAgent(...)`.

Under concurrent updates for the same agent:

1. Push A builds snapshot from older DB state; Push B builds newer state.
2. Both increment `config_version` (now atomic in `UpdateConfigVersion`).
3. If A finishes the increment/send after B, the agent can receive **stale content with a higher version**.

`UpdateConfigVersion` alone does not fix snapshot freshness.

Additionally, when the agent is offline, `PushConfigToAgent` returns success without sending (`IsOnline` early return). Entity rows are updated, but **Connect does not push config**, so the agent keeps last-known local config until a later online write triggers a push.

## Decision

Adopt **option A** from the issue discussion: per-agent serialized / coalesced push. Add **push-on-connect** for approved agents so DB is the source of truth across offline edits.

Defer hybrid revision + heartbeat sync (option D) until observability needs justify proto and agent changes.

## Architecture

### Public API on `configpush.Resolver`

| Method | Behavior |
|---|---|
| `RequestPush(agentID string)` | Non-blocking. Marks the agent as needing a push and ensures a worker will run. Safe to call from HTTP handlers, hooks, settings, and Connect. |
| `PushConfigToAgent(ctx, agentID)` | Existing build → bump version → send. Becomes the **worker body** only; call sites should not invoke it directly from `go` except tests that need a synchronous push. |
| `PushConfigToAllAgents(ctx)` | Lists approved agents and calls `RequestPush` per agent (or equivalent), not unbounded concurrent `PushConfigToAgent` goroutines. |

Exact names may vary slightly in implementation; semantics above are required.

### Per-agent coalescing

Maintain per-agent state (mutex-protected map keyed by `agentID`):

- **inFlight** — a push is currently running for this agent.
- **pending** — at least one `RequestPush` arrived while inFlight (or during debounce).

Algorithm:

1. `RequestPush(id)`: if not inFlight, set inFlight and start a worker for `id`; else set `pending = true`.
2. Worker loop:
   - Clear `pending`.
   - Optionally sleep a short debounce (≤50–100ms); if debounce is used, clear `pending` again after the sleep so requests during the wait coalesce.
   - Call `PushConfigToAgent` (uses `context.Background()` or a resolver-owned context — not the HTTP request context, which may cancel when the handler returns).
   - If `pending` is set, continue the loop; otherwise clear inFlight and exit.

This guarantees:

- At most **one** concurrent build/send per agent (no cross-snapshot version race).
- A burst of N writes collapses to **1 or 2** pushes of the latest DB state (second push only if a write landed during the first).

Optional short debounce (e.g. ≤50–100ms) before the first build is allowed but not required for correctness if pending coalescing exists. Prefer minimal debounce so online edits feel immediate.

### Offline behavior

- `PushConfigToAgent` continues to no-op send when `!IsOnline` (no error to callers).
- `RequestPush` while offline may still run the worker; the worker then no-ops. That is acceptable. Alternatively, `RequestPush` may skip starting a worker when offline — either is fine as long as Connect always requests a push.

No durable “dirty” flag in SQLite is required for v1: the DB already holds the latest entities; reconnect always pushes current resolution.

### Push on connect

In `grpcserver.Connect`, after the agent is registered in `agentmgr` and auth/status checks pass, if status is **approved**, call `RequestPush(agentID)`.

- Pending / rejected agents do not receive config (unchanged).
- Push is async via the queue so Connect setup is not blocked on DB resolution.
- Send path already has a buffered `sendCh`; config message is delivered once the send goroutine is running. Implementation must ensure `RequestPush` cannot race ahead of `Register` (call RequestPush **after** `mgr.Register`).

### Call-site migration

All production call sites that currently `go resolver.PushConfigToAgent` / `PushConfigToAllAgents` / helper wrappers switch to `RequestPush` (or a thin helper that resolves agent IDs then `RequestPush`).

Affected areas (non-exhaustive; grep at implementation time):

- `server/internal/api/plans.go`
- `server/internal/api/hooks.go`
- `server/internal/api/repositories.go`
- `server/internal/api/scripts.go`
- `server/internal/api/agents.go`
- `server/internal/api/settings.go`
- `server/internal/grpcserver/connect.go` (new reconnect push)

### Lifecycle

The queue is process-local. On server shutdown, in-flight pushes may be abandoned (same as today’s detached goroutines). No new persistence requirement.

If the resolver needs a shutdown hook later, that is optional hardening, not part of this design.

## Correctness properties

1. **Single-flight per agent:** two overlapping `RequestPush` calls cannot run two `PushConfigToAgent` builds concurrently for the same agent.
2. **Freshness after coalescing:** if any `RequestPush` occurs during an in-flight push, a subsequent push runs after it and reads the DB again.
3. **Reconnect freshness:** an approved agent that connects receives a push of the current resolved config without needing a further API write.
4. **Protocol unchanged:** `AgentConfig.config_version` still increments once per successful push that passes the online/approved checks and reaches the bump step (offline no-ops do not bump); agent `ConfigAck` unchanged.
5. **Connect ordering:** `RequestPush` runs only after `mgr.Register`, so `IsOnline` is true when the worker runs for a reconnecting agent.

## Testing

- Unit tests for coalescing: N rapid `RequestPush` for one agent → at most two `PushConfigToAgent` executions (or one if all requests precede the worker start), never concurrent for the same ID (use a fake pusher / hook).
- Concurrent `RequestPush` for different agents may proceed in parallel.
- Offline: `RequestPush` + `!IsOnline` does not panic; no send.
- Connect: approved agent register path invokes `RequestPush` (mock resolver or spy).
- Existing resolver tests that call `PushConfigToAgent` directly remain valid for build/send content.

## Alternatives considered

| Option | Why not now |
|---|---|
| Heartbeat revision sync only (#47 proposal) | Up to ~30s latency; user wants immediate online effect. |
| Optimistic locking / CAS alone | Does not by itself serialize builds/sends; usually needs a queue anyway. |
| Hybrid revision + immediate push (D) | Valuable for observability later; extra proto/agent surface not needed yet. |

## Follow-ups (explicitly later)

- Revision in heartbeat + desired/applied tracking for fleet observability.
- Agent rejecting non-monotonic `config_version` as defense in depth.
- Durable dirty bit if Connect push must be distinguishable from “already applied” without always re-pushing (optimization only; always push on connect is simpler and correct).
