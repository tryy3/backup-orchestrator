<script setup lang="ts">
import { onMounted, computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useJobsStore } from '../stores/jobs'
import StatusBadge from '../components/common/StatusBadge.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import { formatDate, formatDuration, durationBetween, formatBytes } from '../utils/time'

const route = useRoute()
const jobsStore = useJobsStore()

// job-detail uses :id param; job-console uses :jobId param
const jobId = computed(() => (route.params.jobId ?? route.params.id) as string)
const job = computed(() => jobsStore.current)

// Track which log entries are expanded. Errors start expanded.
const expandedEntries = ref<Set<number>>(new Set())

function isExpanded(index: number): boolean {
  return expandedEntries.value.has(index)
}

function toggleEntry(index: number) {
  const next = new Set(expandedEntries.value)
  if (next.has(index)) {
    next.delete(index)
  } else {
    next.add(index)
  }
  expandedEntries.value = next
}

function hasAttributes(attrs?: Record<string, string>): boolean {
  return !!attrs && Object.keys(attrs).length > 0
}

// Auto-expand error entries once job loads.
function initExpandedEntries() {
  if (!job.value?.log_entries) return
  const expanded = new Set<number>()
  job.value.log_entries.forEach((entry, i) => {
    if (entry.level === 'error' && hasAttributes(entry.attributes)) {
      expanded.add(i)
    }
  })
  expandedEntries.value = expanded
}

onMounted(async () => {
  await jobsStore.fetchOne(jobId.value)
  initExpandedEntries()
})

function formatLogTimestamp(ts: string, firstTs?: string): string {
  if (!firstTs) return ts
  const start = new Date(firstTs).getTime()
  const current = new Date(ts).getTime()
  const diffMs = current - start
  const diffSec = diffMs / 1000
  if (diffSec < 0.1) return '+0.0s'
  if (diffSec < 60) return `+${diffSec.toFixed(1)}s`
  const min = Math.floor(diffSec / 60)
  const sec = Math.round(diffSec % 60)
  return `+${min}m${sec}s`
}

function levelClass(level: string): string {
  switch (level) {
    case 'error': return 'text-red-400'
    case 'warn': return 'text-amber-400'
    case 'debug': return 'text-gray-500'
    default: return 'text-green-400'
  }
}
</script>

