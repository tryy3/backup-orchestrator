import { defineStore } from 'pinia'
import { ref } from 'vue'
import { agents as api } from '../api/client'
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

  return { list, current, loading, error, fetchAll, fetchOne, approve, reject, remove, updateRclone }
})
