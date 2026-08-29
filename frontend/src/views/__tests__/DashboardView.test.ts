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
  return { wrapper, router }
}

describe('DashboardView pending approval', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    document.body.innerHTML = ''
  })

  it('shows Approve and Reject on pending cards only', async () => {
    const { wrapper } = await mountDashboard([pendingAgent, approvedAgent])

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons).toContain('Approve')
    expect(buttons).toContain('Reject')

    // Approved card should not render those actions — only one of each on the page
    expect(buttons.filter((t) => t === 'Approve')).toHaveLength(1)
    expect(buttons.filter((t) => t === 'Reject')).toHaveLength(1)

    wrapper.unmount()
  })

  it('does not navigate when Approve or Reject is clicked', async () => {
    const { wrapper, router } = await mountDashboard([pendingAgent])

    const approveBtn = wrapper.findAll('button').find((b) => b.text() === 'Approve')
    await approveBtn!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('fleet-overview')
    expect(router.currentRoute.value.path).toBe('/')

    const rejectBtn = wrapper.findAll('button').find((b) => b.text() === 'Reject')
    await rejectBtn!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('fleet-overview')
    expect(router.currentRoute.value.path).toBe('/')

    wrapper.unmount()
  })

  it('opens Approve confirm dialog and calls store.approve on confirm', async () => {
    const { wrapper } = await mountDashboard([pendingAgent])
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
    const { wrapper } = await mountDashboard([pendingAgent])
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
    const { wrapper } = await mountDashboard([pendingAgent])
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
