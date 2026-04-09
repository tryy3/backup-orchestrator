import { defineStore } from 'pinia'
import { ref } from 'vue'
import { plans as api } from '../api/client'
import type { BackupPlan, BackupPlanCreate } from '../types/api'

export const usePlansStore = defineStore('plans', () => {
  const list = ref<BackupPlan[]>([])
  const current = ref<BackupPlan | null>(null)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)

  async function fetchAll(params?: { agent_id?: string }) {
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

  async function create(data: BackupPlanCreate) {
    saving.value = true
    error.value = null
    try {
      const plan = await api.create(data)
      list.value.push(plan)
      return plan
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return null
    } finally {
      saving.value = false
    }
  }

  async function update(id: string, data: Partial<BackupPlanCreate>) {
    saving.value = true
    error.value = null
    try {
      const plan = await api.update(id, data)
      const idx = list.value.findIndex((p) => p.id === id)
      if (idx >= 0) list.value[idx] = plan
      if (current.value?.id === id) current.value = plan
      return plan
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return null
    } finally {
      saving.value = false
    }
  }

  async function remove(id: string) {
    saving.value = true
    error.value = null
    try {
      await api.remove(id)
      list.value = list.value.filter((p) => p.id !== id)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      saving.value = false
    }
  }

  async function trigger(id: string): Promise<string | null> {
    saving.value = true
    error.value = null
    try {
      const result = await api.trigger(id)
      return result.job_id ?? null
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return null
    } finally {
      saving.value = false
    }
  }

  return { list, current, loading, saving, error, fetchAll, fetchOne, create, update, remove, trigger }
})
