<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useAgentsStore } from '../stores/agents'
import { useRepositoriesStore } from '../stores/repositories'
import { useSnapshotsStore } from '../stores/snapshots'
import DataTable from '../components/common/DataTable.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import FileBrowser from '../components/common/FileBrowser.vue'
import { formatDate } from '../utils/time'
import type { Column } from '../components/common/DataTable.vue'

const agentsStore = useAgentsStore()
const repoStore = useRepositoriesStore()
const snapshotsStore = useSnapshotsStore()

const selectedAgent = ref('')
const selectedRepo = ref('')
const restoreDialogOpen = ref(false)
const restoreSnapshotId = ref('')
const restoreTarget = ref('/mnt/restore')
const restorePaths = ref('')
const restoreSuccess = ref<boolean | null>(null)
const restoreMessage = ref('')

onMounted(() => {
  agentsStore.fetchAll()
  repoStore.fetchAll()
})

const availableRepos = computed(() => {
  if (!selectedAgent.value) return []
  return repoStore.list.filter(
    (r) => r.scope === 'global' || r.agent_id === selectedAgent.value,
  )
})

function loadSnapshots() {
  if (selectedAgent.value && selectedRepo.value) {
    snapshotsStore.fetchList(selectedAgent.value, selectedRepo.value)
  }
}

const columns: Column[] = [
  { key: 'id', label: 'Snapshot ID' },
  { key: 'time', label: 'Time', sortable: true },
  { key: 'hostname', label: 'Hostname' },
  { key: 'tags', label: 'Tags' },
  { key: 'paths', label: 'Paths' },
  { key: 'actions', label: 'Actions' },
]

function openRestore(snapshotId: string) {
  restoreSnapshotId.value = snapshotId
  restoreTarget.value = '/mnt/restore'
  restorePaths.value = ''
  restoreSuccess.value = null
  restoreMessage.value = ''
  restoreDialogOpen.value = true
}

async function handleRestore() {
  const paths = restorePaths.value
    .split('\n')
    .map((p) => p.trim())
    .filter((p) => p.length > 0)

  const ok = await snapshotsStore.restore(selectedAgent.value, {
    repository_id: selectedRepo.value,
    snapshot_id: restoreSnapshotId.value,
    paths,
    target: restoreTarget.value,
  })

  if (ok) {
    restoreSuccess.value = true
    restoreMessage.value = 'Restore triggered successfully.'
  } else {
    restoreSuccess.value = false
    restoreMessage.value = snapshotsStore.error ?? 'Restore failed.'
  }
}
</script>

<template>
  <div class="space-y-4">
    <!-- Selectors -->
    <div class="flex flex-wrap gap-3">
      <div>
        <label class="block text-sm font-medium text-slate-400">Agent</label>
        <select
          v-model="selectedAgent"
          class="mt-1 rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
          @change="selectedRepo = ''; snapshotsStore.list = []"
        >
          <option value="">Select agent</option>
          <option v-for="a in agentsStore.list" :key="a.id" :value="a.id">
            {{ a.name }}
          </option>
        </select>
      </div>

      <div>
        <label class="block text-sm font-medium text-slate-400">Repository</label>
        <select
          v-model="selectedRepo"
          :disabled="!selectedAgent"
          class="mt-1 rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30 disabled:bg-surface-800 disabled:text-slate-600"
        >
          <option value="">Select repository</option>
          <option v-for="r in availableRepos" :key="r.id" :value="r.id">
            {{ r.name }} ({{ r.scope }})
          </option>
        </select>
      </div>

      <div class="flex items-end">
        <button
          :disabled="!selectedAgent || !selectedRepo"
          class="rounded bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 transition-colors hover:bg-accent/20 disabled:opacity-50"
          @click="loadSnapshots"
        >
          Load Snapshots
        </button>
      </div>
    </div>

    <LoadingSpinner v-if="snapshotsStore.loading" />

    <DataTable
      v-else-if="snapshotsStore.list.length > 0 || (selectedAgent && selectedRepo)"
      :columns="columns"
      :rows="(snapshotsStore.list as unknown as Record<string, unknown>[])"
      :loading="snapshotsStore.loading"
      empty-title="No snapshots"
      empty-message="No snapshots found for this agent and repository."
    >
      <template #cell-id="{ row }">
        <span class="font-mono text-xs">{{ row.id }}</span>
      </template>

      <template #cell-time="{ row }">
        {{ formatDate(row.time as string) }}
      </template>

      <template #cell-tags="{ row }">
        <span
          v-for="tag in (row.tags as string[] ?? [])"
          :key="tag"
          class="mr-1 rounded-full bg-surface-800 px-2 py-0.5 text-xs text-slate-400"
        >
          {{ tag }}
        </span>
        <span v-if="!(row.tags as string[])?.length" class="text-slate-600">-</span>
      </template>

      <template #cell-paths="{ row }">
        <span class="font-mono text-xs text-slate-500">
          {{ (row.paths as string[] ?? []).join(', ') || '-' }}
        </span>
      </template>

      <template #cell-actions="{ row }">
        <button
          class="rounded bg-green-500/10 px-2.5 py-1 text-xs font-medium text-green-400 ring-1 ring-green-500/20 hover:bg-green-500/20"
          @click.stop="openRestore(row.id as string)"
        >
          Restore
        </button>
      </template>
    </DataTable>

    <!-- Restore dialog -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition-opacity duration-200"
        leave-active-class="transition-opacity duration-150"
        enter-from-class="opacity-0"
        leave-to-class="opacity-0"
      >
        <div v-if="restoreDialogOpen" class="fixed inset-0 z-50 flex items-center justify-center">
          <div class="absolute inset-0 bg-black/50" @click="restoreDialogOpen = false" />
          <div class="relative z-10 w-full max-w-md rounded border border-surface-600 bg-surface-800 p-6 shadow-xl">
            <h3 class="text-lg font-semibold text-slate-100">Restore Snapshot</h3>
            <p class="mt-1 text-sm text-slate-400">
              Restoring snapshot <code class="rounded bg-surface-700 px-1 font-mono text-xs text-slate-300">{{ restoreSnapshotId }}</code>
            </p>

            <div v-if="restoreSuccess !== null" :class="[
              'mt-3 rounded border p-3 text-sm',
              restoreSuccess
                ? 'border-green-500/20 bg-green-500/10 text-green-400'
                : 'border-red-500/20 bg-red-500/10 text-red-400',
            ]">
              {{ restoreMessage }}
            </div>

            <div class="mt-4 space-y-4">
              <div>
                <label class="block text-sm font-medium text-slate-400">Target path</label>
                <div class="mt-1">
                  <FileBrowser
                    v-model="restoreTarget"
                    :agent-id="selectedAgent"
                  />
                </div>
              </div>

              <div>
                <label class="block text-sm font-medium text-slate-400">
                  Specific paths (optional, one per line)
                </label>
                <textarea
                  v-model="restorePaths"
                  rows="3"
                  class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 font-mono text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
                  placeholder="Leave empty to restore all"
                />
              </div>
            </div>

            <div class="mt-6 flex justify-end gap-3">
              <button
                class="rounded border border-surface-600 bg-surface-700 px-4 py-2 text-sm font-medium text-slate-300 transition-colors hover:bg-surface-600"
                @click="restoreDialogOpen = false"
              >
                Close
              </button>
              <button
                class="rounded bg-green-500/10 px-4 py-2 text-sm font-medium text-green-400 ring-1 ring-green-500/20 transition-colors hover:bg-green-500/20"
                @click="handleRestore"
              >
                Start Restore
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
