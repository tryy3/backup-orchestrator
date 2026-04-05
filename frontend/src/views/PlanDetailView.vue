<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { usePlansStore } from '../stores/plans'
import { useAgentsStore } from '../stores/agents'
import { useJobsStore } from '../stores/jobs'
import { useScriptsStore } from '../stores/scripts'
import { hooks as hooksApi } from '../api/client'
import StatusBadge from '../components/common/StatusBadge.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import DataTable from '../components/common/DataTable.vue'
import HookEditor from '../components/plans/HookEditor.vue'
import ConfirmDialog from '../components/common/ConfirmDialog.vue'
import { relativeTime, formatDuration, durationBetween } from '../utils/time'
import type { PlanHook } from '../types/api'
import type { Column } from '../components/common/DataTable.vue'

const route = useRoute()
const router = useRouter()
const plansStore = usePlansStore()
const agentsStore = useAgentsStore()
const jobsStore = useJobsStore()
const scriptsStore = useScriptsStore()

const planId = computed(() => route.params.id as string)
const plan = computed(() => plansStore.current)

const planHooks = ref<PlanHook[]>([])
const hooksLoading = ref(false)
const showHookEditor = ref(false)
const editingHook = ref<PlanHook | undefined>(undefined)
const triggerLoading = ref(false)

const deleteHookConfirm = ref(false)
const deleteHookId = ref('')

onMounted(async () => {
  await Promise.all([
    plansStore.fetchOne(planId.value),
    agentsStore.fetchAll(),
    scriptsStore.fetchAll(),
  ])
  if (plan.value) {
    jobsStore.fetchAll({ plan_id: planId.value })
  }
  loadHooks()
})

async function loadHooks() {
  hooksLoading.value = true
  try {
    planHooks.value = await hooksApi.list(planId.value)
  } catch (_) {
    // ignore
  }
  hooksLoading.value = false
}

const agentName = computed(() => {
  if (!plan.value) return ''
  const agent = agentsStore.list.find((a) => a.id === plan.value!.agent_id)
  return agent?.name ?? plan.value.agent_id
})

const scriptMap = computed(() => {
  const m = new Map<string, string>()
  for (const s of scriptsStore.list) {
    m.set(s.id, s.name)
  }
  return m
})

const sortedHooks = computed(() =>
  [...planHooks.value].sort((a, b) => a.sort_order - b.sort_order),
)

const jobColumns: Column[] = [
  { key: 'status', label: 'Status' },
  { key: 'trigger', label: 'Trigger' },
  { key: 'started_at', label: 'Started', sortable: true },
  { key: 'duration', label: 'Duration' },
]

const recentJobs = computed(() =>
  [...jobsStore.list]
    .sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime())
    .slice(0, 20),
)

async function triggerBackup() {
  triggerLoading.value = true
  await plansStore.trigger(planId.value)
  triggerLoading.value = false
}

function startAddHook() {
  editingHook.value = undefined
  showHookEditor.value = true
}

function startEditHook(hook: PlanHook) {
  editingHook.value = hook
  showHookEditor.value = true
}

async function onHookSaved() {
  showHookEditor.value = false
  editingHook.value = undefined
  await loadHooks()
}

function confirmDeleteHook(hookId: string) {
  deleteHookId.value = hookId
  deleteHookConfirm.value = true
}

async function handleDeleteHook() {
  deleteHookConfirm.value = false
  await hooksApi.remove(planId.value, deleteHookId.value)
  await loadHooks()
}

async function moveHook(hookId: string, direction: 'up' | 'down') {
  const ids = sortedHooks.value.map((h) => h.id)
  const idx = ids.indexOf(hookId)
  if (direction === 'up' && idx > 0) {
    ;[ids[idx - 1], ids[idx]] = [ids[idx], ids[idx - 1]]
  } else if (direction === 'down' && idx < ids.length - 1) {
    ;[ids[idx + 1], ids[idx]] = [ids[idx], ids[idx + 1]]
  }
  await hooksApi.reorder(planId.value, ids)
  await loadHooks()
}
</script>

