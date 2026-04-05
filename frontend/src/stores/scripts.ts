import { defineStore } from 'pinia'
import { ref } from 'vue'
import { scripts as api } from '../api/client'
import type { Script, ScriptCreate } from '../types/api'

export const useScriptsStore = defineStore('scripts', () => {
  const list = ref<Script[]>([])
  const current = ref<Script | null>(null)
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

  async function create(data: ScriptCreate) {
    error.value = null
    try {
      const script = await api.create(data)
      list.value.push(script)
      return script
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return null
    }
  }

  async function update(id: string, data: Partial<ScriptCreate>) {
    error.value = null
    try {
      const script = await api.update(id, data)
      const idx = list.value.findIndex((s) => s.id === id)
      if (idx >= 0) list.value[idx] = script
      if (current.value?.id === id) current.value = script
      return script
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return null
    }
  }

  async function remove(id: string) {
    error.value = null
    try {
      await api.remove(id)
      list.value = list.value.filter((s) => s.id !== id)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  return { list, current, loading, error, fetchAll, fetchOne, create, update, remove }
})
