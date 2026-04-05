<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAgentsStore } from '../stores/agents'
import { usePlansStore } from '../stores/plans'
import { useJobsStore } from '../stores/jobs'
import StatusBadge from '../components/common/StatusBadge.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import { formatDate, formatDuration, durationBetween } from '../utils/time'

const route = useRoute()
const agentsStore = useAgentsStore()
const plansStore = usePlansStore()
const jobsStore = useJobsStore()

const agentId = computed(() => route.params.id as string)
const planId = computed(() => route.params.planId as string)

onMounted(() => {
  agentsStore.fetchOne(agentId.value)
  plansStore.fetchOne(planId.value)
  jobsStore.fetchAll({ plan_id: planId.value })
})

const plan = computed(() => plansStore.current)

const sortedJobs = computed(() =>
  [...jobsStore.list].sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime()),
)

const completedJobs = computed(() => jobsStore.list.filter((j) => j.status !== 'running'))

const successRate = computed(() => {
  if (!completedJobs.value.length) return null
  const successes = completedJobs.value.filter((j) => j.status === 'success').length
  return ((successes / completedJobs.value.length) * 100).toFixed(1)
})

const avgDuration = computed(() => {
  const finished = jobsStore.list.filter((j) => j.finished_at)
  if (!finished.length) return null
  const totalMs = finished.reduce((sum, j) => sum + (durationBetween(j.started_at, j.finished_at) ?? 0), 0)
  return formatDuration(totalMs / finished.length)
})

async function triggerBackup() {
  if (!plan.value) return
  await plansStore.trigger(plan.value.id)
  jobsStore.fetchAll({ plan_id: planId.value })
}
</script>

