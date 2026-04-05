<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRepositoriesStore } from '../stores/repositories'
import DataTable from '../components/common/DataTable.vue'
import ConfirmDialog from '../components/common/ConfirmDialog.vue'
import type { Column } from '../components/common/DataTable.vue'

const store = useRepositoriesStore()

const scopeFilter = ref<'all' | 'global' | 'local'>('all')

onMounted(() => {
  loadRepos()
})

function loadRepos() {
  const params: { scope?: string } = {}
  if (scopeFilter.value !== 'all') params.scope = scopeFilter.value
  store.fetchAll(params)
}

function onFilterChange(scope: 'all' | 'global' | 'local') {
  scopeFilter.value = scope
  loadRepos()
}

const columns: Column[] = [
  { key: 'name', label: 'Name', sortable: true },
  { key: 'type', label: 'Type', sortable: true },
  { key: 'scope', label: 'Scope', sortable: true },
  { key: 'path', label: 'Path' },
  { key: 'actions', label: 'Actions' },
]

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
    <div class="flex items-center justify-between">
      <!-- Filter tabs -->
      <div class="flex gap-1 rounded-lg bg-gray-100 p-1">
        <button
          v-for="scope in (['all', 'global', 'local'] as const)"
          :key="scope"
          :class="[
            'rounded-md px-3 py-1.5 text-sm font-medium capitalize',
            scopeFilter === scope
              ? 'bg-white text-gray-900 shadow-sm'
              : 'text-gray-600 hover:text-gray-900',
          ]"
          @click="onFilterChange(scope)"
        >
          {{ scope }}
        </button>
      </div>

      <router-link
        to="/repositories/new"
        class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
      >
        New Repository
      </router-link>
    </div>

    <DataTable
      :columns="columns"
      :rows="(store.list as unknown as Record<string, unknown>[])"
      :loading="store.loading"
      empty-title="No repositories"
      empty-message="Create a repository to get started."
    >
      <template #cell-name="{ row }">
        <span class="font-medium text-gray-900">{{ row.name }}</span>
      </template>

      <template #cell-scope="{ row }">
        <span
          :class="[
            'rounded-full px-2 py-0.5 text-xs font-medium capitalize',
            row.scope === 'global' ? 'bg-blue-100 text-blue-800' : 'bg-gray-100 text-gray-700',
          ]"
        >
          {{ row.scope }}
        </span>
      </template>

      <template #cell-path="{ row }">
        <span class="font-mono text-xs text-gray-600">{{ row.path }}</span>
      </template>

      <template #cell-actions="{ row }">
        <div class="flex items-center gap-2">
          <router-link
            :to="`/repositories/${row.id}/edit`"
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
      title="Delete Repository"
      message="Are you sure you want to delete this repository? This action cannot be undone."
      confirm-text="Delete"
      confirm-variant="danger"
      @confirm="handleDelete"
      @cancel="confirmOpen = false"
    />
  </div>
</template>
