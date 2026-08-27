import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import SettingsView from '../SettingsView.vue'
import RetentionEditor from '../../components/plans/RetentionEditor.vue'
import { SETTINGS_DEFAULTS } from '../../types/api'

vi.mock('../../api/client', async () => {
  const actual = await vi.importActual<typeof import('../../api/client')>('../../api/client')
  return {
    ...actual,
    settings: { get: vi.fn(), update: vi.fn() },
    version: { get: vi.fn().mockResolvedValue({ version: '1.0.0', commit: 'abc', build_date: '2026-01-01' }) },
  }
})

describe('SettingsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders seeded fieldErrors in the global list and inline under controls', async () => {
    const wrapper = mount(SettingsView, {
      global: {
        plugins: [
          createTestingPinia({
            createSpy: vi.fn,
            initialState: {
              settings: {
                settings: { ...SETTINGS_DEFAULTS },
                loading: false,
                error: 'validation failed',
                fieldErrors: {
                  heartbeat_interval_seconds: 'must be at least 5',
                },
              },
            },
          }),
        ],
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('heartbeat_interval_seconds: must be at least 5')
    const inline = wrapper.findAll('p').filter((p) => p.text() === 'must be at least 5')
    expect(inline.length).toBeGreaterThanOrEqual(1)
  })

  it('uses registry-aligned retention defaults before fetch completes', async () => {
    const { settings: settingsApi } = await import('../../api/client')
    vi.mocked(settingsApi.get).mockImplementation(
      () => new Promise(() => {}), // never resolves — stay on pre-fetch defaults
    )

    const wrapper = mount(SettingsView, {
      global: {
        plugins: [
          createTestingPinia({
            createSpy: vi.fn,
            stubActions: false,
            initialState: {
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

    expect(SETTINGS_DEFAULTS.default_retention).toEqual({
      keep_last: 5,
      keep_hourly: 0,
      keep_daily: 0,
      keep_weekly: 0,
      keep_monthly: 0,
      keep_yearly: 0,
    })
    expect(wrapper.findComponent(RetentionEditor).props('modelValue')).toEqual(
      SETTINGS_DEFAULTS.default_retention,
    )
  })
})
