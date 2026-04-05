<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useJobsStore } from '../stores/jobs'
import { useAgentsStore } from '../stores/agents'
import { usePlansStore } from '../stores/plans'
import DataTable from '../components/common/DataTable.vue'
import StatusBadge from '../components/common/StatusBadge.vue'
import { formatDate, formatDuration, durationBetween } from '../utils/time'
import type { Column } from '../components/common/DataTable.vue'

const router = useRouter()
const jobsStore = useJobsStore()
const agentsStore = useAgentsStore()
const plansStore = usePlansStore()

const agentFilter = ref('')
const planFilter = ref('')
const statusFilter = ref('')

onMounted(() => {
  agentsStore.fetchAll()
  plansStore.fetchAll()
  loadJobs()
})

function loadJobs() {
  const params: { agent_id?: string; plan_id?: string; status?: string } = {}
  if (agentFilter.value) params.agent_id = agentFilter.value
  if (planFilter.value) params.plan_id = planFilter.value
  if (statusFilter.value) params.status = statusFilter.value
  jobsStore.fetchAll(params)
}

const columns: Column[] = [
  { key: 'started_at', label: 'Started', sortable: true },
  { key: 'plan_name', label: 'Plan', sortable: true },
  { key: 'agent_id', label: 'Agent' },
  { key: 'type', label: 'Type' },
  { key: 'trigger', label: 'Trigger' },
  { key: 'status', label: 'Status' },
  { key: 'duration', label: 'Duration' },
]

const agentMap = new Map<string, string>()

function getAgentName(id: string) {
  if (agentMap.size === 0) {
    for (const a of agentsStore.list) {
      agentMap.set(a.id, a.name)
    }
  }
  return agentMap.get(id) || id.slice(0, 8)
}

const statuses = ['', 'running', 'success', 'partial', 'failed']
</script>

<template>
  <div class="space-y-4">
    <!-- Filters -->
    <div class="flex flex-wrap gap-3">
      <select
        v-model="agentFilter"
        class="rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
        @change="loadJobs()"
      >
        <option value="">All agents</option>
        <option v-for="agent in agentsStore.list" :key="agent.id" :value="agent.id">
          {{ agent.name }}
        </option>
      </select>

      <select
        v-model="planFilter"
        class="rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
        @change="loadJobs()"
      >
        <option value="">All plans</option>
        <option v-for="plan in plansStore.list" :key="plan.id" :value="plan.id">
          {{ plan.name }}
        </option>
      </select>

      <select
        v-model="statusFilter"
        class="rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm capitalize text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
        @change="loadJobs()"
      >
        <option v-for="s in statuses" :key="s" :value="s">
          {{ s || 'All statuses' }}
        </option>
      </select>
    </div>

    <DataTable
      :columns="columns"
      :rows="(jobsStore.list as unknown as Record<string, unknown>[])"
      :loading="jobsStore.loading"
      empty-title="No jobs"
      empty-message="No jobs match the current filters."
      @row-click="(row) => router.push(`/jobs/${row.id}`)"
    >
      <template #cell-started_at="{ row }">
        {{ formatDate(row.started_at as string) }}
      </template>

      <template #cell-plan_name="{ row }">
        <span class="font-medium text-slate-200">{{ row.plan_name || '-' }}</span>
      </template>

      <template #cell-agent_id="{ row }">
        {{ getAgentName(row.agent_id as string) }}
      </template>

      <template #cell-type="{ row }">
        <span class="capitalize">{{ row.type }}</span>
      </template>

      <template #cell-trigger="{ row }">
        <span class="capitalize">{{ row.trigger }}</span>
      </template>

      <template #cell-status="{ row }">
        <StatusBadge :status="(row.status as string)" />
      </template>

      <template #cell-duration="{ row }">
        {{ formatDuration(durationBetween(row.started_at as string, row.finished_at as string | null)) }}
      </template>
    </DataTable>
  </div>
</template>
