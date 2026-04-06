<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
}>()

const colorClasses = computed(() => {
  const s = props.status.toLowerCase()
  if (['success', 'approved', 'active'].includes(s))
    return 'bg-green-500/15 text-green-400 ring-1 ring-green-500/30'
  if (['failed', 'rejected'].includes(s))
    return 'bg-red-500/15 text-red-400 ring-1 ring-red-500/30'
  if (['partial', 'degraded'].includes(s))
    return 'bg-amber-500/15 text-amber-400 ring-1 ring-amber-500/30'
  if (s === 'pending' || s === 'planned')
    return 'bg-slate-500/15 text-slate-300 ring-1 ring-slate-500/30'
  if (s === 'running')
    return 'bg-cyan-500/15 text-cyan-400 ring-1 ring-cyan-500/30 animate-pulse'
  if (s === 'offline')
    return 'bg-slate-700/50 text-slate-500 ring-1 ring-slate-700'
  return 'bg-slate-700/30 text-slate-400'
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
