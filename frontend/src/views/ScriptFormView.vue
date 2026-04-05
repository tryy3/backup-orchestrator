<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useScriptsStore } from '../stores/scripts'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import type { ScriptCreate } from '../types/api'

const route = useRoute()
const router = useRouter()
const store = useScriptsStore()

const isEdit = computed(() => !!route.params.id)
const scriptId = computed(() => route.params.id as string)

const form = ref<ScriptCreate>({
  name: '',
  type: 'command',
  command: '',
  timeout: 60,
  on_error: 'continue',
})

const saving = ref(false)
const formLoading = ref(false)

onMounted(async () => {
  if (isEdit.value) {
    formLoading.value = true
    await store.fetchOne(scriptId.value)
    if (store.current) {
      form.value = {
        name: store.current.name,
        type: store.current.type,
        command: store.current.command,
        timeout: store.current.timeout,
        on_error: store.current.on_error,
      }
    }
    formLoading.value = false
  }
})

async function handleSubmit() {
  saving.value = true
  let result
  if (isEdit.value) {
    result = await store.update(scriptId.value, form.value)
  } else {
    result = await store.create(form.value)
  }
  saving.value = false
  if (result) {
    router.push('/scripts')
  }
}
</script>

<template>
  <div class="mx-auto max-w-2xl">
    <LoadingSpinner v-if="formLoading" />
    <form v-else class="space-y-6 rounded border border-surface-700 bg-surface-900 p-6" @submit.prevent="handleSubmit">
      <div v-if="store.error" class="rounded border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-400">
        {{ store.error }}
      </div>

      <!-- Name -->
      <div>
        <label class="block text-sm font-medium text-slate-400">Name</label>
        <input
          v-model="form.name"
          type="text"
          required
          class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
          placeholder="e.g. healthcheck-start, notify-discord"
        />
      </div>

      <!-- Command -->
      <div>
        <label class="block text-sm font-medium text-slate-400">Command</label>
        <textarea
          v-model="form.command"
          rows="4"
          required
          class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 font-mono text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
          placeholder="curl -s https://hc-ping.com/uuid/start"
        />
      </div>

      <!-- Timeout -->
      <div>
        <label class="block text-sm font-medium text-slate-400">Timeout (seconds)</label>
        <input
          v-model.number="form.timeout"
          type="number"
          min="1"
          class="mt-1 block w-32 rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
        />
      </div>

      <!-- On Error -->
      <div>
        <label class="block text-sm font-medium text-slate-400">On Error</label>
        <select
          v-model="form.on_error"
          class="mt-1 block w-48 rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
        >
          <option value="continue">Continue</option>
          <option value="abort">Abort</option>
        </select>
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-3 border-t border-surface-700 pt-4">
        <router-link
          to="/scripts"
          class="rounded border border-surface-600 bg-surface-700 px-4 py-2 text-sm font-medium text-slate-300 transition-colors hover:bg-surface-600"
        >
          Cancel
        </router-link>
        <button
          type="submit"
          class="rounded bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 transition-colors hover:bg-accent/20 disabled:opacity-50"
          :disabled="saving"
        >
          {{ saving ? 'Saving...' : isEdit ? 'Update Script' : 'Create Script' }}
        </button>
      </div>
    </form>
  </div>
</template>
