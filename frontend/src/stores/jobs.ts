import { defineStore } from 'pinia'
import { ref } from 'vue'
import { jobs as api } from '../api/client'
import { subscribe } from '../api/websocket'
import type { Job, JobDetail, JobCreatedEvent, JobStartedEvent, JobProgressEvent, JobCompletedEvent } from '../types/api'

// Active job progress tracked from WebSocket events.
// Keyed by agent_id (since only one job runs at a time per agent).
export const jobProgress = ref<Map<string, { planName: string; percent: number }>>(new Map())

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

  // Subscribe to WebSocket events for live job updates.
  subscribe('job.created', (payload) => {
    const event = payload as JobCreatedEvent
    // Add to list if not already present.
    if (!list.value.some((j) => j.id === event.id)) {
      list.value.unshift({
        id: event.id,
        agent_id: event.agent_id,
        plan_id: event.plan_id,
        plan_name: event.plan_name,
        type: event.type as Job['type'],
        trigger: event.trigger as Job['trigger'],
        status: event.status as Job['status'],
        started_at: event.started_at,
        finished_at: null,
        log_tail: null,
        created_at: event.created_at,
      })
    }
  })

  subscribe('job.started', (payload) => {
    const event = payload as JobStartedEvent
    // Update matching job in list.
    const job = list.value.find((j) => j.id === event.job_id)
    if (job) {
      job.status = 'running'
      if (event.started_at) job.started_at = event.started_at
    }
    // Update current if viewing this job.
    if (current.value?.id === event.job_id) {
      current.value.status = 'running'
      if (event.started_at) current.value.started_at = event.started_at
    }
    // Track progress.
    jobProgress.value.set(event.agent_id, {
      planName: event.plan_name,
      percent: event.progress_percent,
    })
  })

  subscribe('job.progress', (payload) => {
    const event = payload as JobProgressEvent
    jobProgress.value.set(event.agent_id, {
      planName: event.plan_name,
      percent: event.progress_percent,
    })
  })

  subscribe('job.completed', (payload) => {
    const event = payload as JobCompletedEvent
    // Update or add the job in the list.
    const idx = list.value.findIndex((j) => j.id === event.id)
    const updatedJob: Job = {
      id: event.id,
      agent_id: event.agent_id,
      plan_id: event.plan_id,
      plan_name: event.plan_name,
      type: event.type as Job['type'],
      trigger: event.trigger as Job['trigger'],
      status: event.status as Job['status'],
      started_at: event.started_at,
      finished_at: event.finished_at,
      log_tail: null,
      created_at: event.created_at,
    }
    if (idx >= 0) {
      list.value[idx] = updatedJob
    } else {
      list.value.unshift(updatedJob)
    }
    // If currently viewing this job, refetch to get full detail (results, logs).
    if (current.value?.id === event.id) {
      fetchOne(event.id)
    }
    // Clear progress tracking for this agent.
    jobProgress.value.delete(event.agent_id)
  })

  return { list, current, loading, error, fetchAll, fetchOne }
})
