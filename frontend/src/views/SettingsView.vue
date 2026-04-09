<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useSettingsStore } from '../stores/settings'
import RetentionEditor from '../components/plans/RetentionEditor.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import type { RetentionPolicy, ServerVersion } from '../types/api'
import * as api from '../api/client'

const store = useSettingsStore()

const retention = ref<RetentionPolicy>({
  keep_last: 5,
  keep_hourly: 0,
  keep_daily: 7,
  keep_weekly: 4,
  keep_monthly: 6,
  keep_yearly: 0,
})

const saving = ref(false)
const saved = ref(false)

const serverVersion = ref<ServerVersion | null>(null)
const appVersion = import.meta.env.VITE_APP_VERSION || 'dev'

onMounted(async () => {
  await store.fetch()
  if (store.settings) {
    retention.value = { ...store.settings.default_retention }
  }
  try {
    serverVersion.value = await api.version.get()
  } catch {
    // non-fatal: version info is best-effort
  }
})

async function handleSave() {
  saving.value = true
  saved.value = false
  const ok = await store.update({ default_retention: retention.value })
  saving.value = false
  if (ok) {
    saved.value = true
    setTimeout(() => { saved.value = false }, 3000)
  }
}
</script>

<template>
  <div class="mx-auto max-w-2xl space-y-6">
    <LoadingSpinner v-if="store.loading" />

    <div v-else class="rounded border border-surface-700 bg-surface-900 p-6">
      <h3 class="mb-6 text-lg font-semibold text-slate-100">Default Retention Policy</h3>

      <p class="mb-4 text-sm text-slate-400">
        These defaults apply to backup plans that do not override retention settings.
      </p>

      <div v-if="store.error" class="mb-4 rounded border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-400">
        {{ store.error }}
      </div>

      <div v-if="saved" class="mb-4 rounded border border-green-500/20 bg-green-500/10 p-3 text-sm text-green-400">
        Settings saved successfully.
      </div>

      <RetentionEditor v-model="retention" />

      <div class="mt-6 flex justify-end">
        <button
          class="rounded bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 transition-colors hover:bg-accent/20 disabled:opacity-50"
          :disabled="saving"
          @click="handleSave"
        >
          {{ saving ? 'Saving...' : 'Save Settings' }}
        </button>
      </div>
    </div>

    <!-- Version info -->
    <div class="rounded border border-surface-700 bg-surface-900 p-6">
      <h3 class="mb-4 text-lg font-semibold text-slate-100">About</h3>
      <dl class="space-y-2 text-sm">
        <div class="flex justify-between">
          <dt class="text-slate-400">Frontend</dt>
          <dd class="font-mono text-slate-300">{{ appVersion }}</dd>
        </div>
        <template v-if="serverVersion">
          <div class="flex justify-between">
            <dt class="text-slate-400">Server</dt>
            <dd class="font-mono text-slate-300">{{ serverVersion.version }}</dd>
          </div>
          <div class="flex justify-between">
            <dt class="text-slate-400">Commit</dt>
            <dd class="font-mono text-slate-300">{{ serverVersion.commit }}</dd>
          </div>
          <div class="flex justify-between">
            <dt class="text-slate-400">Build Date</dt>
            <dd class="font-mono text-slate-300">{{ serverVersion.build_date }}</dd>
          </div>
        </template>
      </dl>
    </div>
  </div>
</template>
