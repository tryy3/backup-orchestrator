<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useAgentsStore } from '../stores/agents'
import { useJobsStore } from '../stores/jobs'
import type { Agent, Job } from '../types/api'

const agentsStore = useAgentsStore()
const jobsStore = useJobsStore()

const filterStatus = ref<'all' | 'failing' | 'warning' | 'healthy' | 'offline'>('all')

onMounted(() => {
  agentsStore.fetchAll()
  jobsStore.fetchAll()
})

// Group jobs by agent_id, filtered to past 30 days
const jobsByAgent = computed((): Record<string, Job[]> => {
  const map: Record<string, Job[]> = {}
  const cutoff = Date.now() - 30 * 24 * 60 * 60 * 1000
  for (const job of jobsStore.list) {
    if (new Date(job.started_at).getTime() < cutoff) continue
    if (!map[job.agent_id]) map[job.agent_id] = []
    map[job.agent_id].push(job)
  }
  return map
})

// Returns 30-day sparkline data (oldest → newest)
function getSparkline(agentId: string) {
  const jobs = jobsByAgent.value[agentId] ?? []
  const days: { success: number; failure: number; total: number }[] = []
  for (let i = 29; i >= 0; i--) {
    const dayStart = new Date()
    dayStart.setHours(0, 0, 0, 0)
    dayStart.setDate(dayStart.getDate() - i)
    const dayEnd = new Date(dayStart)
    dayEnd.setDate(dayEnd.getDate() + 1)
    const dayJobs = jobs.filter((j) => {
      const t = new Date(j.started_at).getTime()
      return t >= dayStart.getTime() && t < dayEnd.getTime()
    })
    days.push({
      success: dayJobs.filter((j) => j.status === 'success').length,
      failure: dayJobs.filter((j) => j.status === 'failed' || j.status === 'partial').length,
      total: dayJobs.length,
    })
  }
  return days
}

function isOnline(agent: Agent): boolean {
  if (!agent.last_heartbeat) return false
  return Date.now() - new Date(agent.last_heartbeat).getTime() < 5 * 60 * 1000
}

function agentHealthStatus(agent: Agent): 'healthy' | 'warning' | 'failing' | 'offline' {
  if (agent.status !== 'approved' || !isOnline(agent)) return 'offline'
  const jobs = jobsByAgent.value[agent.id] ?? []
  if (!jobs.length) return 'healthy'
  const rate = jobs.filter((j) => j.status === 'success').length / jobs.length
  if (rate < 0.9) return 'failing'
  if (rate < 0.99) return 'warning'
  return 'healthy'
}

function reliabilityText(agentId: string): string {
  const jobs = jobsByAgent.value[agentId] ?? []
  if (!jobs.length) return '—'
  return ((jobs.filter((j) => j.status === 'success').length / jobs.length) * 100).toFixed(1) + '%'
}

function reliabilityColor(agentId: string): string {
  const jobs = jobsByAgent.value[agentId] ?? []
  if (!jobs.length) return 'text-slate-500'
  const rate = jobs.filter((j) => j.status === 'success').length / jobs.length
  if (rate >= 0.99) return 'text-green-400'
  if (rate >= 0.9) return 'text-amber-400'
  return 'text-red-400'
}

const pendingAgents = computed(() => agentsStore.list.filter((a) => a.status === 'pending'))

const healthCounts = computed(() => {
  const counts = { healthy: 0, warning: 0, failing: 0, offline: 0 }
  for (const agent of agentsStore.list) counts[agentHealthStatus(agent)]++
  return counts
})

const filteredAgents = computed(() => {
  if (filterStatus.value === 'all') return agentsStore.list
  return agentsStore.list.filter((a) => agentHealthStatus(a) === filterStatus.value)
})

const globalSuccessRate = computed(() => {
  const recent = jobsStore.list.filter(
    (j) => Date.now() - new Date(j.started_at).getTime() < 30 * 24 * 60 * 60 * 1000,
  )
  if (!recent.length) return null
  return ((recent.filter((j) => j.status === 'success').length / recent.length) * 100).toFixed(1)
})

const maxJobsPerDay = computed(() => {
  let max = 1
  for (const agent of agentsStore.list) {
    for (const d of getSparkline(agent.id)) {
      if (d.total > max) max = d.total
    }
  }
  return max
})

function cardBorderClass(status: ReturnType<typeof agentHealthStatus>) {
  if (status === 'healthy') return 'border-green-500/20 hover:border-green-500/50'
  if (status === 'warning') return 'border-amber-500/30 hover:border-amber-500/60'
  if (status === 'failing') return 'border-red-500/40 hover:border-red-500/70'
  return 'border-surface-600 hover:border-surface-500'
}

function statusDotClass(status: ReturnType<typeof agentHealthStatus>) {
  if (status === 'healthy') return 'bg-green-400'
  if (status === 'warning') return 'bg-amber-400'
  if (status === 'failing') return 'bg-red-400 animate-pulse'
  return 'bg-slate-600'
}
</script>

