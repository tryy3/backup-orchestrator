<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAgentsStore } from '../stores/agents'
import { useJobsStore } from '../stores/jobs'
import { usePlansStore } from '../stores/plans'
import StatusBadge from '../components/common/StatusBadge.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import DataTable from '../components/common/DataTable.vue'
import { relativeTime, formatDate, formatDuration, durationBetween } from '../utils/time'
import type { Column } from '../components/common/DataTable.vue'

const route = useRoute()
const agentsStore = useAgentsStore()
const jobsStore = useJobsStore()
const plansStore = usePlansStore()

const agentId = computed(() => route.params.id as string)
const activeTab = ref<'rclone' | 'plans' | 'jobs'>('plans')
const rcloneConfig = ref('')
const saving = ref(false)

onMounted(async () => {
  await agentsStore.fetchOne(agentId.value)
  if (agentsStore.current) {
    rcloneConfig.value = agentsStore.current.rclone_config || ''
  }
  plansStore.fetchAll({ agent_id: agentId.value })
  jobsStore.fetchAll({ agent_id: agentId.value })
})

const agent = computed(() => agentsStore.current)

const jobColumns: Column[] = [
  { key: 'plan_name', label: 'Plan', sortable: true },
  { key: 'status', label: 'Status' },
  { key: 'trigger', label: 'Trigger' },
  { key: 'started_at', label: 'Started', sortable: true },
  { key: 'duration', label: 'Duration' },
]

const recentJobs = computed(() => {
  return [...jobsStore.list]
    .sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime())
    .slice(0, 20)
})

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
      <!-- Agent info card -->
      <div class="rounded-lg bg-white p-6 shadow">
        <div class="flex items-start justify-between">
          <div>
            <h2 class="text-xl font-bold text-gray-900">{{ agent.name }}</h2>
            <p class="mt-1 text-sm text-gray-500">{{ agent.hostname }}</p>
          </div>
          <StatusBadge :status="agent.status" />
        </div>

        <div class="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
          <div>
            <dt class="text-xs font-medium text-gray-500">OS</dt>
            <dd class="mt-1 text-sm text-gray-900">{{ agent.os || '-' }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Agent Version</dt>
            <dd class="mt-1 text-sm text-gray-900">{{ agent.agent_version || '-' }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Restic Version</dt>
            <dd class="mt-1 text-sm text-gray-900">{{ agent.restic_version || '-' }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Rclone Version</dt>
            <dd class="mt-1 text-sm text-gray-900">{{ agent.rclone_version || '-' }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Config Version</dt>
            <dd class="mt-1 text-sm text-gray-900">{{ agent.config_version }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Last Heartbeat</dt>
            <dd class="mt-1 text-sm text-gray-900">{{ relativeTime(agent.last_heartbeat) }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Config Applied</dt>
            <dd class="mt-1 text-sm text-gray-900">{{ formatDate(agent.config_applied_at) }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Registered</dt>
            <dd class="mt-1 text-sm text-gray-900">{{ formatDate(agent.created_at) }}</dd>
          </div>
        </div>
      </div>

      <!-- Tabs -->
      <div class="border-b border-gray-200">
        <nav class="-mb-px flex gap-6">
          <button
            v-for="tab in (['plans', 'jobs', 'rclone'] as const)"
            :key="tab"
            :class="[
              'border-b-2 pb-3 text-sm font-medium capitalize',
              activeTab === tab
                ? 'border-blue-600 text-blue-600'
                : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700',
            ]"
            @click="activeTab = tab"
          >
            {{ tab === 'rclone' ? 'Rclone Config' : tab === 'plans' ? 'Backup Plans' : 'Recent Jobs' }}
          </button>
        </nav>
      </div>

      <!-- Tab: Backup Plans -->
      <div v-if="activeTab === 'plans'" class="space-y-3">
        <div v-if="plansStore.list.length === 0" class="rounded-lg bg-white p-6 text-center text-sm text-gray-500 shadow">
          No backup plans assigned to this agent.
        </div>
        <div v-else class="space-y-2">
          <router-link
            v-for="plan in plansStore.list"
            :key="plan.id"
            :to="`/plans/${plan.id}`"
            class="block rounded-lg bg-white p-4 shadow hover:bg-blue-50"
          >
            <div class="flex items-center justify-between">
              <div>
                <span class="font-medium text-gray-900">{{ plan.name }}</span>
                <span class="ml-2 text-sm text-gray-500">{{ plan.schedule }}</span>
              </div>
              <span
                :class="[
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  plan.enabled ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-600',
                ]"
              >
                {{ plan.enabled ? 'Enabled' : 'Disabled' }}
              </span>
            </div>
          </router-link>
        </div>
      </div>

      <!-- Tab: Recent Jobs -->
      <div v-if="activeTab === 'jobs'">
        <DataTable
          :columns="jobColumns"
          :rows="(recentJobs as unknown as Record<string, unknown>[])"
          :loading="jobsStore.loading"
          empty-title="No jobs"
          empty-message="No jobs have been recorded for this agent."
          @row-click="(row) => $router.push(`/jobs/${row.id}`)"
        >
          <template #cell-status="{ row }">
            <StatusBadge :status="(row.status as string)" />
          </template>
          <template #cell-started_at="{ row }">
            {{ relativeTime(row.started_at as string) }}
          </template>
          <template #cell-duration="{ row }">
            {{ formatDuration(durationBetween(row.started_at as string, row.finished_at as string | null)) }}
          </template>
        </DataTable>
      </div>

      <!-- Tab: Rclone Config -->
      <div v-if="activeTab === 'rclone'" class="rounded-lg bg-white p-6 shadow">
        <label class="block text-sm font-medium text-gray-700">Rclone Configuration (INI format)</label>
        <textarea
          v-model="rcloneConfig"
          rows="12"
          class="mt-2 block w-full rounded-md border border-gray-300 px-3 py-2 font-mono text-sm focus:border-blue-500 focus:ring-blue-500"
          placeholder="[remote-name]
type = s3
provider = AWS
access_key_id = ...
secret_access_key = ...
region = us-east-1"
        />
        <div class="mt-4 flex justify-end">
          <button
            class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            :disabled="saving"
            @click="saveRclone"
          >
            {{ saving ? 'Saving...' : 'Save Rclone Config' }}
          </button>
        </div>
      </div>
    </template>

    <div v-else class="rounded-lg bg-white p-6 text-center text-gray-500 shadow">
      Agent not found.
    </div>
  </div>
</template>