<template>
  <div class="space-y-6">
    <LoadingSpinner v-if="plansStore.loading && !plan" />

    <template v-else-if="plan">
      <!-- Plan info -->
      <div class="rounded-lg bg-white p-6 shadow">
        <div class="flex items-start justify-between">
          <div>
            <h2 class="text-xl font-bold text-gray-900">{{ plan.name }}</h2>
            <p class="mt-1 text-sm text-gray-500">
              Agent: <router-link :to="`/agents/${plan.agent_id}`" class="text-blue-600 hover:text-blue-700">{{ agentName }}</router-link>
            </p>
          </div>
          <div class="flex items-center gap-3">
            <StatusBadge :status="plan.enabled ? 'active' : 'offline'" />
            <button
              class="rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white hover:bg-green-700 disabled:opacity-50"
              :disabled="triggerLoading"
              @click="triggerBackup"
            >
              {{ triggerLoading ? 'Triggering...' : 'Trigger Backup' }}
            </button>
            <router-link
              :to="`/plans/${plan.id}/edit`"
              class="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
            >
              Edit
            </router-link>
          </div>
        </div>

        <div class="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-3">
          <div>
            <dt class="text-xs font-medium text-gray-500">Schedule</dt>
            <dd class="mt-1 font-mono text-sm text-gray-900">{{ plan.schedule }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Paths</dt>
            <dd class="mt-1 text-sm text-gray-900">
              <div v-for="p in plan.paths" :key="p" class="font-mono text-xs">{{ p }}</div>
            </dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Excludes</dt>
            <dd class="mt-1 text-sm text-gray-900">
              <div v-if="plan.excludes?.length" v-for="e in plan.excludes" :key="e" class="font-mono text-xs">{{ e }}</div>
              <span v-else class="text-gray-400">None</span>
            </dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Tags</dt>
            <dd class="mt-1 text-sm text-gray-900">
              <span v-if="plan.tags?.length" v-for="t in plan.tags" :key="t" class="mr-1 rounded-full bg-gray-100 px-2 py-0.5 text-xs">{{ t }}</span>
              <span v-else class="text-gray-400">None</span>
            </dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Forget after backup</dt>
            <dd class="mt-1 text-sm text-gray-900">{{ plan.forget_after_backup ? 'Yes' : 'No' }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500">Prune after forget</dt>
            <dd class="mt-1 text-sm text-gray-900">{{ plan.prune_after_forget ? 'Yes' : 'No' }}</dd>
          </div>
        </div>

        <!-- Retention -->
        <div v-if="plan.retention" class="mt-6 border-t border-gray-200 pt-4">
          <h4 class="text-sm font-medium text-gray-700">Custom Retention Policy</h4>
          <div class="mt-2 grid grid-cols-3 gap-3 sm:grid-cols-6">
            <div>
              <span class="text-xs text-gray-500">Last</span>
              <p class="font-medium">{{ plan.retention.keep_last }}</p>
            </div>
            <div>
              <span class="text-xs text-gray-500">Hourly</span>
              <p class="font-medium">{{ plan.retention.keep_hourly }}</p>
            </div>
            <div>
              <span class="text-xs text-gray-500">Daily</span>
              <p class="font-medium">{{ plan.retention.keep_daily }}</p>
            </div>
            <div>
              <span class="text-xs text-gray-500">Weekly</span>
              <p class="font-medium">{{ plan.retention.keep_weekly }}</p>
            </div>
            <div>
              <span class="text-xs text-gray-500">Monthly</span>
              <p class="font-medium">{{ plan.retention.keep_monthly }}</p>
            </div>
            <div>
              <span class="text-xs text-gray-500">Yearly</span>
              <p class="font-medium">{{ plan.retention.keep_yearly }}</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Hooks section -->
      <div class="rounded-lg bg-white p-6 shadow">
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold text-gray-900">Hooks</h3>
          <button
            class="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700"
            @click="startAddHook"
          >
            Add Hook
          </button>
        </div>

        <LoadingSpinner v-if="hooksLoading" size="sm" />
        <div v-else-if="sortedHooks.length === 0" class="mt-4 text-sm text-gray-500">
          No hooks configured for this plan.
        </div>
        <div v-else class="mt-4 space-y-2">
          <div
            v-for="(hook, idx) in sortedHooks"
            :key="hook.id"
            class="flex items-center gap-3 rounded-md border border-gray-200 p-3"
          >
            <div class="flex flex-col gap-1">
              <button
                :disabled="idx === 0"
                class="text-gray-400 hover:text-gray-600 disabled:opacity-30"
                @click="moveHook(hook.id, 'up')"
              >
                &#9650;
              </button>
              <button
                :disabled="idx === sortedHooks.length - 1"
                class="text-gray-400 hover:text-gray-600 disabled:opacity-30"
                @click="moveHook(hook.id, 'down')"
              >
                &#9660;
              </button>
            </div>
            <div class="flex-1">
              <div class="flex items-center gap-2">
                <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700">
                  {{ hook.on_event }}
                </span>
                <span class="text-sm font-medium text-gray-900">
                  {{ hook.script_id ? scriptMap.get(hook.script_id) ?? 'Script' : 'Inline' }}
                </span>
              </div>
              <p v-if="hook.command" class="mt-1 truncate font-mono text-xs text-gray-500">
                {{ hook.command }}
              </p>
            </div>
            <div class="flex items-center gap-2 text-xs text-gray-500">
              <span v-if="hook.timeout">{{ hook.timeout }}s</span>
              <span v-if="hook.on_error" :class="hook.on_error === 'abort' ? 'text-red-600' : ''">
                {{ hook.on_error }}
              </span>
            </div>
            <div class="flex items-center gap-1">
              <button
                class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600"
                @click="startEditHook(hook)"
              >
                Edit
              </button>
              <button
                class="rounded p-1 text-red-400 hover:bg-red-50 hover:text-red-600"
                @click="confirmDeleteHook(hook.id)"
              >
                Delete
              </button>
            </div>
          </div>
        </div>

        <div v-if="showHookEditor" class="mt-4">
          <HookEditor
            :plan-id="planId"
            :hook="editingHook"
            :scripts="scriptsStore.list"
            @saved="onHookSaved"
            @cancel="showHookEditor = false"
          />
        </div>
      </div>

      <!-- Recent Jobs -->
      <div class="rounded-lg bg-white p-6 shadow">
        <h3 class="mb-4 text-lg font-semibold text-gray-900">Recent Jobs</h3>
        <DataTable
          :columns="jobColumns"
          :rows="(recentJobs as unknown as Record<string, unknown>[])"
          :loading="jobsStore.loading"
          empty-title="No jobs"
          empty-message="No jobs have been executed for this plan."
          @row-click="(row) => router.push(`/jobs/${row.id}`)"
        >
          <template #cell-status="{ row }">
            <StatusBadge :status="(row.status as string)" />
          </template>
          <template #cell-started_at="{ row }">
            {{ relativeTime(row.started_at as string) }}
          </template>
          <template #cell-duration="{ row }">
            {{ formatDuration(durationBetween(row.started_at as string, row.finished_at as string | null)) }}
          </template>
        </DataTable>
      </div>
    </template>

    <div v-else class="rounded-lg bg-white p-6 text-center text-gray-500 shadow">
      Plan not found.
    </div>

    <ConfirmDialog
      :open="deleteHookConfirm"
      title="Delete Hook"
      message="Remove this hook from the plan?"
      confirm-text="Delete"
      confirm-variant="danger"
      @confirm="handleDeleteHook"
      @cancel="deleteHookConfirm = false"
    />
  </div>
</template>
