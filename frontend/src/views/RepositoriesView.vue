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
      <div class="flex gap-1 rounded bg-surface-800 p-1">
        <button
          v-for="scope in (['all', 'global', 'local'] as const)"
          :key="scope"
          :class="[
            'rounded px-3 py-1.5 text-sm font-medium capitalize transition-colors',
            scopeFilter === scope
              ? 'bg-surface-900 text-slate-100 ring-1 ring-accent/30'
              : 'text-slate-500 hover:text-slate-300',
          ]"
          @click="onFilterChange(scope)"
        >
          {{ scope }}
        </button>
      </div>

      <router-link
        to="/repositories/new"
        class="rounded bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 transition-colors hover:bg-accent/20"
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
        <span class="font-medium text-slate-200">{{ row.name }}</span>
      </template>

      <template #cell-scope="{ row }">
        <span
          :class="[
            'rounded-full px-2 py-0.5 text-xs font-medium capitalize',
            row.scope === 'global'
              ? 'bg-cyan-500/10 text-cyan-400 ring-1 ring-cyan-500/20'
              : 'bg-surface-800 text-slate-400',
          ]"
        >
          {{ row.scope }}
        </span>
      </template>

      <template #cell-path="{ row }">
        <span class="font-mono text-xs text-slate-500">{{ row.path }}</span>
      </template>

      <template #cell-actions="{ row }">
        <div class="flex items-center gap-2">
          <router-link
            :to="`/repositories/${row.id}/edit`"
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
      title="Delete Repository"
      message="Are you sure you want to delete this repository? This action cannot be undone."
      confirm-text="Delete"
      confirm-variant="danger"
      @confirm="handleDelete"
      @cancel="confirmOpen = false"
    />
  </div>
</template>
