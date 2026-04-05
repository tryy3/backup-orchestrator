# Frontend Redesign Plan

> Tracking document for the Command Center redesign. Update this file as work progresses.

## Goal

Redesign the Vue.js frontend from a flat CRUD admin panel into a dark-mode, NOC-style **Command Center**
with a monitoring-first drill-down hierarchy: **Fleet Overview → Agent Inspector → Plan History → Job Console**.
Configuration screens (Plans, Repositories, Scripts, Settings) move to a separate sidebar section.

## Design Decisions

- **Dark mode only** — deep blacks (`surface-950` = #07070f), neon cyan accent (`#0ddbf2`), Space Grotesk font
- **No chart library** for Phase 1 — CSS-based sparkline histogram bars (avoid extra dependency until needed)
- **No backend changes** in Phase 1 — client-side aggregation from existing `jobsStore.list` for sparklines
- **URL structure** — monitoring via `/agents/:id/plans/:planId` (new); config keeps existing `/plans`, `/repositories`, `/scripts`
- **Sidebar sections**: MONITOR (Fleet Overview) / CONFIGURE (Plans, Repositories, Scripts) / SYSTEM (Snapshots, Settings)
- **Jobs & Agents removed from sidebar** — Jobs accessed via Plan History drill-down; Agents are the Fleet Overview itself
- **Mock data** — where backend stats don't exist, compute client-side or display placeholders

## Navigation Hierarchy

```
/ (Fleet Overview)
  └── /agents/:id (Agent Inspector)
        └── /agents/:id/plans/:planId (Plan History) ← NEW
              └── /jobs/:id (Job Console)

Sidebar CONFIGURE:
  /plans, /plans/new, /plans/:id/edit
  /repositories, /repositories/new, /repositories/:id/edit
  /scripts, /scripts/new, /scripts/:id/edit
Sidebar SYSTEM:
  /snapshots
  /settings
```

---

## Phase 1: Foundation ✅ Complete

### Theme & Layout
- [x] Dark theme tokens in `style.css` via Tailwind v4 `@theme` (surface-950 → surface-600, accent cyan)
- [x] Space Grotesk font from Google Fonts
- [x] `AppLayout.vue` — dark background
- [x] `Sidebar.vue` — grouped sections (MONITOR / CONFIGURE / SYSTEM), dark nav, cyan active indicator
- [x] `Header.vue` — dark breadcrumb bar with dynamic back-navigation (reads from agent/plan stores)

### Router
- [x] Add `/agents/:id/plans/:planId` route → `PlanHistoryView.vue` (name: `plan-history`)
- [x] Rename route names: `dashboard` → `fleet-overview`, `agent-detail` → `agent-inspect`

### Common Components (dark restyle)
- [x] `StatusBadge.vue` — neon status colors (green-400/red-400/amber-400/cyan-400) on dark pill
- [x] `DataTable.vue` — dark surface table (surface-900 bg, surface-700 borders/dividers)
- [x] `EmptyState.vue` — dark dashed border, toned-down icon
- [x] `LoadingSpinner.vue` — cyan-400 spinner
- [x] `ConfirmDialog.vue` — dark modal (surface-800 bg)

---

## Phase 2: Monitoring Screens ✅ Complete

- [x] `DashboardView.vue` → **Fleet Overview** — agent cards grid, 30-day CSS sparklines, filter buttons,
      global success rate, online/offline/healthy/warning/failing classification
- [x] `AgentDetailView.vue` → **Agent Inspector** — plan table with drill-down link, metadata grid,
      failing-plan alert banner, collapsible rclone config
- [x] `PlanHistoryView.vue` → **Plan History** (NEW) — KPIs (success rate, avg duration), job executions
      table, trigger backup button, plan config summary
- [x] `JobDetailView.vue` → **Job Console** — dark terminal-style log viewer, repository results,
      hook results, dark card styling throughout

---

## Phase 3: Configuration Screens 🔲 Pending

- [ ] `RepositoriesView.vue` → card grid with connection status indicator
- [ ] `RepositoryFormView.vue` → dark restyle
- [ ] `PlansView.vue` → dark restyle with agent grouping
- [ ] `PlanFormView.vue` → multi-step wizard layout
- [ ] `PlanDetailView.vue` → dark restyle (plan edit + hook management)
- [ ] `ScriptsView.vue` + `ScriptFormView.vue` → merged split-pane Script Library
- [ ] `SettingsView.vue` → dark restyle + agent enrollment section
- [ ] `SnapshotsView.vue` → dark restyle
- [ ] Plan components: `HookEditor.vue`, `RepositoryPicker.vue`, `RetentionEditor.vue` → dark restyle

---

## Phase 4: Polish 🔲 Pending

- [ ] Agent enrollment (pending approval) surfaced on Fleet Overview banner
- [ ] Responsive pass (mobile sidebar collapse, card stacking)
- [ ] Loading skeleton states (dark placeholders)
- [ ] Route transition animations
- [ ] Install Chart.js / vue-chartjs for richer sparklines (replace CSS bars)
- [ ] Backend stats endpoints (`/api/dashboard/agent-stats`, etc.) when ready

---

## Backend Changes Deferred

These will be needed in Phase 4+ when the backend adds aggregation endpoints:

- `GET /api/dashboard/fleet-health` — online agent count + overall %
- `GET /api/dashboard/agent-stats?days=30` — per-agent success/fail counts + daily sparkline data
- `GET /api/dashboard/plan-stats/{planID}?days=30` — success rate, avg duration, total data
- Add composite indexes on `jobs(agent_id, status, started_at)` and `jobs(plan_id, status, started_at)`
- Add pagination to `GET /api/jobs` (`?limit=&offset=`)

Until then: client-side aggregation from `jobsStore.list` (all jobs fetched on dashboard load).

---

## Known Limitations (Phase 1-2)

- **Sparklines** are computed from all jobs returned by `jobsStore.fetchAll()` — no pagination, works for
  small-to-medium job counts; may be slow for very large histories
- **Agent throughput / latency / storage quota** shown in mockups not available in current data model — not displayed
- **Live log streaming** (shown in mockup) out of scope — logs display on load; running jobs have a refresh button
- **"Pause Agent"** action (shown in mockup Agent Inspector) not in current API — not implemented

---

## Files Changed

### Created
- `docs/redesign-plan.md` (this file)
- `frontend/src/views/PlanHistoryView.vue`

### Rewritten
- `frontend/src/style.css`
- `frontend/src/components/layout/AppLayout.vue`
- `frontend/src/components/layout/Sidebar.vue`
- `frontend/src/components/layout/Header.vue`
- `frontend/src/views/DashboardView.vue` (Fleet Overview)
- `frontend/src/views/AgentDetailView.vue` (Agent Inspector)
- `frontend/src/views/JobDetailView.vue` (Job Console)

### Modified
- `frontend/src/router/index.ts`
- `frontend/src/components/common/StatusBadge.vue`
- `frontend/src/components/common/DataTable.vue`
- `frontend/src/components/common/EmptyState.vue`
- `frontend/src/components/common/LoadingSpinner.vue`
- `frontend/src/components/common/ConfirmDialog.vue`
