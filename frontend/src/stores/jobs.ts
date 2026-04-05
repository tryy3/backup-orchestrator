import { defineStore } from 'pinia'
import { ref } from 'vue'
import { jobs as api } from '../api/client'
import type { Job, JobDetail } from '../types/api'

export const useJobsStore = defineStore('jobs', () => {
  const list = ref<Job[]>([])
  const current = ref<JobDetail | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchAll(params?: { agent_id?: string; plan_id?: string; status?: string }) {
    loading.value = true
    error.value = null
    try {
      list.value = await api.list(params)
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

  return { list, current, loading, error, fetchAll, fetchOne }
})
