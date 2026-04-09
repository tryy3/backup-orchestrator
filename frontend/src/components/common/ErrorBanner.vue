<script setup lang="ts">
defineProps<{
  message: string | null
}>()

const emit = defineEmits<{
  dismiss: []
}>()
</script>

<template>
  <div
    v-if="message"
    class="flex items-start gap-3 rounded-lg border border-red-500/20 bg-red-500/5 px-4 py-3"
  >
    <svg class="mt-0.5 h-4 w-4 shrink-0 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
      <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
    </svg>
    <span class="text-sm text-red-300">{{ message }}</span>
    <button
      class="ml-auto shrink-0 text-red-400 hover:text-red-300"
      @click="emit('dismiss')"
    >
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
      </svg>
    </button>
  </div>
</template>
