<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { usePlansStore } from '../stores/plans'
import { useAgentsStore } from '../stores/agents'
import DataTable from '../components/common/DataTable.vue'
import StatusBadge from '../components/common/StatusBadge.vue'
import ConfirmDialog from '../components/common/ConfirmDialog.vue'
import type { Column } from '../components/common/DataTable.vue'

const plansStore = usePlansStore()
const agentsStore = useAgentsStore()

const agentFilter = ref('')

onMounted(() => {
  agentsStore.fetchAll()
  loadPlans()
})

function loadPlans() {
  const params: { agent_id?: string } = {}
  if (agentFilter.value) params.agent_id = agentFilter.value
  plansStore.fetchAll(params)
}

const columns: Column[] = [
  { key: 'name', label: 'Name', sortable: true },
  { key: 'agent_id', label: 'Agent' },
  { key: 'schedule', label: 'Schedule' },
  { key: 'repository_ids', label: 'Repos' },
  { key: 'enabled', label: 'Enabled' },
  { key: 'actions', label: 'Actions' },
]

const agentMap = computed(() => {
  const m = new Map<string, string>()
  for (const a of agentsStore.list) {
    m.set(a.id, a.name)
  }
  return m
})

const confirmOpen = ref(false)
const deleteId = ref('')

function openDelete(id: string) {
  deleteId.value = id
  confirmOpen.value = true
}

async function handleDelete() {
  confirmOpen.value = false
  await plansStore.remove(deleteId.value)
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <div>
        <select
          v-model="agentFilter"
          class="rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
          @change="loadPlans()"
        >
          <option value="">All agents</option>
          <option v-for="agent in agentsStore.list" :key="agent.id" :value="agent.id">
            {{ agent.name }}
          </option>
        </select>
      </div>

      <router-link
        to="/plans/new"
        class="rounded bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 transition-colors hover:bg-accent/20"
      >
        New Plan
      </router-link>
    </div>

    <DataTable
      :columns="columns"
      :rows="(plansStore.list as unknown as Record<string, unknown>[])"
      :loading="plansStore.loading"
      empty-title="No backup plans"
      empty-message="Create a backup plan to start scheduling backups."
      @row-click="(row) => $router.push(`/plans/${row.id}`)"
    >
      <template #cell-name="{ row }">
        <router-link
          :to="`/plans/${row.id}`"
          class="font-medium text-accent hover:text-accent-dim"
          @click.stop
        >
          {{ row.name }}
        </router-link>
      </template>

      <template #cell-agent_id="{ row }">
        {{ agentMap.get(row.agent_id as string) || row.agent_id }}
      </template>

      <template #cell-repository_ids="{ row }">
        {{ (row.repository_ids as string[])?.length ?? 0 }} repo{{ (row.repository_ids as string[])?.length === 1 ? '' : 's' }}
      </template>

      <template #cell-enabled="{ row }">
        <StatusBadge :status="row.enabled ? 'active' : 'offline'" />
      </template>

      <template #cell-actions="{ row }">
        <div class="flex items-center gap-2">
          <router-link
            :to="`/plans/${row.id}`"
            class="rounded bg-surface-800 px-2.5 py-1 text-xs font-medium text-slate-300 hover:bg-surface-700"
            @click.stop
          >
            View
          </router-link>
          <router-link
            :to="`/plans/${row.id}/edit`"
            class="rounded bg-surface-800 px-2.5 py-1 text-xs font-medium text-slate-300 hover:bg-surface-700"
            @click.stop
          >
            Edit
          </router-link>
          <button
            class="rounded bg-red-500/10 px-2.5 py-1 text-xs font-medium text-red-400 ring-1 ring-red-500/20 hover:bg-red-500/20"
            @click.stop="openDelete(row.id as string)"
          >
            Delete
          </button>
        </div>
      </template>
    </DataTable>

    <ConfirmDialog
      :open="confirmOpen"
      title="Delete Plan"
      message="Are you sure you want to delete this backup plan? This action cannot be undone."
      confirm-text="Delete"
      confirm-variant="danger"
      @confirm="handleDelete"
      @cancel="confirmOpen = false"
    />
  </div>
</template>
