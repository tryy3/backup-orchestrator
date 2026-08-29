# Fleet Overview Agent Approval Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let operators Approve / Reject pending agents directly from Fleet Overview cards (with the same confirm dialogs as the Agents list), completing [#6](https://github.com/tryy3/backup-orchestrator/issues/6).

**Architecture:** Frontend-only change in `DashboardView.vue`. Reuse `ConfirmDialog` and existing `agentsStore.approve` / `reject`. Pending cards keep navigating to agent detail on background click; action buttons use `@click.stop`. Agents list and agent detail are unchanged. No server/API work.

**Tech Stack:** Vue 3 + TypeScript + Pinia + Vue Router + Vitest + Vue Test Utils + `@pinia/testing`

**Spec:** [docs/superpowers/specs/2026-08-29-agent-approval-fleet-ux-design.md](../specs/2026-08-29-agent-approval-fleet-ux-design.md)

## Global Constraints

- No Approve / Reject on `AgentDetailView`.
- No server, proto, or DB changes.
- No Delete on Fleet Overview cards.
- Confirm before approve/reject (same copy as Agents list).
- Keep pending banner + Review → `/agents`.
- Prefer `just test-frontend` for verification.
- Do not commit secrets.

## File map

| File | Responsibility |
|---|---|
| `frontend/src/views/DashboardView.vue` | Pending-card Approve/Reject footer + ConfirmDialog + handlers |
| `frontend/src/views/__tests__/DashboardView.test.ts` | Vitest coverage for pending actions / confirm / store calls |
| `frontend/src/views/AgentsView.vue` | Reference only — do not change |
| `frontend/src/stores/agents.ts` | Reference only — reuse `approve` / `reject` |

---

### Task 1: Failing Vitest coverage for Fleet pending actions

**Files:**
- Create: `frontend/src/views/__tests__/DashboardView.test.ts`

**Interfaces:**
- Consumes: `DashboardView` (current, without approve UI yet), Pinia stores `agents` / `jobs` / `settings`, `ConfirmDialog`
- Produces: Failing tests that assert Approve/Reject buttons, confirm dialog copy, and store `approve` / `reject` calls

- [ ] **Step 1: Write the failing test file**

