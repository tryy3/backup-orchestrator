<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAgentsStore } from '../stores/agents'
import { useJobsStore } from '../stores/jobs'
import { usePlansStore } from '../stores/plans'
import StatusBadge from '../components/common/StatusBadge.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import { relativeTime, formatDate } from '../utils/time'

const route = useRoute()
const agentsStore = useAgentsStore()
const jobsStore = useJobsStore()
const plansStore = usePlansStore()

const agentId = computed(() => route.params.id as string)
const configOpen = ref(false)
const rcloneConfig = ref('')
const saving = ref(false)

// Reactive "now" that ticks every 5 seconds for live relative times.
const now = ref(Date.now())
let nowTimer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  nowTimer = setInterval(() => { now.value = Date.now() }, 5000)
  await agentsStore.fetchOne(agentId.value)
  if (agentsStore.current) {
    rcloneConfig.value = agentsStore.current.rclone_config || ''
  }
  plansStore.fetchAll({ agent_id: agentId.value })
  jobsStore.fetchAll({ agent_id: agentId.value })
})

onUnmounted(() => {
  if (nowTimer) clearInterval(nowTimer)
})

const agent = computed(() => agentsStore.current)

const isOnline = computed(() => {
  if (!agent.value?.last_heartbeat) return false
  return now.value - new Date(agent.value.last_heartbeat).getTime() < 5 * 60 * 1000
})

// Most recent job per plan
const recentJobByPlan = computed(() => {
  const map: Record<string, string> = {}
  for (const job of jobsStore.list) {
    if (!job.plan_id) continue
    if (!map[job.plan_id] || new Date(job.started_at) > new Date(map[job.plan_id])) {
      map[job.plan_id] = job.started_at
    }
  }
  return map
})

const failedPlanIds = computed(
  () =>
    new Set(
      jobsStore.list
        .filter((j) => j.status === 'failed' || j.status === 'partial')
        .map((j) => j.plan_id)
        .filter((id): id is string => id != null),
    ),
)

const failingPlans = computed(() => plansStore.list.filter((p) => failedPlanIds.value.has(p.id)))

// Live-updating relative time that refreshes with the `now` tick.
function liveRelativeTime(ts: string | null): string {
  void now.value // create reactive dependency
  return relativeTime(ts)
}

async function saveRclone() {
  saving.value = true
  await agentsStore.updateRclone(agentId.value, rcloneConfig.value)
  saving.value = false
}

</script>

