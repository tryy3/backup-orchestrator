<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
}>()

const colorClasses = computed(() => {
  const s = props.status.toLowerCase()
  if (['success', 'approved', 'active'].includes(s)) {
    return 'bg-green-100 text-green-800'
  }
  if (['failed', 'rejected'].includes(s)) {
    return 'bg-red-100 text-red-800'
  }
  if (['partial', 'degraded'].includes(s)) {
    return 'bg-amber-100 text-amber-800'
  }
  if (s === 'pending') {
    return 'bg-blue-100 text-blue-800'
  }
  if (s === 'running') {
    return 'bg-blue-100 text-blue-800 animate-pulse'
  }
  if (s === 'offline') {
    return 'bg-gray-100 text-gray-800'
  }
  return 'bg-gray-100 text-gray-600'
})
</script>

<template>
  <span
    :class="[
      'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium capitalize',
      colorClasses,
    ]"
  >
    {{ status }}
  </span>
</template>