Create `frontend/src/views/__tests__/DashboardView.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import { createRouter, createMemoryHistory } from 'vue-router'
import DashboardView from '../DashboardView.vue'
import ConfirmDialog from '../../components/common/ConfirmDialog.vue'
import { useAgentsStore } from '../../stores/agents'
import type { Agent } from '../../types/api'

const pendingAgent: Agent = {
  id: 'agent-pending',
  name: 'pending-host',
  hostname: 'pending.local',
  os: 'linux',
  status: 'pending',
  agent_version: '1.0.0',
  restic_version: '0.16.0',
  rclone_version: '',
  has_rclone_config: false,
  last_heartbeat: null,
  last_job_at: null,
  config_version: 0,
  config_applied_at: null,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

const approvedAgent: Agent = {
  ...pendingAgent,
  id: 'agent-approved',
  name: 'approved-host',
  hostname: 'approved.local',
  status: 'approved',
  last_heartbeat: new Date().toISOString(),
}

async function mountDashboard(agents: Agent[]) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'fleet-overview', component: DashboardView },
      { path: '/agents/:id', name: 'agent-detail', component: { template: '<div />' } },
      { path: '/agents', name: 'agents', component: { template: '<div />' } },
    ],
  })
  await router.push('/')
  await router.isReady()

  const wrapper = mount(DashboardView, {
    attachTo: document.body,
    global: {
      plugins: [
        router,
        createTestingPinia({
          createSpy: vi.fn,
          stubActions: true,
          initialState: {
            agents: {
              list: agents,
              current: null,
              loading: false,
              saving: false,
              error: null,
            },
            jobs: {
              list: [],
              current: null,
              jobProgress: new Map(),
              loading: false,
              error: null,
            },
            settings: {
              settings: null,
              loading: false,
              error: null,
              fieldErrors: {},
            },
          },
        }),
      ],
    },
  })
  await flushPromises()
  return wrapper
}

describe('DashboardView pending approval', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    document.body.innerHTML = ''
  })

  it('shows Approve and Reject on pending cards only', async () => {
    const wrapper = await mountDashboard([pendingAgent, approvedAgent])

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons).toContain('Approve')
    expect(buttons).toContain('Reject')

    // Approved card should not render those actions — only one of each on the page
    expect(buttons.filter((t) => t === 'Approve')).toHaveLength(1)
    expect(buttons.filter((t) => t === 'Reject')).toHaveLength(1)

    wrapper.unmount()
  })

  it('opens Approve confirm dialog and calls store.approve on confirm', async () => {
    const wrapper = await mountDashboard([pendingAgent])
    const store = useAgentsStore()

    const approveBtn = wrapper.findAll('button').find((b) => b.text() === 'Approve')
    expect(approveBtn).toBeTruthy()
    await approveBtn!.trigger('click')
    await flushPromises()

    const dialog = wrapper.findComponent(ConfirmDialog)
    expect(dialog.props('open')).toBe(true)
    expect(dialog.props('title')).toBe('Approve Agent')
    expect(dialog.props('message')).toBe(
      'Approve this agent to allow it to receive backup configurations?',
    )
    expect(dialog.props('confirmText')).toBe('Approve')
    expect(dialog.props('confirmVariant')).toBe('primary')

    await dialog.vm.$emit('confirm')
    await flushPromises()

    expect(store.approve).toHaveBeenCalledWith('agent-pending')
    wrapper.unmount()
  })

  it('opens Reject confirm dialog and calls store.reject on confirm', async () => {
    const wrapper = await mountDashboard([pendingAgent])
    const store = useAgentsStore()

    const rejectBtn = wrapper.findAll('button').find((b) => b.text() === 'Reject')
    expect(rejectBtn).toBeTruthy()
    await rejectBtn!.trigger('click')
    await flushPromises()

    const dialog = wrapper.findComponent(ConfirmDialog)
    expect(dialog.props('open')).toBe(true)
    expect(dialog.props('title')).toBe('Reject Agent')
    expect(dialog.props('message')).toBe(
      'Reject this agent? It will not be able to receive backup configurations.',
    )
    expect(dialog.props('confirmText')).toBe('Reject')
    expect(dialog.props('confirmVariant')).toBe('danger')

    await dialog.vm.$emit('confirm')
    await flushPromises()

    expect(store.reject).toHaveBeenCalledWith('agent-pending')
    wrapper.unmount()
  })

  it('cancel closes dialog without calling approve or reject', async () => {
    const wrapper = await mountDashboard([pendingAgent])
    const store = useAgentsStore()

    const approveBtn = wrapper.findAll('button').find((b) => b.text() === 'Approve')
    await approveBtn!.trigger('click')
    await flushPromises()

    const dialog = wrapper.findComponent(ConfirmDialog)
    await dialog.vm.$emit('cancel')
    await flushPromises()

    expect(dialog.props('open')).toBe(false)
    expect(store.approve).not.toHaveBeenCalled()
    expect(store.reject).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd frontend && npx vitest run src/views/__tests__/DashboardView.test.ts
```

Expected: FAIL — pending cards do not yet render Approve/Reject buttons (or ConfirmDialog is missing), so assertions like `toContain('Approve')` fail.

- [ ] **Step 3: Commit the failing tests**

```bash
git add frontend/src/views/__tests__/DashboardView.test.ts
git commit -m "$(cat <<'EOF'
test(frontend): add failing coverage for fleet pending approval

EOF
)"
```

---

### Task 2: Implement Fleet Overview approve/reject UI

**Files:**
- Modify: `frontend/src/views/DashboardView.vue`

**Interfaces:**
- Consumes: `ConfirmDialog`, `useAgentsStore().approve(id: string)` / `reject(id: string)`
- Produces: Pending-card action footer; confirm dialog wired to store; `@click.stop` on action buttons

- [ ] **Step 1: Add script-side confirm state and handlers**

In `frontend/src/views/DashboardView.vue` `<script setup>`:

1. Import `ConfirmDialog` and keep existing imports.
2. Add confirm state and helpers matching Agents list copy (approve/reject only — no delete):

```ts
import ConfirmDialog from '../components/common/ConfirmDialog.vue'

const confirmOpen = ref(false)
const confirmAgentId = ref('')
const confirmAction = ref<'approve' | 'reject'>('approve')
const confirmTitle = ref('')
const confirmMessage = ref('')

function openConfirmDialog(id: string, action: 'approve' | 'reject') {
  confirmAgentId.value = id
  confirmAction.value = action
  if (action === 'approve') {
    confirmTitle.value = 'Approve Agent'
    confirmMessage.value = 'Approve this agent to allow it to receive backup configurations?'
  } else {
    confirmTitle.value = 'Reject Agent'
    confirmMessage.value = 'Reject this agent? It will not be able to receive backup configurations.'
  }
  confirmOpen.value = true
}

async function handleConfirm() {
  confirmOpen.value = false
  if (confirmAction.value === 'approve') {
    await agentsStore.approve(confirmAgentId.value)
  } else {
    await agentsStore.reject(confirmAgentId.value)
  }
}
```

Place these after the existing `ref` / store setup (near other reactive state). Do not remove existing helpers.

