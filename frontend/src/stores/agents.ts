import { defineStore } from 'pinia'
import { ref } from 'vue'
import { agents as api } from '../api/client'
import { subscribe } from '../api/websocket'
import type { Agent } from '../types/api'

export const useAgentsStore = defineStore('agents', () => {
  const list = ref<Agent[]>([])
  const current = ref<Agent | null>(null)
  const loading = ref(false)
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
    error.value = null
    try {
      await api.approve(id)
      await fetchAll()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  async function reject(id: string) {
    error.value = null
    try {
      await api.reject(id)
      await fetchAll()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  async function remove(id: string) {
    error.value = null
    try {
      await api.remove(id)
      list.value = list.value.filter((a) => a.id !== id)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  async function updateRclone(id: string, config: string) {
    error.value = null
    try {
      await api.updateRclone(id, config)
      if (current.value?.id === id) {
        current.value = { ...current.value, rclone_config: config }
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  // Subscribe to WebSocket events for live agent updates.
  subscribe('agent.connected', (payload) => {
    const event = payload as { agent_id: string; hostname: string }
    // Update the agent's last_heartbeat to now (connected = alive).
    const agent = list.value.find((a) => a.id === event.agent_id)
    if (agent) {
      agent.last_heartbeat = new Date().toISOString()
    }
    if (current.value?.id === event.agent_id) {
      current.value.last_heartbeat = new Date().toISOString()
    }
  })

  subscribe('agent.disconnected', (payload) => {
    const event = payload as { agent_id: string }
    // The heartbeat timestamp stays as-is; the isOnline check will eventually flip to false.
    // We could also set a flag, but for now the heartbeat age check handles it.
    // Force the heartbeat to a stale value so "isOnline" immediately reflects offline.
    const stale = new Date(Date.now() - 10 * 60 * 1000).toISOString() // 10 minutes ago
    const agent = list.value.find((a) => a.id === event.agent_id)
    if (agent) {
      agent.last_heartbeat = stale
    }
    if (current.value?.id === event.agent_id) {
      current.value.last_heartbeat = stale
    }
  })

  subscribe('agent.heartbeat', (payload) => {
    const event = payload as { agent_id: string; timestamp: string }
    const agent = list.value.find((a) => a.id === event.agent_id)
    if (agent) {
      agent.last_heartbeat = event.timestamp
    }
    if (current.value?.id === event.agent_id) {
      current.value.last_heartbeat = event.timestamp
    }
  })

  subscribe('agent.registered', (payload) => {
    const agent = payload as Agent
    if (!list.value.some((a) => a.id === agent.id)) {
      list.value.push(agent)
    }
  })

  return { list, current, loading, error, fetchAll, fetchOne, approve, reject, remove, updateRclone }
})
