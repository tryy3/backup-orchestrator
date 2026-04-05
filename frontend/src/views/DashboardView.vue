<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAgentsStore } from '../stores/agents'
import { useJobsStore } from '../stores/jobs'
import StatusBadge from '../components/common/StatusBadge.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import { relativeTime, formatDuration, durationBetween } from '../utils/time'

const agentsStore = useAgentsStore()
const jobsStore = useJobsStore()

onMounted(() => {
  agentsStore.fetchAll()
  jobsStore.fetchAll()
})

const totalAgents = computed(() => agentsStore.list.length)
const pendingAgents = computed(() => agentsStore.list.filter((a) => a.status === 'pending').length)
const approvedAgents = computed(() => agentsStore.list.filter((a) => a.status === 'approved').length)
const offlineAgents = computed(() => {
  return agentsStore.list.filter((a) => {
    if (a.status !== 'approved') return false
    if (!a.last_heartbeat) return true
    const diff = Date.now() - new Date(a.last_heartbeat).getTime()
    return diff > 5 * 60 * 1000
  }).length
})

const recentJobs = computed(() => {
  return [...jobsStore.list]
    .sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime())
    .slice(0, 10)
})

const statsCards = computed(() => [
  { label: 'Total Agents', value: totalAgents.value, color: 'bg-blue-500' },
  { label: 'Pending', value: pendingAgents.value, color: 'bg-amber-500' },
  { label: 'Active', value: approvedAgents.value, color: 'bg-green-500' },
  { label: 'Offline', value: offlineAgents.value, color: 'bg-gray-500' },
])
</script>

<template>
  <div class="space-y-6">
    <!-- Stats cards -->
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <div
        v-for="card in statsCards"
        :key="card.label"
        class="overflow-hidden rounded-lg bg-white shadow"
      >
        <div class="p-5">
          <div class="flex items-center">
            <div :class="['shrink-0 rounded-md p-3', card.color]">
              <svg class="h-6 w-6 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7" />
              </svg>
            </div>
            <div class="ml-5 w-0 flex-1">
              <dl>
                <dt class="truncate text-sm font-medium text-gray-500">{{ card.label }}</dt>
                <dd class="text-3xl font-bold text-gray-900">{{ card.value }}</dd>
              </dl>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Quick actions -->
    <div v-if="pendingAgents > 0" class="rounded-lg border border-amber-200 bg-amber-50 p-4">
      <div class="flex items-center gap-3">
        <svg class="h-5 w-5 text-amber-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
        </svg>
        <span class="text-sm font-medium text-amber-800">
          {{ pendingAgents }} agent{{ pendingAgents > 1 ? 's' : '' }} pending approval
        </span>
        <router-link
          to="/agents"
          class="ml-auto rounded-md bg-amber-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-amber-700"
        >
          Review
        </router-link>
      </div>
    </div>

    <!-- Recent jobs -->
    <div class="rounded-lg bg-white shadow">
      <div class="border-b border-gray-200 px-6 py-4">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold text-gray-900">Recent Jobs</h2>
          <router-link to="/jobs" class="text-sm font-medium text-blue-600 hover:text-blue-700">
            View all
          </router-link>
        </div>
      </div>

      <LoadingSpinner v-if="jobsStore.loading" />
      <div v-else-if="recentJobs.length === 0" class="px-6 py-8 text-center text-sm text-gray-500">
        No jobs recorded yet.
      </div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200">
          <thead class="bg-gray-50">
            <tr>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Plan</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Status</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Trigger</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Started</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Duration</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200">
            <tr
              v-for="job in recentJobs"
              :key="job.id"
              class="cursor-pointer hover:bg-blue-50"
              @click="$router.push(`/jobs/${job.id}`)"
            >
              <td class="whitespace-nowrap px-4 py-3 text-sm font-medium text-gray-900">
                {{ job.plan_name || '-' }}
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-sm">
                <StatusBadge :status="job.status" />
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-sm capitalize text-gray-500">
                {{ job.trigger }}
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
                {{ relativeTime(job.started_at) }}
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
                {{ formatDuration(durationBetween(job.started_at, job.finished_at)) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