<template>
  <div class="space-y-6">
    <LoadingSpinner v-if="plansStore.loading && !plan" />

    <template v-else-if="plan">
      <!-- Page header -->
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-bold tracking-tight text-slate-100">{{ plan.name }}</h1>
          <p class="mt-1 text-sm text-slate-500">
            <span class="font-mono">{{ plan.schedule || 'No schedule' }}</span>
            <span class="mx-2 text-slate-700">·</span>
            <span>{{ agentsStore.current?.name ?? agentId }}</span>
          </p>
        </div>
        <button
          class="shrink-0 rounded-md bg-cyan-500/10 px-4 py-2 text-sm font-medium text-cyan-400 ring-1 ring-cyan-500/30 transition-colors hover:bg-cyan-500/20"
          @click="triggerBackup"
        >
          ▶ Trigger Backup
        </button>
      </div>

      <!-- KPI row -->
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <div class="rounded-lg border border-surface-700 bg-surface-900 p-4">
          <dt class="text-xs font-medium uppercase tracking-wider text-slate-600">Success Rate</dt>
          <dd
            :class="[
              'mt-1 text-2xl font-bold tabular-nums',
              successRate == null
                ? 'text-slate-500'
                : Number(successRate) >= 99
                  ? 'text-green-400'
                  : Number(successRate) >= 90
                    ? 'text-amber-400'
                    : 'text-red-400',
            ]"
          >
            {{ successRate != null ? successRate + '%' : '—' }}
          </dd>
          <dd class="mt-0.5 text-xs text-slate-600">{{ completedJobs.length }} completed</dd>
        </div>
        <div class="rounded-lg border border-surface-700 bg-surface-900 p-4">
          <dt class="text-xs font-medium uppercase tracking-wider text-slate-600">Avg Duration</dt>
          <dd class="mt-1 text-2xl font-bold text-slate-200">{{ avgDuration ?? '—' }}</dd>
          <dd class="mt-0.5 text-xs text-slate-600">per backup run</dd>
        </div>
        <div class="rounded-lg border border-surface-700 bg-surface-900 p-4">
          <dt class="text-xs font-medium uppercase tracking-wider text-slate-600">Plan Status</dt>
          <dd class="mt-2">
            <span
              :class="[
                'inline-flex rounded-full px-2.5 py-0.5 text-sm font-medium',
                plan.enabled
                  ? 'bg-green-500/10 text-green-400 ring-1 ring-green-500/20'
                  : 'bg-slate-700/40 text-slate-500 ring-1 ring-slate-700',
              ]"
            >
              {{ plan.enabled ? 'Enabled' : 'Disabled' }}
            </span>
          </dd>
          <dd class="mt-1 text-xs text-slate-600">current state</dd>
        </div>
      </div>

      <!-- Job executions table -->
      <div>
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-semibold uppercase tracking-wider text-slate-500">Job History</h2>
          <span class="text-xs text-slate-600">{{ sortedJobs.length }} total</span>
        </div>

        <LoadingSpinner v-if="jobsStore.loading" />

        <div
          v-else-if="sortedJobs.length === 0"
          class="rounded-lg border border-dashed border-surface-700 py-12 text-center text-sm text-slate-600"
        >
          No jobs recorded for this plan yet.
        </div>

        <div v-else class="overflow-hidden rounded-lg border border-surface-700">
          <table class="min-w-full">
            <thead class="bg-surface-800">
              <tr>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Job</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Started</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Duration</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Trigger</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Status</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-surface-700">
              <tr
                v-for="job in sortedJobs"
                :key="job.id"
                class="cursor-pointer transition-colors hover:bg-surface-800/60"
                @click="$router.push(`/agents/${agentId}/plans/${planId}/jobs/${job.id}`)"
              >
                <td class="px-4 py-3 font-mono text-xs text-slate-400">{{ job.id.slice(0, 8) }}…</td>
                <td class="px-4 py-3 text-sm text-slate-300">{{ formatDate(job.started_at) }}</td>
                <td class="px-4 py-3 text-sm text-slate-400">
                  {{ formatDuration(durationBetween(job.started_at, job.finished_at)) }}
                </td>
                <td class="px-4 py-3 text-xs">
                  <span
                    :class="[
                      'rounded-full px-2 py-0.5 font-medium capitalize',
                      job.trigger === 'manual'
                        ? 'bg-cyan-500/10 text-cyan-400 ring-1 ring-cyan-500/20'
                        : 'bg-surface-700/50 text-slate-500',
                    ]"
                  >
                    {{ job.trigger }}
                  </span>
                </td>
                <td class="px-4 py-3">
                  <StatusBadge :status="job.status" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Plan configuration summary -->
      <div class="rounded-lg border border-surface-700 bg-surface-900 p-4">
        <h3 class="mb-3 text-xs font-semibold uppercase tracking-wider text-slate-600">Plan Configuration</h3>
        <div class="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
          <div v-if="plan.paths?.length">
            <dt class="mb-1 text-xs text-slate-600">Paths</dt>
            <dd class="space-y-0.5">
              <div v-for="p in plan.paths" :key="p" class="truncate font-mono text-xs text-slate-400">{{ p }}</div>
            </dd>
          </div>
          <div v-if="plan.excludes?.length">
            <dt class="mb-1 text-xs text-slate-600">Excludes</dt>
            <dd class="space-y-0.5">
              <div v-for="e in plan.excludes" :key="e" class="truncate font-mono text-xs text-slate-400">{{ e }}</div>
            </dd>
          </div>
          <div>
            <dt class="mb-1 text-xs text-slate-600">Schedule</dt>
            <dd class="font-mono text-xs text-slate-400">{{ plan.schedule || '—' }}</dd>
          </div>
          <div>
            <dt class="mb-1 text-xs text-slate-600">Repositories</dt>
            <dd class="text-xs text-slate-400">{{ plan.repository_ids?.length ?? 0 }} configured</dd>
          </div>
        </div>
      </div>
    </template>

    <div v-else class="rounded-lg border border-surface-700 bg-surface-900 p-6 text-center text-slate-500">
      Plan not found.
    </div>
  </div>
</template>
