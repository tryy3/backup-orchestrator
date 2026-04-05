<script setup lang="ts">
defineProps<{
  title: string
  message?: string
  actionText?: string
  actionTo?: string
}>()
</script>

<template>
  <div class="flex flex-col items-center justify-center rounded-lg border-2 border-dashed border-gray-300 py-12 text-center">
    <svg class="mb-4 h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1">
      <path stroke-linecap="round" stroke-linejoin="round" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" />
    </svg>
    <h3 class="text-sm font-semibold text-gray-900">{{ title }}</h3>
    <p v-if="message" class="mt-1 text-sm text-gray-500">{{ message }}</p>
    <router-link
      v-if="actionText && actionTo"
      :to="actionTo"
      class="mt-4 inline-flex items-center rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
    >
      {{ actionText }}
    </router-link>
  </div>
</template>
