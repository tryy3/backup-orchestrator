<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { formatDate, formatDuration, durationBetween } from '../../utils/time'

export interface HeatmapRun {
  id: string
  status: 'success' | 'partial' | 'failed' | 'running' | 'planned' | 'aborted'
  started_at: string
  finished_at: string | null
  plan_name?: string
}

const props = defineProps<{
  runs: HeatmapRun[]
  maxRuns?: number
}>()

const router = useRouter()
const hoveredIndex = ref<number | null>(null)
const heatmapEl = ref<HTMLElement | null>(null)

const limit = computed(() => props.maxRuns ?? 30)

// Sorted oldest → newest, limited to the last N completed/failed/partial runs
const displayRuns = computed(() => {
  const completed = props.runs
    .filter((r) => r.status !== 'planned' && r.status !== 'running')
    .sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime())
    .slice(0, limit.value)
  return completed.reverse()
})

function cellColor(run: HeatmapRun): string {
  if (run.status === 'success') return 'bg-green-500'
  if (run.status === 'partial') return 'bg-amber-500'
  if (run.status === 'failed') return 'bg-red-500'
  if (run.status === 'aborted') return 'bg-slate-500'
  return 'bg-slate-600'
}

function tooltipText(run: HeatmapRun): string {
  const date = formatDate(run.started_at)
  const dur = formatDuration(durationBetween(run.started_at, run.finished_at))
  const status = run.status.charAt(0).toUpperCase() + run.status.slice(1)
  const plan = run.plan_name ? ` · ${run.plan_name}` : ''
  return `${status}${plan}\n${date}\nDuration: ${dur}`
}

// Compute tooltip position relative to the heatmap container
const tooltipStyle = computed(() => {
  if (hoveredIndex.value === null || !heatmapEl.value) return {}
  const cells = heatmapEl.value.querySelectorAll('[data-heatmap-cell]')
  const cell = cells[hoveredIndex.value] as HTMLElement | undefined
  if (!cell) return {}
  const containerRect = heatmapEl.value.getBoundingClientRect()
  const cellRect = cell.getBoundingClientRect()
  const left = cellRect.left - containerRect.left + cellRect.width / 2
  return { left: `${left}px` }
})

function navigateToJob(run: HeatmapRun) {
  router.push(`/jobs/${run.id}`)
}

// Fill empty slots so the grid always has a consistent width
const emptyCells = computed(() => Math.max(0, limit.value - displayRuns.value.length))
</script>

<template>
  <div ref="heatmapEl" class="relative">
    <div class="flex items-center gap-[3px]">
      <!-- Empty slots (no runs yet) -->
      <div
        v-for="n in emptyCells"
        :key="'empty-' + n"
        class="h-[14px] flex-1 rounded-[2px] bg-surface-700/50"
      />
      <!-- Run cells -->
      <div
        v-for="(run, i) in displayRuns"
        :key="run.id"
        data-heatmap-cell
        :class="[
          'h-[14px] flex-1 cursor-pointer rounded-[2px] transition-all',
          cellColor(run),
          hoveredIndex === i ? 'opacity-100 ring-1 ring-white/40 scale-y-125' : 'opacity-75 hover:opacity-100',
        ]"
        @mouseenter="hoveredIndex = i"
        @mouseleave="hoveredIndex = null"
        @click.stop.prevent="navigateToJob(run)"
      />
    </div>
    <!-- Tooltip -->
    <Transition name="fade">
      <div
        v-if="hoveredIndex !== null && displayRuns[hoveredIndex]"
        class="absolute bottom-full mb-2 -translate-x-1/2 whitespace-pre rounded-md border border-surface-600 bg-surface-800 px-2.5 py-1.5 text-xs text-slate-300 shadow-lg z-50 pointer-events-none"
        :style="tooltipStyle"
      >
        {{ tooltipText(displayRuns[hoveredIndex]) }}
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
