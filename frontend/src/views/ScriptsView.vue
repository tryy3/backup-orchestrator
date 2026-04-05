<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useScriptsStore } from '../stores/scripts'
import DataTable from '../components/common/DataTable.vue'
import ConfirmDialog from '../components/common/ConfirmDialog.vue'
import type { Column } from '../components/common/DataTable.vue'

const store = useScriptsStore()

onMounted(() => {
  store.fetchAll()
})

const columns: Column[] = [
  { key: 'name', label: 'Name', sortable: true },
  { key: 'command', label: 'Command' },
  { key: 'timeout', label: 'Timeout', sortable: true },
  { key: 'on_error', label: 'On Error' },
  { key: 'actions', label: 'Actions' },
]

function truncate(str: string, len: number): string {
  if (!str) return '-'
  return str.length > len ? str.slice(0, len) + '...' : str
}

const confirmOpen = ref(false)
const deleteId = ref('')

function openDelete(id: string) {
  deleteId.value = id
  confirmOpen.value = true
}

async function handleDelete() {
  confirmOpen.value = false
  await store.remove(deleteId.value)
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex justify-end">
      <router-link
        to="/scripts/new"
        class="rounded bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 transition-colors hover:bg-accent/20"
      >
        New Script
      </router-link>
    </div>

    <DataTable
      :columns="columns"
      :rows="(store.list as unknown as Record<string, unknown>[])"
      :loading="store.loading"
      empty-title="No scripts"
      empty-message="Create reusable scripts for hooks."
    >
      <template #cell-name="{ row }">
        <span class="font-medium text-slate-200">{{ row.name }}</span>
      </template>

      <template #cell-command="{ row }">
        <code class="rounded bg-surface-800 px-1.5 py-0.5 font-mono text-xs text-slate-300">
          {{ truncate(row.command as string, 60) }}
        </code>
      </template>

      <template #cell-timeout="{ row }">
        {{ row.timeout }}s
      </template>

      <template #cell-on_error="{ row }">
        <span
          :class="[
            'rounded-full px-2 py-0.5 text-xs font-medium capitalize',
            row.on_error === 'abort'
              ? 'bg-red-500/15 text-red-400 ring-1 ring-red-500/30'
              : 'bg-surface-800 text-slate-400',
          ]"
        >
          {{ row.on_error }}
        </span>
      </template>

      <template #cell-actions="{ row }">
        <div class="flex items-center gap-2">
          <router-link
            :to="`/scripts/${row.id}/edit`"
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
      title="Delete Script"
      message="Are you sure? This will fail if the script is still referenced by plan hooks."
      confirm-text="Delete"
      confirm-variant="danger"
      @confirm="handleDelete"
      @cancel="confirmOpen = false"
    />
  </div>
</template>
