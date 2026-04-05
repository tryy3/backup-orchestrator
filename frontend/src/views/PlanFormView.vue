<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { usePlansStore } from '../stores/plans'
import { useAgentsStore } from '../stores/agents'
import RetentionEditor from '../components/plans/RetentionEditor.vue'
import RepositoryPicker from '../components/plans/RepositoryPicker.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import type { BackupPlanCreate, RetentionPolicy } from '../types/api'

const route = useRoute()
const router = useRouter()
const plansStore = usePlansStore()
const agentsStore = useAgentsStore()

const isEdit = computed(() => !!route.params.id)
const planId = computed(() => route.params.id as string)

const form = ref<BackupPlanCreate>({
  name: '',
  agent_id: '',
  paths: [''],
  excludes: [],
  tags: [],
  repository_ids: [],
  schedule: '',
  forget_after_backup: true,
  prune_after_forget: true,
  prune_schedule: '',
  retention: null,
  enabled: true,
})

const overrideRetention = ref(false)
const retentionForm = ref<RetentionPolicy>({
  keep_last: 5,
  keep_hourly: 0,
  keep_daily: 7,
  keep_weekly: 4,
  keep_monthly: 6,
  keep_yearly: 0,
})

const saving = ref(false)
const formLoading = ref(false)

onMounted(async () => {
  agentsStore.fetchAll()
  if (isEdit.value) {
    formLoading.value = true
    await plansStore.fetchOne(planId.value)
    if (plansStore.current) {
      const p = plansStore.current
      form.value = {
        name: p.name,
        agent_id: p.agent_id,
        paths: p.paths.length > 0 ? [...p.paths] : [''],
        excludes: p.excludes ? [...p.excludes] : [],
        tags: p.tags ? [...p.tags] : [],
        repository_ids: [...p.repository_ids],
        schedule: p.schedule,
        forget_after_backup: p.forget_after_backup,
        prune_after_forget: p.prune_after_forget,
        prune_schedule: p.prune_schedule || '',
        retention: p.retention ? { ...p.retention } : null,
        enabled: p.enabled,
      }
      if (p.retention) {
        overrideRetention.value = true
        retentionForm.value = { ...p.retention }
      }
    }
    formLoading.value = false
  }
})

function addPath() {
  form.value.paths.push('')
}

function removePath(idx: number) {
  form.value.paths.splice(idx, 1)
}

function addExclude() {
  if (!form.value.excludes) form.value.excludes = []
  form.value.excludes.push('')
}

function removeExclude(idx: number) {
  form.value.excludes?.splice(idx, 1)
}

function addTag() {
  if (!form.value.tags) form.value.tags = []
  form.value.tags.push('')
}

function removeTag(idx: number) {
  form.value.tags?.splice(idx, 1)
}

const approvedAgents = computed(() => agentsStore.list.filter((a) => a.status === 'approved'))

async function handleSubmit() {
  saving.value = true

  const data = { ...form.value }
  data.paths = data.paths.filter((p) => p.trim() !== '')
  data.excludes = (data.excludes ?? []).filter((e) => e.trim() !== '')
  data.tags = (data.tags ?? []).filter((t) => t.trim() !== '')
  data.retention = overrideRetention.value ? { ...retentionForm.value } : null

  let result
  if (isEdit.value) {
    result = await plansStore.update(planId.value, data)
  } else {
    result = await plansStore.create(data)
  }

  saving.value = false
  if (result) {
    router.push(`/plans/${result.id}`)
  }
}

const cronPatterns = [
  { label: 'Daily at 2 AM', value: '0 2 * * *' },
  { label: 'Every 6 hours', value: '0 */6 * * *' },
  { label: 'Weekly Sunday 3 AM', value: '0 3 * * 0' },
  { label: 'Monthly 1st at 4 AM', value: '0 4 1 * *' },
]
</script>

