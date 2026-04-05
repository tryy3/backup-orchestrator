import { defineStore } from 'pinia'
import { ref } from 'vue'
import { settings as api } from '../api/client'
import type { Settings } from '../types/api'

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<Settings | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetch() {
    loading.value = true
    error.value = null
    try {
      settings.value = await api.get()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  async function update(data: Settings) {
    error.value = null
    try {
      settings.value = await api.update(data)
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    }
  }

  return { settings, loading, error, fetch, update }
})