<template>
  <div class="space-y-6">
    <!-- Page header -->
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold tracking-tight text-slate-100">Fleet Overview</h1>
        <p class="mt-1 text-sm text-slate-500">
          {{ agentsStore.list.filter((a) => a.status === 'approved').length }} active
          agent{{ agentsStore.list.filter((a) => a.status === 'approved').length !== 1 ? 's' : '' }}
          · 30-day monitoring
        </p>
      </div>
      <div v-if="globalSuccessRate !== null" class="text-right">
        <div
          :class="[
            'text-3xl font-bold tabular-nums',
            Number(globalSuccessRate) >= 99
              ? 'text-green-400'
              : Number(globalSuccessRate) >= 90
                ? 'text-amber-400'
                : 'text-red-400',
          ]"
        >
          {{ globalSuccessRate }}%
        </div>
        <div class="text-xs uppercase tracking-wider text-slate-500">Global Success Rate</div>
      </div>
    </div>

    <!-- Pending approval banner -->
    <div
      v-if="pendingAgents.length > 0"
      class="flex items-center gap-3 rounded-lg border border-amber-500/20 bg-amber-500/5 px-4 py-3"
    >
      <div class="h-2 w-2 animate-pulse rounded-full bg-amber-400 shrink-0" />
      <span class="text-sm text-amber-300">
        {{ pendingAgents.length }} agent{{ pendingAgents.length !== 1 ? 's' : '' }} pending approval
      </span>
      <router-link to="/agents" class="ml-auto text-xs font-medium text-amber-400 hover:text-amber-300">
        Review →
      </router-link>
    </div>

    <!-- Filter bar -->
    <div class="flex items-center gap-2 overflow-x-auto pb-0.5">
      <button
        v-for="f in ([
          { value: 'all', label: 'All', count: agentsStore.list.length, dot: '' },
          { value: 'failing', label: 'Failing', count: healthCounts.failing, dot: 'bg-red-400' },
          { value: 'warning', label: 'Warning', count: healthCounts.warning, dot: 'bg-amber-400' },
          { value: 'healthy', label: 'Healthy', count: healthCounts.healthy, dot: 'bg-green-400' },
          { value: 'offline', label: 'Offline', count: healthCounts.offline, dot: 'bg-slate-600' },
        ] as const)"
        :key="f.value"
        :class="[
          'flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
          filterStatus === f.value
            ? 'bg-accent/10 text-accent ring-1 ring-accent/30'
            : 'bg-surface-800 text-slate-400 hover:bg-surface-700 hover:text-slate-300',
        ]"
        @click="filterStatus = f.value"
      >
        <span v-if="f.dot" :class="['h-1.5 w-1.5 rounded-full', f.dot]" />
        {{ f.label }}
        <span class="text-slate-500">({{ f.count }})</span>
      </button>
    </div>

    <!-- Loading state -->
    <div
      v-if="agentsStore.loading && !agentsStore.list.length"
      class="flex flex-col items-center justify-center gap-3 py-20"
    >
      <svg class="h-8 w-8 animate-spin text-cyan-400" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
        <path
          class="opacity-75"
          fill="currentColor"
          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
        />
      </svg>
      <span class="text-sm text-slate-500">Loading fleet data...</span>
    </div>

    <!-- Empty state -->
    <div
      v-else-if="filteredAgents.length === 0"
      class="flex flex-col items-center justify-center rounded-lg border border-dashed border-surface-700 py-16 text-center"
    >
      <svg
        class="mb-3 h-10 w-10 text-slate-700"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="1.5"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3"
        />
      </svg>
      <p class="text-sm text-slate-500">No agents found</p>
      <p v-if="filterStatus !== 'all'" class="mt-1 text-xs text-slate-600">Try a different filter</p>
    </div>

    <!-- Agent cards grid -->
    <div v-else class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      <router-link
        v-for="agent in filteredAgents"
        :key="agent.id"
        :to="`/agents/${agent.id}`"
        :class="[
          'group block rounded-lg border bg-surface-900 p-4 transition-all',
          cardBorderClass(agentHealthStatus(agent)),
        ]"
      >
        <!-- Card header -->
        <div class="mb-3 flex items-start justify-between gap-2">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span :class="['h-2 w-2 shrink-0 rounded-full', statusDotClass(agentHealthStatus(agent))]" />
              <span class="truncate text-sm font-semibold text-slate-100">{{ agent.name }}</span>
            </div>
            <p class="mt-0.5 truncate pl-4 text-xs text-slate-500">{{ agent.hostname || agent.os || '—' }}</p>
          </div>
          <span
            v-if="agent.status === 'pending'"
            class="shrink-0 rounded-full bg-amber-500/15 px-2 py-0.5 text-xs font-medium text-amber-400 ring-1 ring-amber-500/30"
          >
            Pending
          </span>
        </div>

        <!-- 30-day sparkline histogram -->
        <div class="mb-3 flex items-end gap-px" style="height: 28px">
          <div
            v-for="(day, i) in getSparkline(agent.id)"
            :key="i"
            :class="[
              'flex-1 rounded-[1px] transition-colors',
              day.failure > 0
                ? 'bg-red-500/70'
                : day.success > 0
                  ? 'bg-green-500/50'
                  : 'bg-surface-700',
            ]"
            :style="{
              height: day.total === 0 ? '2px' : Math.max(3, (day.total / maxJobsPerDay) * 26) + 'px',
            }"
            :title="`${30 - i}d ago: ${day.success} ok, ${day.failure} failed`"
          />
        </div>

        <!-- Reliability stat -->
        <div class="flex items-center justify-between">
          <span class="text-xs uppercase tracking-wider text-slate-600">30d reliability</span>
          <span :class="['text-sm font-semibold tabular-nums', reliabilityColor(agent.id)]">
            {{ reliabilityText(agent.id) }}
          </span>
        </div>
      </router-link>
    </div>
  </div>
</template>
