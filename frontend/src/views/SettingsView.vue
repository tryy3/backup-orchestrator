<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useSettingsStore } from '../stores/settings'
import RetentionEditor from '../components/plans/RetentionEditor.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import type { RetentionPolicy } from '../types/api'

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

onMounted(async () => {
  await store.fetch()
  if (store.settings) {
    retention.value = { ...store.settings.default_retention }
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

    <div v-else class="rounded-lg bg-white p-6 shadow">
      <h3 class="mb-6 text-lg font-semibold text-gray-900">Default Retention Policy</h3>

      <p class="mb-4 text-sm text-gray-500">
        These defaults apply to backup plans that do not override retention settings.
      </p>

      <div v-if="store.error" class="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
        {{ store.error }}
      </div>

      <div v-if="saved" class="mb-4 rounded-md bg-green-50 p-3 text-sm text-green-700">
        Settings saved successfully.
      </div>

      <RetentionEditor v-model="retention" />

      <div class="mt-6 flex justify-end">
        <button
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          :disabled="saving"
          @click="handleSave"
        >
          {{ saving ? 'Saving...' : 'Save Settings' }}
        </button>
      </div>
    </div>
  </div>
</template>