- [ ] **Step 2: Add pending action footer on each card**

Inside the agent card `router-link` template, **after** the running-job indicator block (still inside the `router-link`), add:

```vue
        <!-- Pending approval actions -->
        <div
          v-if="agent.status === 'pending'"
          class="mt-3 flex items-center gap-2 border-t border-surface-700 pt-3"
          @click.stop
        >
          <button
            type="button"
            class="rounded bg-green-500/10 px-2.5 py-1 text-xs font-medium text-green-400 ring-1 ring-green-500/20 hover:bg-green-500/20"
            @click.stop.prevent="openConfirmDialog(agent.id, 'approve')"
          >
            Approve
          </button>
          <button
            type="button"
            class="rounded bg-red-500/10 px-2.5 py-1 text-xs font-medium text-red-400 ring-1 ring-red-500/20 hover:bg-red-500/20"
            @click.stop.prevent="openConfirmDialog(agent.id, 'reject')"
          >
            Reject
          </button>
        </div>
```

Keep the existing Pending badge in the card header. Do not add Delete. Do not change the pending banner.

- [ ] **Step 3: Mount ConfirmDialog at the end of the root template**

Before the closing `</div>` of the page root, add:

```vue
    <ConfirmDialog
      :open="confirmOpen"
      :title="confirmTitle"
      :message="confirmMessage"
      :confirm-text="confirmAction === 'approve' ? 'Approve' : 'Reject'"
      :confirm-variant="confirmAction === 'approve' ? 'primary' : 'danger'"
      @confirm="handleConfirm"
      @cancel="confirmOpen = false"
    />
```

- [ ] **Step 4: Run unit tests**

Run:

```bash
cd frontend && npx vitest run src/views/__tests__/DashboardView.test.ts
```

Expected: PASS (all four tests).

If `jobs` store initial state shape differs (e.g. extra fields), adjust the test `initialState.jobs` to match `useJobsStore` — do not weaken assertions.

- [ ] **Step 5: Run full frontend suite**

Run:

```bash
just test-frontend
```

Expected: all frontend tests pass.

- [ ] **Step 6: Commit implementation**

```bash
git add frontend/src/views/DashboardView.vue frontend/src/views/__tests__/DashboardView.test.ts
git commit -m "$(cat <<'EOF'
feat(frontend): approve pending agents from fleet overview cards

EOF
)"
```

---

### Task 3: Manual smoke check against a live pending agent

**Files:**
- None (verification only)

**Interfaces:**
- Consumes: Running `just dev-server` + `just dev-frontend` (or `just dev`) with a pending agent enrollment
- Produces: Confirmed manual checklist from the spec

- [ ] **Step 1: Start the stack and enroll a pending agent (if not already available)**

Use the project’s normal local flow (`.env` already configured). Ensure at least one agent is in `pending` status so Fleet Overview shows the amber banner and a Pending card.

- [ ] **Step 2: Walk the manual checklist**

Verify each item:

1. Pending card shows Approve and Reject in a footer separated from reliability stats.
2. Approve → dialog “Approve Agent” → confirm → agent becomes approved; badge/footer gone; banner count updates (or banner disappears if last pending).
3. Reject path (use another pending agent or re-enroll): dialog “Reject Agent” → confirm → no longer pending.
4. Cancel leaves the agent pending with no API side effect.
5. Clicking Approve/Reject does **not** navigate to `/agents/:id`.
6. Clicking card body (outside buttons) still opens agent detail.
7. Agents list (`/agents`) still has Approve / Reject.
8. Agent detail has no Approve / Reject (by design).

- [ ] **Step 3: Final commit only if Task 2 left uncommitted fixes**

If smoke testing required small UI tweaks, commit them:

```bash
git add frontend/src/views/DashboardView.vue
git commit -m "$(cat <<'EOF'
fix(frontend): polish fleet pending approval actions

EOF
)"
```

Otherwise skip this commit.

---

## Spec coverage checklist

| Spec requirement | Task |
|---|---|
| Inline Approve/Reject on Fleet pending cards | Task 2 |
| Confirm dialog same copy/variants as Agents list | Task 2 |
| `@click.stop` so buttons don’t navigate | Task 2 |
| Card body still navigates to detail | Task 2 (unchanged link) + Task 3 |
| Banner unchanged | Task 2 (do not touch) |
| Agents list unchanged | No task modifies it |
| No detail-page approval | No task modifies `AgentDetailView` |
| No server changes | File map is frontend-only |
| ErrorBanner on failure | Existing banner already binds `agentsStore.error`; Task 2 uses store methods that set it |
| Automated tests | Task 1–2 |
| Manual verification | Task 3 |
