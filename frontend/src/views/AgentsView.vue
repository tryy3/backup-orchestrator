<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAgentsStore } from '../stores/agents'
import DataTable from '../components/common/DataTable.vue'
import StatusBadge from '../components/common/StatusBadge.vue'
import ConfirmDialog from '../components/common/ConfirmDialog.vue'
import { relativeTime } from '../utils/time'
import type { Column } from '../components/common/DataTable.vue'

const store = useAgentsStore()

onMounted(() => {
  store.fetchAll()
})

const columns: Column[] = [
  { key: 'name', label: 'Name', sortable: true },
  { key: 'hostname', label: 'Hostname', sortable: true },
  { key: 'status', label: 'Status', sortable: true },
  { key: 'last_heartbeat', label: 'Last Heartbeat', sortable: true },
  { key: 'agent_version', label: 'Version' },
  { key: 'actions', label: 'Actions' },
]

const confirmOpen = ref(false)
const confirmAgentId = ref('')
const confirmAction = ref<'delete' | 'approve' | 'reject'>('delete')

function openConfirm(id: string, action: 'delete' | 'approve' | 'reject') {
  confirmAgentId.value = id
  confirmAction.value = action
  confirmOpen.value = true
}

async function handleConfirm() {
  confirmOpen.value = false
  if (confirmAction.value === 'delete') {
    await store.remove(confirmAgentId.value)
  } else if (confirmAction.value === 'approve') {
    await store.approve(confirmAgentId.value)
  } else if (confirmAction.value === 'reject') {
    await store.reject(confirmAgentId.value)
  }
}

const confirmTitle = ref('')
const confirmMessage = ref('')

function getConfirmDetails() {
  if (confirmAction.value === 'delete') {
    confirmTitle.value = 'Delete Agent'
    confirmMessage.value = 'Are you sure you want to remove this agent? This action cannot be undone.'
  } else if (confirmAction.value === 'approve') {
    confirmTitle.value = 'Approve Agent'
    confirmMessage.value = 'Approve this agent to allow it to receive backup configurations?'
  } else {
    confirmTitle.value = 'Reject Agent'
    confirmMessage.value = 'Reject this agent? It will not be able to receive backup configurations.'
  }
}

function openConfirmDialog(id: string, action: 'delete' | 'approve' | 'reject') {
  openConfirm(id, action)
  getConfirmDetails()
}
</script>

<template>
  <div class="space-y-4">
    <DataTable
      :columns="columns"
      :rows="(store.list as unknown as Record<string, unknown>[])"
      :loading="store.loading"
      empty-title="No agents"
      empty-message="No agents have registered yet."
      @row-click="(row) => $router.push(`/agents/${row.id}`)"
    >
      <template #cell-name="{ row }">
        <router-link
          :to="`/agents/${row.id}`"
          class="font-medium text-accent hover:text-accent-dim"
          @click.stop
        >
          {{ row.name }}
        </router-link>
      </template>

      <template #cell-status="{ row }">
        <StatusBadge :status="(row.status as string)" />
      </template>

      <template #cell-last_heartbeat="{ row }">
        {{ relativeTime(row.last_heartbeat as string | null) }}
      </template>

      <template #cell-actions="{ row }">
        <div class="flex items-center gap-2">
          <template v-if="row.status === 'pending'">
            <button
              class="rounded bg-green-500/10 px-2.5 py-1 text-xs font-medium text-green-400 ring-1 ring-green-500/20 hover:bg-green-500/20"
              @click.stop="openConfirmDialog(row.id as string, 'approve')"
            >
              Approve
            </button>
            <button
              class="rounded bg-red-500/10 px-2.5 py-1 text-xs font-medium text-red-400 ring-1 ring-red-500/20 hover:bg-red-500/20"
              @click.stop="openConfirmDialog(row.id as string, 'reject')"
            >
              Reject
            </button>
          </template>
          <button
            class="rounded bg-red-500/10 px-2.5 py-1 text-xs font-medium text-red-400 ring-1 ring-red-500/20 hover:bg-red-500/20"
            @click.stop="openConfirmDialog(row.id as string, 'delete')"
          >
            Delete
          </button>
        </div>
      </template>
    </DataTable>

    <ConfirmDialog
      :open="confirmOpen"
      :title="confirmTitle"
      :message="confirmMessage"
      :confirm-text="confirmAction === 'delete' ? 'Delete' : confirmAction === 'approve' ? 'Approve' : 'Reject'"
      :confirm-variant="confirmAction === 'approve' ? 'primary' : 'danger'"
      @confirm="handleConfirm"
      @cancel="confirmOpen = false"
    />
  </div>
</template>
