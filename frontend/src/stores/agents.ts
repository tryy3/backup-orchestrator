import { defineStore } from 'pinia'
import { ref, onScopeDispose } from 'vue'
import { agents as api } from '../api/client'
import { subscribe } from '../api/websocket'
import type { Agent, CommandTimeouts } from '../types/api'
import { SETTINGS_DEFAULTS } from '../types/api'

/**
 * Heartbeat age (ms) beyond which an agent is considered offline.
 * Used as a fallback for WebSocket disconnect events; the actual threshold
 * used for UI display comes from the settings store.
 */
const OFFLINE_THRESHOLD_MS = SETTINGS_DEFAULTS.agent_offline_threshold_seconds * 1000

export const useAgentsStore = defineStore('agents', () => {
  const list = ref<Agent[]>([])
  const current = ref<Agent | null>(null)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)

  async function fetchAll() {
    loading.value = true
    error.value = null
    try {
      list.value = await api.list()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  async function fetchOne(id: string) {
    loading.value = true
    error.value = null
    try {
      current.value = await api.get(id)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  async function approve(id: string) {
    saving.value = true
    error.value = null
    try {
      await api.approve(id)
      await fetchAll()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      saving.value = false
    }
  }

  async function reject(id: string) {
    saving.value = true
    error.value = null
    try {
      await api.reject(id)
      await fetchAll()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      saving.value = false
    }
  }

  async function remove(id: string) {
    saving.value = true
    error.value = null
    try {
      await api.remove(id)
      list.value = list.value.filter((a) => a.id !== id)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      saving.value = false
    }
  }

  async function updateRclone(id: string, config: string) {
    saving.value = true
    error.value = null
    try {
      await api.updateRclone(id, config)
      if (current.value?.id === id) {
        current.value = { ...current.value, has_rclone_config: config !== '' }
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      saving.value = false
    }
  }

  async function fetchRcloneConfig(id: string): Promise<string> {
    try {
      const result = await api.getRclone(id)
      return result.rclone_config
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return ''
    }
  }

  async function updateCommandTimeouts(id: string, timeouts: CommandTimeouts | null) {
    saving.value = true
    error.value = null
    try {
      const updated = await api.updateCommandTimeouts(id, timeouts)
      if (current.value?.id === id) {
        current.value = updated
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      saving.value = false
    }
  }

  // Subscribe to WebSocket events for live agent updates.
  const unsubs: (() => void)[] = []

  unsubs.push(subscribe('agent.connected', (event) => {
    const agent = list.value.find((a) => a.id === event.agent_id)
    if (agent) {
      agent.last_heartbeat = new Date().toISOString()
    }
    if (current.value?.id === event.agent_id) {
      current.value.last_heartbeat = new Date().toISOString()
    }
  }))

  unsubs.push(subscribe('agent.disconnected', (event) => {
    const stale = new Date(Date.now() - OFFLINE_THRESHOLD_MS).toISOString()
    const agent = list.value.find((a) => a.id === event.agent_id)
    if (agent) {
      agent.last_heartbeat = stale
    }
    if (current.value?.id === event.agent_id) {
      current.value.last_heartbeat = stale
    }
  }))

  unsubs.push(subscribe('agent.heartbeat', (event) => {
    const agent = list.value.find((a) => a.id === event.agent_id)
    if (agent) {
      agent.last_heartbeat = event.timestamp
    }
    if (current.value?.id === event.agent_id) {
      current.value.last_heartbeat = event.timestamp
    }
  }))

  unsubs.push(subscribe('agent.registered', (agent) => {
    if (!list.value.some((a) => a.id === agent.id)) {
      list.value.push(agent)
    }
  }))

  onScopeDispose(() => {
    unsubs.forEach((fn) => fn())
  })

  return { list, current, loading, saving, error, fetchAll, fetchOne, approve, reject, remove, updateRclone, fetchRcloneConfig, updateCommandTimeouts }
})
