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
    <form v-else class="space-y-6 rounded-lg bg-white p-6 shadow" @submit.prevent="handleSubmit">
      <div v-if="store.error" class="rounded-md bg-red-50 p-3 text-sm text-red-700">
        {{ store.error }}
      </div>

      <!-- Name -->
      <div>
        <label class="block text-sm font-medium text-gray-700">Name</label>
        <input
          v-model="form.name"
          type="text"
          required
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
          placeholder="e.g. healthcheck-start, notify-discord"
        />
      </div>

      <!-- Command -->
      <div>
        <label class="block text-sm font-medium text-gray-700">Command</label>
        <textarea
          v-model="form.command"
          rows="4"
          required
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 font-mono text-sm focus:border-blue-500 focus:ring-blue-500"
          placeholder="curl -s https://hc-ping.com/uuid/start"
        />
      </div>

      <!-- Timeout -->
      <div>
        <label class="block text-sm font-medium text-gray-700">Timeout (seconds)</label>
        <input
          v-model.number="form.timeout"
          type="number"
          min="1"
          class="mt-1 block w-32 rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
        />
      </div>

      <!-- On Error -->
      <div>
        <label class="block text-sm font-medium text-gray-700">On Error</label>
        <select
          v-model="form.on_error"
          class="mt-1 block w-48 rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
        >
          <option value="continue">Continue</option>
          <option value="abort">Abort</option>
        </select>
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-3 border-t border-gray-200 pt-4">
        <router-link
          to="/scripts"
          class="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
        >
          Cancel
        </router-link>
        <button
          type="submit"
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          :disabled="saving"
        >
          {{ saving ? 'Saving...' : isEdit ? 'Update Script' : 'Create Script' }}
        </button>
      </div>
    </form>
  </div>
</template>
