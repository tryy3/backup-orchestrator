import { defineStore } from 'pinia'
import { ref } from 'vue'
import { repositories as api } from '../api/client'
import type { Repository, RepositoryCreate } from '../types/api'

export const useRepositoriesStore = defineStore('repositories', () => {
  const list = ref<Repository[]>([])
  const current = ref<Repository | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchAll(params?: { scope?: string; agent_id?: string }) {
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

  async function create(data: RepositoryCreate) {
    error.value = null
    try {
      const repo = await api.create(data)
      list.value.push(repo)
      return repo
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return null
    }
  }

  async function update(id: string, data: Partial<RepositoryCreate>) {
    error.value = null
    try {
      const repo = await api.update(id, data)
      const idx = list.value.findIndex((r) => r.id === id)
      if (idx >= 0) list.value[idx] = repo
      if (current.value?.id === id) current.value = repo
      return repo
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return null
    }
  }

  async function remove(id: string) {
    error.value = null
    try {
      await api.remove(id)
      list.value = list.value.filter((r) => r.id !== id)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  return { list, current, loading, error, fetchAll, fetchOne, create, update, remove }
})
