import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useSettingsStore } from '../settings'
import * as api from '../../api/client'
import { SettingsValidationError } from '../../api/client'

vi.mock('../../api/client', async () => {
  const actual = await vi.importActual<typeof import('../../api/client')>('../../api/client')
  return { ...actual, settings: { get: vi.fn(), update: vi.fn() } }
})

describe('settings store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(api.settings.update).mockReset()
  })

  it('maps validation errors to fieldErrors', async () => {
    vi.mocked(api.settings.update).mockRejectedValue(
      new SettingsValidationError([
        { key: 'heartbeat_interval_seconds', message: 'must be at least 5' },
      ]),
    )
    const store = useSettingsStore()
    const ok = await store.update({} as never)
    expect(ok).toBe(false)
    expect(store.fieldErrors.heartbeat_interval_seconds).toBe('must be at least 5')
  })
})
