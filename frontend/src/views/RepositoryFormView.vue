<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useRepositoriesStore } from '../stores/repositories'
import { useAgentsStore } from '../stores/agents'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
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
        path: repoStore.current.path,
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
</script>

<template>
  <div class="mx-auto max-w-2xl">
    <LoadingSpinner v-if="formLoading" />
    <form v-else class="space-y-6 rounded-lg bg-white p-6 shadow" @submit.prevent="handleSubmit">
      <div v-if="repoStore.error" class="rounded-md bg-red-50 p-3 text-sm text-red-700">
        {{ repoStore.error }}
      </div>

      <!-- Name -->
      <div>
        <label class="block text-sm font-medium text-gray-700">Name</label>
        <input
          v-model="form.name"
          type="text"
          required
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
          placeholder="e.g. local-nas, s3-offsite"
        />
      </div>

      <!-- Scope -->
      <div>
        <label class="block text-sm font-medium text-gray-700">Scope</label>
        <div class="mt-2 flex gap-4">
          <label class="flex items-center gap-2">
            <input v-model="form.scope" type="radio" value="global" class="text-blue-600" />
            <span class="text-sm text-gray-700">Global (available to all agents)</span>
          </label>
          <label class="flex items-center gap-2">
            <input v-model="form.scope" type="radio" value="local" class="text-blue-600" />
            <span class="text-sm text-gray-700">Local (bound to one agent)</span>
          </label>
        </div>
      </div>

      <!-- Agent (shown for local scope) -->
      <div v-if="form.scope === 'local'">
        <label class="block text-sm font-medium text-gray-700">Agent</label>
        <select
          v-model="form.agent_id"
          required
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
        >
          <option value="" disabled>Select an agent</option>
          <option v-for="agent in approvedAgents" :key="agent.id" :value="agent.id">
            {{ agent.name }} ({{ agent.hostname }})
          </option>
        </select>
      </div>

      <!-- Type -->
      <div>
        <label class="block text-sm font-medium text-gray-700">Type</label>
        <select
          v-model="form.type"
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
        >
          <option v-for="t in repoTypes" :key="t" :value="t">{{ t }}</option>
        </select>
      </div>

      <!-- Path -->
      <div>
        <label class="block text-sm font-medium text-gray-700">Path</label>
        <input
          v-model="form.path"
          type="text"
          required
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 font-mono text-sm focus:border-blue-500 focus:ring-blue-500"
          placeholder="e.g. /mnt/backup or rclone:remote:bucket/path"
        />
      </div>

      <!-- Password -->
      <div>
        <label class="block text-sm font-medium text-gray-700">Repository Password</label>
        <input
          v-model="form.password"
          type="password"
          required
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
          placeholder="Restic repository password"
        />
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-3 border-t border-gray-200 pt-4">
        <router-link
          to="/repositories"
          class="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
        >
          Cancel
        </router-link>
        <button
          type="submit"
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          :disabled="saving"
        >
          {{ saving ? 'Saving...' : isEdit ? 'Update Repository' : 'Create Repository' }}
        </button>
      </div>
    </form>
  </div>
</template>
