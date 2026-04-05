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
        class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
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
        <span class="font-medium text-gray-900">{{ row.name }}</span>
      </template>

      <template #cell-command="{ row }">
        <code class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-700">
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
            row.on_error === 'abort' ? 'bg-red-100 text-red-700' : 'bg-gray-100 text-gray-700',
          ]"
        >
          {{ row.on_error }}
        </span>
      </template>

      <template #cell-actions="{ row }">
        <div class="flex items-center gap-2">
          <router-link
            :to="`/scripts/${row.id}/edit`"
            class="rounded bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-700 hover:bg-gray-200"
            @click.stop
          >
            Edit
          </router-link>
          <button
            class="rounded bg-red-100 px-2.5 py-1 text-xs font-medium text-red-700 hover:bg-red-200"
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
