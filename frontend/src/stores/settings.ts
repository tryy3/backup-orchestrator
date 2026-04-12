import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { settings as api } from '../api/client'
import type { Settings } from '../types/api'
import { SETTINGS_DEFAULTS } from '../types/api'

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<Settings | null>(null)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)

  /** Resolved settings with defaults applied for any missing fields. */
  const resolved = computed(() => ({
    heartbeat_interval_seconds:
      settings.value?.heartbeat_interval_seconds ?? SETTINGS_DEFAULTS.heartbeat_interval_seconds,
    agent_offline_threshold_seconds:
      settings.value?.agent_offline_threshold_seconds ?? SETTINGS_DEFAULTS.agent_offline_threshold_seconds,
    job_history_days:
      settings.value?.job_history_days ?? SETTINGS_DEFAULTS.job_history_days,
    health_threshold_failing:
      settings.value?.health_threshold_failing ?? SETTINGS_DEFAULTS.health_threshold_failing,
    health_threshold_warning:
      settings.value?.health_threshold_warning ?? SETTINGS_DEFAULTS.health_threshold_warning,
    max_heatmap_runs:
      settings.value?.max_heatmap_runs ?? SETTINGS_DEFAULTS.max_heatmap_runs,
    default_hook_timeout_seconds:
      settings.value?.default_hook_timeout_seconds ?? SETTINGS_DEFAULTS.default_hook_timeout_seconds,
    file_browser_blocked_paths:
      settings.value?.file_browser_blocked_paths ?? [...SETTINGS_DEFAULTS.file_browser_blocked_paths],
  }))

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
    saving.value = true
    error.value = null
    try {
      settings.value = await api.update(data)
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    } finally {
      saving.value = false
    }
  }

  return { settings, resolved, loading, saving, error, fetch, update }
})
