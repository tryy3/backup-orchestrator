# Frontend Dark Theme Consistency — Implementation Tracker

## Goal
Bring all views and components in line with the Cyber Command dark design system established in `DESIGN.md`. The reference implementation is `DashboardView.vue` (Fleet Overview).

## Status: ✅ Complete

---

## Phase 1: Shared Components

| File | Status | Notes |
|------|--------|-------|
| `frontend/src/components/plans/RetentionEditor.vue` | ✅ Done | Labels, inputs, disabled state |
| `frontend/src/components/plans/HookEditor.vue` | ✅ Done | Full overhaul — form bg, all inputs/selects/radios, error, buttons |
| `frontend/src/components/plans/RepositoryPicker.vue` | ✅ Done | Checkbox labels, borders, text colors |

---

## Phase 2: Form Views

| File | Status | Notes |
|------|--------|-------|
| `frontend/src/views/PlanFormView.vue` | ✅ Done | 7 white card sections, all inputs, cron buttons, "+ Add" links, alerts, submit/cancel |
| `frontend/src/views/RepositoryFormView.vue` | ✅ Done | Form container, inputs, radios, submit/cancel |
| `frontend/src/views/ScriptFormView.vue` | ✅ Done | Form container, inputs, select, submit/cancel |

---

## Phase 3: List Views

| File | Status | Notes |
|------|--------|-------|
| `frontend/src/views/PlansView.vue` | ✅ Done | Filter select, name link color, View/Edit/Delete row buttons |
| `frontend/src/views/RepositoriesView.vue` | ✅ Done | Filter tabs, name text, scope badges, path text, Edit/Delete buttons |
| `frontend/src/views/ScriptsView.vue` | ✅ Done | Name text, code snippet, on_error badge, Edit/Delete buttons |
| `frontend/src/views/JobsView.vue` | ✅ Done | 3 filter selects, plan_name cell text |
| `frontend/src/views/AgentsView.vue` | ✅ Done | Name link color, Approve/Reject/Delete buttons |
| `frontend/src/views/SnapshotsView.vue` | ✅ Done | Selector labels+selects, tag pills, path text, Restore button, white restore modal |

---

## Phase 4: Detail Views

| File | Status | Notes |
|------|--------|-------|
| `frontend/src/views/PlanDetailView.vue` | ✅ Done | 4 white card sections, all text/links/buttons/badges, hooks list |
| `frontend/src/views/SettingsView.vue` | ✅ Done | Card container, title, success/error alerts, Save button |

---

## Phase 5: Minor Fixes

| File | Status | Notes |
|------|--------|-------|
| `frontend/src/views/JobDetailView.vue` | ✅ Done | Log panel: `bg-gray-900` → `bg-surface-950`, `border-gray-800` → `border-surface-700` |
| `frontend/src/views/PlanHistoryView.vue` | ✅ Done | `rounded-md` → `rounded` on trigger button |

---

## Already Compliant (No Changes Needed)

| File | Notes |
|------|-------|
| `DashboardView.vue` | Reference implementation ✅ |
| `AgentDetailView.vue` | Already dark-themed ✅ |
| `PlanHistoryView.vue` | Mostly good, minor check only ✅ |
| `JobDetailView.vue` | Mostly good, minor log colors only ✅ |
| `components/common/ConfirmDialog.vue` | ✅ |
| `components/common/DataTable.vue` | ✅ |
| `components/common/StatusBadge.vue` | ✅ |
| `components/common/EmptyState.vue` | ✅ |
| `components/common/LoadingSpinner.vue` | ✅ |
| `components/layout/AppLayout.vue` | ✅ |
| `components/layout/Sidebar.vue` | ✅ |
| `components/layout/Header.vue` | ✅ |

---

## Key Pattern Replacements

| Light (remove) | Dark (use) |
|----------------|-----------|
| `bg-white` | `bg-surface-900` |
| `bg-gray-50 / bg-gray-100` | `bg-surface-800` |
| `border-gray-200 / border-gray-300` | `border-surface-700` |
| `text-gray-900` (headings) | `text-slate-100` |
| `text-gray-700` (labels) | `text-slate-400` |
| `text-gray-500 / text-gray-600` | `text-slate-500` |
| `text-gray-400` | `text-slate-600` |
| `text-blue-600 hover:text-blue-700` (links) | `text-accent hover:text-accent-dim` |
| `bg-blue-600 text-white hover:bg-blue-700` (button) | `bg-accent/10 text-accent ring-1 ring-accent/30 hover:bg-accent/20` |
| `border-gray-300 bg-white text-gray-700` (secondary btn) | `border-surface-600 bg-surface-700 text-slate-300 hover:bg-surface-600` |
| `bg-gray-100 text-gray-700 hover:bg-gray-200` (small btn) | `bg-surface-800 text-slate-300 hover:bg-surface-700` |
| `bg-red-100 text-red-700 hover:bg-red-200` (delete btn) | `bg-red-500/10 text-red-400 ring-1 ring-red-500/20 hover:bg-red-500/20` |
| `bg-green-600 text-white hover:bg-green-700` (approve btn) | `bg-green-500/10 text-green-400 ring-1 ring-green-500/20 hover:bg-green-500/20` |
| `bg-red-600 text-white hover:bg-red-700` (reject btn) | `bg-red-500/10 text-red-400 ring-1 ring-red-500/20 hover:bg-red-500/20` |
| `rounded-lg bg-white ... shadow` (card) | `rounded border border-surface-700 bg-surface-900 ...` |
| `border-gray-300 focus:border-blue-500 focus:ring-blue-500` (input) | `border-surface-600 bg-surface-950 text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30` |
| `text-blue-600` (checkbox/radio) | `text-accent` |
| `bg-gray-100 text-gray-700` (badge) | `bg-surface-800 text-slate-400` |
| `bg-blue-100 text-blue-800` (scope badge) | `bg-cyan-500/10 text-cyan-400 ring-1 ring-cyan-500/20` |
| `bg-red-50 text-red-700` (error alert) | `bg-red-500/10 text-red-400 border border-red-500/20` (add `rounded`) |
| `bg-green-50 text-green-700` (success alert) | `bg-green-500/10 text-green-400 border border-green-500/20` (add `rounded`) |
| `bg-gray-100 text-gray-700` (code snippet) | `bg-surface-800 text-slate-300` |
| `disabled:bg-gray-100 disabled:text-gray-500` | `disabled:bg-surface-800 disabled:text-slate-600` |
| `border-gray-200` (inner divider) | `border-surface-700` |

---

## Verification Checklist

- [ ] `cd frontend && npm run build` — no compile errors
- [ ] No remaining `bg-white` in `frontend/src/views/` or `frontend/src/components/plans/`
- [ ] No remaining `border-gray-300` / `border-gray-200` in same scope
- [ ] No remaining `text-gray-900` / `text-gray-700` in same scope
- [ ] No remaining `bg-blue-600` / `text-blue-600` in same scope
- [ ] No remaining `bg-gray-100` / `bg-gray-50` in same scope
- [ ] All modals use dark backgrounds
- [ ] All dropdowns/selects readable on dark background
- [ ] All filter tabs follow segmented control pattern
- [ ] Disabled inputs visually distinguishable without being white
