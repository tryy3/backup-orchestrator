<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { hooks as hooksApi } from '../../api/client'
import type { PlanHook, PlanHookCreate, Script } from '../../types/api'

const props = defineProps<{
  planId: string
  hook?: PlanHook
  scripts: Script[]
}>()

const emit = defineEmits<{
  saved: []
  cancel: []
}>()

const eventOptions = ['pre_backup', 'post_backup', 'on_success', 'on_failure']

const onEvent = ref(props.hook?.on_event ?? 'pre_backup')
const sourceType = ref<'script' | 'inline'>(props.hook?.script_id ? 'script' : 'inline')
const scriptId = ref(props.hook?.script_id ?? '')
const command = ref(props.hook?.command ?? '')
const timeout = ref<number | undefined>(props.hook?.timeout ?? undefined)
const onError = ref<string | undefined>(props.hook?.on_error ?? undefined)
const saving = ref(false)
const error = ref<string | null>(null)

const isEdit = computed(() => !!props.hook)

onMounted(() => {
  // Already initialized from props defaults above
})

async function handleSubmit() {
  saving.value = true
  error.value = null

  const data: PlanHookCreate = {
    on_event: onEvent.value,
    sort_order: props.hook?.sort_order ?? 999,
  }

  if (sourceType.value === 'script') {
    data.script_id = scriptId.value
  } else {
    data.type = 'command'
    data.command = command.value
  }

  if (timeout.value != null) data.timeout = timeout.value
  if (onError.value) data.on_error = onError.value

  try {
    if (isEdit.value && props.hook) {
      await hooksApi.update(props.planId, props.hook.id, data)
    } else {
      await hooksApi.create(props.planId, data)
    }
    emit('saved')
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <form class="space-y-4 rounded-lg border border-gray-200 bg-gray-50 p-4" @submit.prevent="handleSubmit">
    <h4 class="text-sm font-semibold text-gray-900">{{ isEdit ? 'Edit Hook' : 'Add Hook' }}</h4>

    <div v-if="error" class="rounded-md bg-red-50 p-2 text-sm text-red-700">{{ error }}</div>

    <!-- Event -->
    <div>
      <label class="block text-sm font-medium text-gray-700">Event</label>
      <select
        v-model="onEvent"
        class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
      >
        <option v-for="ev in eventOptions" :key="ev" :value="ev">{{ ev }}</option>
      </select>
    </div>

    <!-- Source type -->
    <div>
      <label class="block text-sm font-medium text-gray-700">Source</label>
      <div class="mt-2 flex gap-4">
        <label class="flex items-center gap-2">
          <input v-model="sourceType" type="radio" value="script" class="text-blue-600" />
          <span class="text-sm text-gray-700">Use script</span>
        </label>
        <label class="flex items-center gap-2">
          <input v-model="sourceType" type="radio" value="inline" class="text-blue-600" />
          <span class="text-sm text-gray-700">Inline command</span>
        </label>
      </div>
    </div>

    <!-- Script selector -->
    <div v-if="sourceType === 'script'">
      <label class="block text-sm font-medium text-gray-700">Script</label>
      <select
        v-model="scriptId"
        required
        class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
      >
        <option value="" disabled>Select a script</option>
        <option v-for="s in scripts" :key="s.id" :value="s.id">{{ s.name }}</option>
      </select>
    </div>

    <!-- Inline command -->
    <div v-if="sourceType === 'inline'">
      <label class="block text-sm font-medium text-gray-700">Command</label>
      <textarea
        v-model="command"
        rows="2"
        required
        class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 font-mono text-sm focus:border-blue-500 focus:ring-blue-500"
        placeholder="pg_dumpall -U postgres > /tmp/dump.sql"
      />
    </div>

    <!-- Overrides -->
    <div class="grid grid-cols-2 gap-4">
      <div>
        <label class="block text-sm font-medium text-gray-700">Timeout override (seconds)</label>
        <input
          v-model.number="timeout"
          type="number"
          min="1"
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
          placeholder="Default"
        />
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700">On error override</label>
        <select
          v-model="onError"
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-blue-500"
        >
          <option :value="undefined">Default</option>
          <option value="continue">Continue</option>
          <option value="abort">Abort</option>
        </select>
      </div>
    </div>

    <!-- Buttons -->
    <div class="flex justify-end gap-2">
      <button
        type="button"
        class="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50"
        @click="$emit('cancel')"
      >
        Cancel
      </button>
      <button
        type="submit"
        class="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
        :disabled="saving"
      >
        {{ saving ? 'Saving...' : isEdit ? 'Update' : 'Add' }}
      </button>
    </div>
  </form>
</template>
