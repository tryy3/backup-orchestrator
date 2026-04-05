import { defineStore } from 'pinia'
import { ref } from 'vue'
import { snapshots as api } from '../api/client'
import type { SnapshotInfo, RestoreRequest, BrowseRequest } from '../types/api'

export const useSnapshotsStore = defineStore('snapshots', () => {
  const list = ref<SnapshotInfo[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchList(agentId: string, repoId: string) {
    loading.value = true
    error.value = null
    try {
      list.value = await api.list(agentId, repoId)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  async function browse(agentId: string, data: BrowseRequest) {
    error.value = null
    try {
      return await api.browse(agentId, data)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return null
    }
  }

  async function restore(agentId: string, data: RestoreRequest) {
    error.value = null
    try {
      await api.restore(agentId, data)
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    }
  }

  return { list, loading, error, fetchList, browse, restore }
})
