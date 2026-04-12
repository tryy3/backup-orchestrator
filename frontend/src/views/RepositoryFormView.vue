<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useRepositoriesStore } from '../stores/repositories'
import { useAgentsStore } from '../stores/agents'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import FileBrowser from '../components/common/FileBrowser.vue'
import type { RepositoryCreate } from '../types/api'

const route = useRoute()
const router = useRouter()
const repoStore = useRepositoriesStore()
const agentsStore = useAgentsStore()

const isEdit = computed(() => !!route.params.id)
const repoId = computed(() => route.params.id as string)

const form = ref<RepositoryCreate>({
  name: '',
  scope: 'global',
  agent_id: undefined,
  type: 'local',
  path: '',
  password: '',
})

const saving = ref(false)
const formLoading = ref(false)

const repoTypes = ['local', 'rclone', 'sftp', 's3', 'b2', 'rest', 'azure', 'gs']

const typePrefix = computed(() => form.value.type + ':')

const pathPlaceholder = computed(() => {
  switch (form.value.type) {
    case 'local': return '/mnt/backup'
    case 'rclone': return 'remote:bucket/path'
    case 'sftp': return 'user@host:/path'
    case 's3': return 'bucketname/path'
    case 'b2': return 'bucketname:/path'
    case 'rest': return 'http://host:8000/path'
    case 'azure': return 'containername:/path'
    case 'gs': return 'bucketname:/path'
    default: return '/path/to/repo'
  }
})

function stripTypePrefix(path: string, type: string): string {
  const prefix = type + ':'
  return path.startsWith(prefix) ? path.slice(prefix.length) : path
}

// When editing and the user changes the type, strip the old prefix if present
watch(() => form.value.type, (_newType, oldType) => {
  if (oldType) {
    form.value.path = stripTypePrefix(form.value.path, oldType)
  }
})

onMounted(async () => {
  agentsStore.fetchAll()
  if (isEdit.value) {
    formLoading.value = true
    await repoStore.fetchOne(repoId.value)
    if (repoStore.current) {
      form.value = {
        name: repoStore.current.name,
        scope: repoStore.current.scope,
        agent_id: repoStore.current.agent_id ?? undefined,
        type: repoStore.current.type,
        path: stripTypePrefix(repoStore.current.path, repoStore.current.type),
        password: repoStore.current.password,
      }
    }
    formLoading.value = false
  }
})

async function handleSubmit() {
  saving.value = true
  const data = { ...form.value }
  if (data.scope === 'global') {
    data.agent_id = undefined
  }

  // Auto-prepend type prefix to path (guard against manual double-prefix)
  const prefix = data.type + ':'
  if (!data.path.startsWith(prefix)) {
    data.path = prefix + data.path
  }

  let result
  if (isEdit.value) {
    result = await repoStore.update(repoId.value, data)
  } else {
    result = await repoStore.create(data)
  }

  saving.value = false
  if (result) {
    router.push('/repositories')
  }
}

const approvedAgents = computed(() => agentsStore.list.filter((a) => a.status === 'approved'))

const canBrowse = computed(() =>
  form.value.scope === 'local' && form.value.type === 'local' && !!form.value.agent_id,
)
</script>

<template>
  <div class="mx-auto max-w-2xl">
    <LoadingSpinner v-if="formLoading" />
    <form v-else class="space-y-6 rounded border border-surface-700 bg-surface-900 p-6" @submit.prevent="handleSubmit">
      <div v-if="repoStore.error" class="rounded border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-400">
        {{ repoStore.error }}
      </div>

      <!-- Name -->
      <div>
        <label class="block text-sm font-medium text-slate-400">Name</label>
        <input
          v-model="form.name"
          type="text"
          required
          class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
          placeholder="e.g. local-nas, s3-offsite"
        />
      </div>

      <!-- Scope -->
      <div>
        <label class="block text-sm font-medium text-slate-400">Scope</label>
        <div class="mt-2 flex gap-4">
          <label class="flex items-center gap-2">
            <input v-model="form.scope" type="radio" value="global" class="text-accent" />
            <span class="text-sm text-slate-300">Global (available to all agents)</span>
          </label>
          <label class="flex items-center gap-2">
            <input v-model="form.scope" type="radio" value="local" class="text-accent" />
            <span class="text-sm text-slate-300">Local (bound to one agent)</span>
          </label>
        </div>
      </div>

      <!-- Agent (shown for local scope) -->
      <div v-if="form.scope === 'local'">
        <label class="block text-sm font-medium text-slate-400">Agent</label>
        <select
          v-model="form.agent_id"
          required
          class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
        >
          <option value="" disabled>Select an agent</option>
          <option v-for="agent in approvedAgents" :key="agent.id" :value="agent.id">
            {{ agent.name }} ({{ agent.hostname }})
          </option>
        </select>
      </div>

      <!-- Type -->
      <div>
        <label class="block text-sm font-medium text-slate-400">Type</label>
        <select
          v-model="form.type"
          class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
        >
          <option v-for="t in repoTypes" :key="t" :value="t">{{ t }}</option>
        </select>
      </div>

      <!-- Path -->
      <div>
        <label class="block text-sm font-medium text-slate-400">Path</label>
        <div class="mt-1">
          <FileBrowser
            v-if="canBrowse"
            v-model="form.path"
            :agent-id="form.agent_id!"
          />
          <div v-else class="flex">
            <span
              class="inline-flex items-center rounded-l border border-r-0 border-surface-600 bg-surface-800 px-3 font-mono text-sm text-slate-400"
            >
              {{ typePrefix }}
            </span>
            <input
              v-model="form.path"
              type="text"
              required
              class="block w-full rounded-r border border-surface-600 bg-surface-950 px-3 py-2 font-mono text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
              :placeholder="pathPlaceholder"
            />
          </div>
        </div>
      </div>

      <!-- Password -->
      <div>
        <label class="block text-sm font-medium text-slate-400">Repository Password</label>
        <input
          v-model="form.password"
          type="password"
          required
          class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
          placeholder="Restic repository password"
        />
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-3 border-t border-surface-700 pt-4">
        <router-link
          to="/repositories"
          class="rounded border border-surface-600 bg-surface-700 px-4 py-2 text-sm font-medium text-slate-300 transition-colors hover:bg-surface-600"
        >
          Cancel
        </router-link>
        <button
          type="submit"
          class="rounded bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 transition-colors hover:bg-accent/20 disabled:opacity-50"
          :disabled="saving"
        >
          {{ saving ? 'Saving...' : isEdit ? 'Update Repository' : 'Create Repository' }}
        </button>
      </div>
    </form>
  </div>
</template>