<template>
  <div class="space-y-6">
    <LoadingSpinner v-if="jobsStore.loading && !job" />

    <template v-else-if="job">
      <!-- Job info -->
      <div class="rounded-lg border border-surface-700 bg-surface-900 p-6">
        <div class="flex items-start justify-between">
          <div>
            <h2 class="text-xl font-bold text-slate-100">{{ job.plan_name || 'Job' }}</h2>
            <p class="mt-1 text-sm text-slate-500">
              {{ job.type }} · {{ job.trigger }}
            </p>
          </div>
          <StatusBadge :status="job.status" />
        </div>

        <div class="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div>
            <dt class="text-xs font-medium text-slate-600">Started</dt>
            <dd class="mt-1 text-sm text-slate-300">{{ formatDate(job.started_at) }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-slate-600">Finished</dt>
            <dd class="mt-1 text-sm text-slate-300">{{ formatDate(job.finished_at) }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-slate-600">Duration</dt>
            <dd class="mt-1 text-sm text-slate-300">
              {{ formatDuration(durationBetween(job.started_at, job.finished_at)) }}
            </dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-slate-600">Type</dt>
            <dd class="mt-1 text-sm capitalize text-slate-300">{{ job.type }}</dd>
          </div>
        </div>
      </div>

      <!-- Repository Results -->
      <div v-if="job.repository_results?.length" class="rounded-lg border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-sm font-semibold uppercase tracking-wider text-slate-500">Repository Results</h3>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-surface-700">
            <thead class="bg-surface-800">
              <tr>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Repository</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Status</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Snapshot</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">New</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Changed</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Unmodified</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Bytes Added</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Duration</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-surface-700">
              <tr v-for="r in job.repository_results" :key="r.repository_id">
                <td class="whitespace-nowrap px-4 py-3 text-sm font-medium text-slate-200">{{ r.repository_name }}</td>
                <td class="whitespace-nowrap px-4 py-3 text-sm">
                  <StatusBadge :status="r.status" />
                </td>
                <td class="whitespace-nowrap px-4 py-3 font-mono text-xs text-slate-400">{{ r.snapshot_id || '—' }}</td>
                <td class="whitespace-nowrap px-4 py-3 text-sm text-slate-300">{{ r.files_new }}</td>
                <td class="whitespace-nowrap px-4 py-3 text-sm text-slate-300">{{ r.files_changed }}</td>
                <td class="whitespace-nowrap px-4 py-3 text-sm text-slate-300">{{ r.files_unmodified }}</td>
                <td class="whitespace-nowrap px-4 py-3 text-sm text-slate-300">{{ formatBytes(r.bytes_added) }}</td>
                <td class="whitespace-nowrap px-4 py-3 text-sm text-slate-300">{{ formatDuration(r.duration_ms) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Hook Results -->
      <div v-if="job.hook_results?.length" class="rounded-lg border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-sm font-semibold uppercase tracking-wider text-slate-500">Hook Results</h3>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-surface-700">
            <thead class="bg-surface-800">
              <tr>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Hook</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Phase</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Status</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Error</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-slate-500">Duration</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-surface-700">
              <tr v-for="(h, i) in job.hook_results" :key="i">
                <td class="whitespace-nowrap px-4 py-3 text-sm font-medium text-slate-200">{{ h.hook_name }}</td>
                <td class="whitespace-nowrap px-4 py-3 text-sm capitalize text-slate-400">{{ h.phase }}</td>
                <td class="whitespace-nowrap px-4 py-3 text-sm">
                  <StatusBadge :status="h.status" />
                </td>
                <td class="max-w-xs truncate px-4 py-3 text-sm text-red-400">{{ h.error || '—' }}</td>
                <td class="whitespace-nowrap px-4 py-3 text-sm text-slate-300">{{ formatDuration(h.duration_ms) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Structured Log Timeline -->
      <div v-if="job.log_entries?.length" class="rounded-lg border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-sm font-semibold uppercase tracking-wider text-slate-500">Job Log</h3>
        <div class="max-h-[32rem] overflow-auto rounded-lg bg-surface-950 p-4">
          <div
            v-for="(entry, i) in job.log_entries"
            :key="i"
            class="border-b border-surface-700 last:border-0"
          >
            <!-- Main log line -->
            <div
              class="flex gap-3 py-1.5 font-mono text-sm leading-relaxed"
              :class="{ 'cursor-pointer hover:bg-surface-800/50 rounded': hasAttributes(entry.attributes) }"
              @click="hasAttributes(entry.attributes) && toggleEntry(i)"
            >
              <span class="shrink-0 text-gray-500">{{ formatLogTimestamp(entry.timestamp, job.log_entries[0]?.timestamp) }}</span>
              <span
                :class="['shrink-0 w-12 text-center text-xs font-bold uppercase', levelClass(entry.level)]"
              >{{ entry.level }}</span>
              <span class="shrink-0 w-28 text-cyan-400">{{ entry.source }}</span>
              <span class="flex-1 text-gray-100">{{ entry.message }}</span>
              <span
                v-if="hasAttributes(entry.attributes)"
                class="shrink-0 text-gray-500 text-xs leading-relaxed select-none"
              >{{ isExpanded(i) ? '&#9660;' : '&#9654;' }}</span>
            </div>

            <!-- Expanded attributes -->
            <div
              v-if="isExpanded(i) && hasAttributes(entry.attributes)"
              class="ml-[13.5rem] mb-2 rounded bg-surface-800 p-3 font-mono text-xs"
            >
              <div
                v-for="(value, key) in entry.attributes"
                :key="key"
                class="flex gap-2 py-0.5"
              >
                <span class="shrink-0 text-slate-400">{{ key }}:</span>
                <span class="whitespace-pre-wrap break-all text-slate-200">{{ value }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Fallback: plain log_tail for old jobs -->
      <div v-else-if="job.log_tail" class="rounded-lg border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-4 text-sm font-semibold uppercase tracking-wider text-slate-500">Log Output</h3>
        <pre class="max-h-96 overflow-auto rounded-lg bg-surface-950 p-4 font-mono text-sm text-slate-100">{{ job.log_tail }}</pre>
      </div>
    </template>

    <div v-else class="rounded-lg border border-surface-700 bg-surface-900 p-6 text-center text-slate-500">
      Job not found.
    </div>
  </div>
</template>
