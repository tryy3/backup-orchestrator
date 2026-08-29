# Agent approval from Fleet Overview

**Date:** 2026-08-29  
**Status:** Approved for implementation planning  
**Issue:** [#6](https://github.com/tryy3/backup-orchestrator/issues/6)  
**Scope:** Frontend Fleet Overview pending-agent approve/reject UX

## Summary

Allow approving or rejecting a pending agent directly from a Fleet Overview card, using the same confirmation dialog pattern as the Agents list. Keep the Agents list as the multi-agent review surface. Do not add approval actions on the agent detail page. The sidebar Agents link is already present and needs no work for this issue.

## Goals

- Approve / Reject pending agents from Fleet Overview cards without navigating away.
- Reuse existing confirm dialogs and `agentsStore.approve` / `reject` APIs.
- Preserve card → agent detail navigation for inspection (background click).
- Keep Agents list (`/agents`) approval actions unchanged.
- Keep the pending banner (count + Review → `/agents`) as a secondary path.

## Non-goals

- Approve / Reject on agent detail (`AgentDetailView`).
- Server or API changes (approve/reject endpoints already exist).
- Adding Agents to the sidebar (already shipped).
- Delete on Fleet Overview cards (Delete remains on the Agents list only).
- Toast/undo-style immediate approve without confirmation.

## Current state

- Fleet Overview (`DashboardView`) shows a pending banner with Review → `/agents`, and pending cards with a Pending badge.
- Pending cards are full `router-link`s to `/agents/:id`.
- Approve / Reject exist only on `AgentsView` (with `ConfirmDialog`).
- `AgentDetailView` has no approve/reject actions.
- Sidebar already includes Agents under MONITOR.

## Interaction design

### Fleet Overview — pending cards

1. Pending cards show an action footer with **Approve** and **Reject** buttons.
2. Button styles match the Agents list (soft green / red).
3. Buttons use `@click.stop` (and prevent default as needed) so they do not follow the card link.
4. Clicking Approve or Reject opens `ConfirmDialog` with the same titles, messages, and variants as `AgentsView`.
5. On confirm, call `agentsStore.approve(id)` or `agentsStore.reject(id)`.
6. On success, list/banner update from store state (pending badge and footer disappear for that agent as status changes).
7. On cancel, no API call; card unchanged.
8. Clicking the rest of the card still navigates to agent detail.

### Accidental navigation note

Background click still navigates. This is accepted for v1; if it becomes noisy in practice, tighten the click target later (e.g. name-only link).

### Pending banner

Unchanged: amber banner with pending count and Review → `/agents`.

### Agents list

No behavior change. Remains the table view for reviewing multiple pending agents with hostname, heartbeat, version, and actions.

### Agent detail

No approval UI. Users return to Fleet Overview or Agents list to approve/reject.

## Layout

On pending cards only, below reliability (and any running-job indicator):

- Compact action footer, visually separated (top border or clear spacing).
- Approve | Reject only — no Delete.

Approved / rejected cards unchanged (no action footer).

## Components & data flow

| Piece | Role |
|---|---|
| `DashboardView.vue` | Add footer buttons, confirm state, dialog, handlers |
| `ConfirmDialog` | Reuse existing component |
| `agentsStore.approve` / `reject` | Existing; refetch/update list as today |
| `ErrorBanner` | Surface store errors on failure |

Optional later: extract shared confirm copy between Dashboard and Agents views — not required for this change.

## Error handling

- Failed approve/reject: leave dialog closed after attempt (same as Agents list) and show `agentsStore.error` via existing ErrorBanner on Fleet Overview.
- Do not navigate away on failure.

## Testing

### Manual

- Pending agent shows Approve / Reject on Fleet card.
- Approve → confirm → approved; badge/actions gone; banner count updates.
- Reject → confirm → no longer pending; banner updates.
- Cancel confirm → no change.
- Button clicks do not navigate to detail.
- Card body click still opens detail.
- Agents list Approve / Reject still works.
- Failed API shows ErrorBanner.

### Automated

Optional Vitest for DashboardView pending actions / click-stop, following `SettingsView.test.ts` patterns. Not a hard requirement.

## Implementation notes

- Primary file: `frontend/src/views/DashboardView.vue`.
- No proto, DB, or server changes.
- Issue #6 can be closed after this ships (sidebar half already done; this completes the primary ask).