<template>
  <div class="mx-auto max-w-3xl">
    <LoadingSpinner v-if="formLoading" />
    <form v-else class="space-y-8" @submit.prevent="handleSubmit">
      <div v-if="plansStore.error" class="rounded border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-400">
        {{ plansStore.error }}
      </div>

      <!-- Section: Basic -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-lg font-semibold text-slate-100">Basic Information</h3>

        <div class="space-y-4">
          <div>
            <label class="block text-sm font-medium text-slate-400">Name</label>
            <input
              v-model="form.name"
              type="text"
              required
              class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
              placeholder="e.g. daily-home, database-backup"
            />
          </div>

          <div>
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

          <div class="flex items-center gap-2">
            <input
              v-model="form.enabled"
              type="checkbox"
              class="rounded text-accent"
            />
            <label class="text-sm font-medium text-slate-400">Enabled</label>
          </div>
        </div>
      </div>

      <!-- Section: Paths -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-lg font-semibold text-slate-100">Paths</h3>

        <div class="space-y-3">
          <label class="block text-sm font-medium text-slate-400">Paths to back up</label>
          <div v-for="(_, idx) in form.paths" :key="idx" class="flex gap-2">
            <input
              v-model="form.paths[idx]"
              type="text"
              class="block flex-1 rounded border border-surface-600 bg-surface-950 px-3 py-2 font-mono text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
              placeholder="/path/to/backup"
            />
            <button
              v-if="form.paths.length > 1"
              type="button"
              class="rounded px-2 text-red-400 hover:bg-red-500/10"
              @click="removePath(idx)"
            >
              &times;
            </button>
          </div>
          <button
            type="button"
            class="text-sm font-medium text-accent hover:text-accent-dim"
            @click="addPath"
          >
            + Add path
          </button>
        </div>

        <div class="mt-6 space-y-3">
          <label class="block text-sm font-medium text-slate-400">Excludes</label>
          <div v-for="(_, idx) in (form.excludes ?? [])" :key="idx" class="flex gap-2">
            <input
              v-model="form.excludes![idx]"
              type="text"
              class="block flex-1 rounded border border-surface-600 bg-surface-950 px-3 py-2 font-mono text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
              placeholder="*.tmp, .cache, node_modules"
            />
            <button
              type="button"
              class="rounded px-2 text-red-400 hover:bg-red-500/10"
              @click="removeExclude(idx)"
            >
              &times;
            </button>
          </div>
          <button
            type="button"
            class="text-sm font-medium text-accent hover:text-accent-dim"
            @click="addExclude"
          >
            + Add exclude
          </button>
        </div>
      </div>

      <!-- Section: Schedule -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-lg font-semibold text-slate-100">Schedule</h3>

        <div>
          <label class="block text-sm font-medium text-slate-400">Cron expression</label>
          <input
            v-model="form.schedule"
            type="text"
            required
            class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 font-mono text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
            placeholder="0 2 * * *"
          />
          <div class="mt-2 flex flex-wrap gap-2">
            <button
              v-for="pat in cronPatterns"
              :key="pat.value"
              type="button"
              class="rounded bg-surface-800 px-2 py-1 text-xs text-slate-300 hover:bg-surface-700"
              @click="form.schedule = pat.value"
            >
              {{ pat.label }}
            </button>
          </div>
        </div>
      </div>

      <!-- Section: Repositories -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-lg font-semibold text-slate-100">Repositories</h3>
        <RepositoryPicker v-model="form.repository_ids" :agent-id="form.agent_id" />
      </div>

      <!-- Section: Retention -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-lg font-semibold text-slate-100">Retention Policy</h3>

        <label class="flex items-center gap-2">
          <input
            v-model="overrideRetention"
            type="checkbox"
            class="rounded text-accent"
          />
          <span class="text-sm font-medium text-slate-400">Override global defaults</span>
        </label>

        <div v-if="overrideRetention" class="mt-4">
          <RetentionEditor v-model="retentionForm" />
        </div>
      </div>

      <!-- Section: Forget/Prune -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-lg font-semibold text-slate-100">Forget & Prune</h3>

        <div class="space-y-3">
          <label class="flex items-center gap-2">
            <input
              v-model="form.forget_after_backup"
              type="checkbox"
              class="rounded text-accent"
            />
            <span class="text-sm text-slate-400">Run forget after backup</span>
          </label>

          <label class="flex items-center gap-2">
            <input
              v-model="form.prune_after_forget"
              type="checkbox"
              class="rounded text-accent"
            />
            <span class="text-sm text-slate-400">Run prune after forget</span>
          </label>

          <div v-if="!form.prune_after_forget">
            <label class="block text-sm font-medium text-slate-400">Separate prune schedule</label>
            <input
              v-model="form.prune_schedule"
              type="text"
              class="mt-1 block w-64 rounded border border-surface-600 bg-surface-950 px-3 py-2 font-mono text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
              placeholder="0 4 * * 0"
            />
          </div>
        </div>
      </div>

      <!-- Section: Tags -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-lg font-semibold text-slate-100">Tags</h3>
        <div class="space-y-2">
          <div v-for="(_, idx) in (form.tags ?? [])" :key="idx" class="flex gap-2">
            <input
              v-model="form.tags![idx]"
              type="text"
              class="block flex-1 rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
              placeholder="e.g. critical:yes"
            />
            <button
              type="button"
              class="rounded px-2 text-red-400 hover:bg-red-500/10"
              @click="removeTag(idx)"
            >
              &times;
            </button>
          </div>
          <button
            type="button"
            class="text-sm font-medium text-accent hover:text-accent-dim"
            @click="addTag"
          >
            + Add tag
          </button>
        </div>
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-3">
        <router-link
          to="/plans"
          class="rounded border border-surface-600 bg-surface-700 px-4 py-2 text-sm font-medium text-slate-300 transition-colors hover:bg-surface-600"
        >
          Cancel
        </router-link>
        <button
          type="submit"
          class="rounded bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 transition-colors hover:bg-accent/20 disabled:opacity-50"
          :disabled="saving"
        >
          {{ saving ? 'Saving...' : isEdit ? 'Update Plan' : 'Create Plan' }}
        </button>
      </div>
    </form>
  </div>
</template>