<template>
  <div class="space-y-6">
    <LoadingSpinner v-if="agentsStore.loading && !agent" />

    <template v-else-if="agent">
      <!-- Page title -->
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="flex items-center gap-3">
            <div :class="['h-3 w-3 shrink-0 rounded-full', isOnline ? 'bg-green-400' : 'bg-slate-600']" />
            <h1 class="text-2xl font-bold tracking-tight text-slate-100">{{ agent.name }}</h1>
            <StatusBadge :status="agent.status" />
          </div>
          <p class="mt-1 pl-6 text-sm text-slate-500">{{ agent.hostname }}</p>
        </div>
        <div class="shrink-0 text-right text-xs text-slate-500">
          <div>Last heartbeat: <span class="text-slate-400">{{ liveRelativeTime(agent.last_heartbeat) }}</span></div>
          <div>v{{ agent.agent_version || '—' }}</div>
        </div>
      </div>

      <!-- Failing plan alert -->
      <div
        v-if="failingPlans.length > 0"
        class="flex items-center gap-3 rounded-lg border border-red-500/20 bg-red-500/5 px-4 py-3"
      >
        <div class="h-2 w-2 animate-pulse rounded-full bg-red-400 shrink-0" />
        <span class="text-sm text-red-300">
          {{ failingPlans.length }} plan{{ failingPlans.length !== 1 ? 's' : '' }} with recent failures
        </span>
      </div>

      <!-- Metadata grid -->
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        <div
          v-for="(item, i) in [
            { label: 'OS', value: agent.os || '—' },
            { label: 'Agent', value: agent.agent_version || '—' },
            { label: 'Restic', value: agent.restic_version || '—' },
            { label: 'Rclone', value: agent.rclone_version || '—' },
            { label: 'Config Version', value: String(agent.config_version) },
            { label: 'Registered', value: formatDate(agent.created_at) },
            { label: 'Config Applied', value: formatDate(agent.config_applied_at) },
            { label: 'Last Heartbeat', value: liveRelativeTime(agent.last_heartbeat) },
          ]"
          :key="i"
          class="rounded-lg border border-surface-700 bg-surface-900 p-3"
        >
          <dt class="text-[10px] font-medium uppercase tracking-wider text-slate-600">{{ item.label }}</dt>
          <dd class="mt-1 truncate text-sm text-slate-300">{{ item.value }}</dd>
        </div>
      </div>

      <!-- Backup Plans section -->
      <div>
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-semibold uppercase tracking-wider text-slate-500">Backup Plans</h2>
          <span class="text-xs text-slate-600">
            {{ plansStore.list.length }} plan{{ plansStore.list.length !== 1 ? 's' : '' }}
          </span>
        </div>
        <LoadingSpinner v-if="plansStore.loading" />
        <div
          v-else-if="plansStore.list.length === 0"
          class="rounded-lg border border-dashed border-surface-700 py-8 text-center text-sm text-slate-600"
        >
          No backup plans assigned to this agent.
        </div>
        <div v-else class="overflow-hidden rounded-lg border border-surface-700">
          <table class="min-w-full">
            <thead class="bg-surface-800">
              <tr>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Plan</th>
                <th class="hidden px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500 sm:table-cell">Schedule</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Status</th>
                <th class="hidden px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500 md:table-cell">Last Run</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-surface-700">
              <tr
                v-for="plan in plansStore.list"
                :key="plan.id"
                class="cursor-pointer transition-colors hover:bg-surface-800/60"
                @click="$router.push(`/agents/${agentId}/plans/${plan.id}`)"
              >
                <td class="px-4 py-3">
                  <div class="flex items-center gap-2">
                    <span
                      :class="[
                        'h-1.5 w-1.5 shrink-0 rounded-full',
                        failedPlanIds.has(plan.id)
                          ? 'bg-red-400'
                          : plan.enabled
                            ? 'bg-green-400'
                            : 'bg-slate-600',
                      ]"
                    />
                    <span class="text-sm font-medium text-slate-200">{{ plan.name }}</span>
                  </div>
                </td>
                <td class="hidden px-4 py-3 font-mono text-xs text-slate-400 sm:table-cell">
                  {{ plan.schedule || '—' }}
                </td>
                <td class="px-4 py-3">
                  <span
                    :class="[
                      'rounded-full px-2 py-0.5 text-xs font-medium',
                      plan.enabled
                        ? 'bg-green-500/10 text-green-400 ring-1 ring-green-500/20'
                        : 'bg-slate-700/40 text-slate-500 ring-1 ring-slate-700',
                    ]"
                  >
                    {{ plan.enabled ? 'Enabled' : 'Disabled' }}
                  </span>
                </td>
                <td class="hidden px-4 py-3 text-xs text-slate-500 md:table-cell">
                  {{ recentJobByPlan[plan.id] ? liveRelativeTime(recentJobByPlan[plan.id]) : '—' }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Rclone config (collapsible) -->
      <div class="rounded-lg border border-surface-700 bg-surface-900">
        <button
          class="flex w-full items-center justify-between px-4 py-3 text-sm text-slate-400 transition-colors hover:text-slate-200"
          @click="configOpen = !configOpen"
        >
          <span class="font-medium">Rclone Configuration</span>
          <svg
            :class="['h-4 w-4 transition-transform', configOpen ? 'rotate-180' : '']"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="2"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
          </svg>
        </button>
        <div v-if="configOpen" class="border-t border-surface-700 p-4">
          <textarea
            v-model="rcloneConfig"
            rows="12"
            class="block w-full rounded-md border border-surface-600 bg-surface-950 px-3 py-2 font-mono text-xs text-slate-300 placeholder:text-slate-600 focus:border-cyan-500 focus:outline-none focus:ring-1 focus:ring-cyan-500"
            placeholder="[remote-name]&#10;type = s3&#10;..."
          />
          <div class="mt-3 flex justify-end">
            <button
              class="rounded-md bg-cyan-500/10 px-4 py-2 text-sm font-medium text-cyan-400 ring-1 ring-cyan-500/30 transition-colors hover:bg-cyan-500/20 disabled:opacity-50"
              :disabled="saving"
              @click="saveRclone"
            >
              {{ saving ? 'Saving...' : 'Save Config' }}
            </button>
          </div>
        </div>
      </div>
    </template>

    <div v-else class="rounded-lg border border-surface-700 bg-surface-900 p-6 text-center text-slate-500">
      Agent not found.
    </div>
  </div>
</template>
