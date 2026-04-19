<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useSettingsStore } from '../stores/settings'
import RetentionEditor from '../components/plans/RetentionEditor.vue'
import LoadingSpinner from '../components/common/LoadingSpinner.vue'
import type { RetentionPolicy, ServerVersion } from '../types/api'
import { SETTINGS_DEFAULTS } from '../types/api'
import * as api from '../api/client'

const store = useSettingsStore()

const retention = ref<RetentionPolicy>({
  keep_last: 5,
  keep_hourly: 0,
  keep_daily: 7,
  keep_weekly: 4,
  keep_monthly: 6,
  keep_yearly: 0,
})

// Global settings form refs
const heartbeatInterval = ref<number>(SETTINGS_DEFAULTS.heartbeat_interval_seconds)
const offlineThreshold = ref<number>(SETTINGS_DEFAULTS.agent_offline_threshold_seconds)
const jobHistoryDays = ref<number>(SETTINGS_DEFAULTS.job_history_days)
const healthThresholdFailing = ref<number>(SETTINGS_DEFAULTS.health_threshold_failing * 100)
const healthThresholdWarning = ref<number>(SETTINGS_DEFAULTS.health_threshold_warning * 100)
const maxHeatmapRuns = ref<number>(SETTINGS_DEFAULTS.max_heatmap_runs)
const defaultHookTimeout = ref<number>(SETTINGS_DEFAULTS.default_hook_timeout_seconds)
const blockedPaths = ref<string>(SETTINGS_DEFAULTS.file_browser_blocked_paths.join('\n'))
const cmdTimeoutBackup = ref<number>(SETTINGS_DEFAULTS.command_timeout_backup_seconds)
const cmdTimeoutRestore = ref<number>(SETTINGS_DEFAULTS.command_timeout_restore_seconds)
const cmdTimeoutListSnapshots = ref<number>(SETTINGS_DEFAULTS.command_timeout_list_snapshots_seconds)
const cmdTimeoutBrowseSnapshot = ref<number>(SETTINGS_DEFAULTS.command_timeout_browse_snapshot_seconds)
const cmdTimeoutBrowseFs = ref<number>(SETTINGS_DEFAULTS.command_timeout_browse_filesystem_seconds)
const cmdTimeoutDefault = ref<number>(SETTINGS_DEFAULTS.command_timeout_default_seconds)

// Outbox tunables (seconds, except spill_max_rows / max_attempts which are counts).
const outboxSpillMaxRows = ref<number>(SETTINGS_DEFAULTS.outbox_spill_max_rows)
const outboxSpillRetentionSeconds = ref<number>(SETTINGS_DEFAULTS.outbox_spill_retention_seconds)
const outboxFlushIntervalSeconds = ref<number>(SETTINGS_DEFAULTS.outbox_flush_interval_seconds)
const outboxDeliveryTimeoutSeconds = ref<number>(SETTINGS_DEFAULTS.outbox_delivery_timeout_seconds)
const outboxMaxAttempts = ref<number>(SETTINGS_DEFAULTS.outbox_max_attempts)

const saving = ref(false)
const saved = ref(false)

const serverVersion = ref<ServerVersion | null>(null)
const appVersion = import.meta.env.VITE_APP_VERSION || 'dev'

onMounted(async () => {
  await store.fetch()
  if (store.settings) {
    if (store.settings.default_retention) {
      retention.value = { ...store.settings.default_retention }
    }
    heartbeatInterval.value = store.settings.heartbeat_interval_seconds ?? SETTINGS_DEFAULTS.heartbeat_interval_seconds
    offlineThreshold.value = store.settings.agent_offline_threshold_seconds ?? SETTINGS_DEFAULTS.agent_offline_threshold_seconds
    jobHistoryDays.value = store.settings.job_history_days ?? SETTINGS_DEFAULTS.job_history_days
    healthThresholdFailing.value = (store.settings.health_threshold_failing ?? SETTINGS_DEFAULTS.health_threshold_failing) * 100
    healthThresholdWarning.value = (store.settings.health_threshold_warning ?? SETTINGS_DEFAULTS.health_threshold_warning) * 100
    maxHeatmapRuns.value = store.settings.max_heatmap_runs ?? SETTINGS_DEFAULTS.max_heatmap_runs
    defaultHookTimeout.value = store.settings.default_hook_timeout_seconds ?? SETTINGS_DEFAULTS.default_hook_timeout_seconds
    const bp = store.settings.file_browser_blocked_paths ?? SETTINGS_DEFAULTS.file_browser_blocked_paths
    blockedPaths.value = bp.join('\n')
    cmdTimeoutBackup.value = store.settings.command_timeout_backup_seconds ?? SETTINGS_DEFAULTS.command_timeout_backup_seconds
    cmdTimeoutRestore.value = store.settings.command_timeout_restore_seconds ?? SETTINGS_DEFAULTS.command_timeout_restore_seconds
    cmdTimeoutListSnapshots.value = store.settings.command_timeout_list_snapshots_seconds ?? SETTINGS_DEFAULTS.command_timeout_list_snapshots_seconds
    cmdTimeoutBrowseSnapshot.value = store.settings.command_timeout_browse_snapshot_seconds ?? SETTINGS_DEFAULTS.command_timeout_browse_snapshot_seconds
    cmdTimeoutBrowseFs.value = store.settings.command_timeout_browse_filesystem_seconds ?? SETTINGS_DEFAULTS.command_timeout_browse_filesystem_seconds
    cmdTimeoutDefault.value = store.settings.command_timeout_default_seconds ?? SETTINGS_DEFAULTS.command_timeout_default_seconds
    outboxSpillMaxRows.value = store.settings.outbox_spill_max_rows ?? SETTINGS_DEFAULTS.outbox_spill_max_rows
    outboxSpillRetentionSeconds.value = store.settings.outbox_spill_retention_seconds ?? SETTINGS_DEFAULTS.outbox_spill_retention_seconds
    outboxFlushIntervalSeconds.value = store.settings.outbox_flush_interval_seconds ?? SETTINGS_DEFAULTS.outbox_flush_interval_seconds
    outboxDeliveryTimeoutSeconds.value = store.settings.outbox_delivery_timeout_seconds ?? SETTINGS_DEFAULTS.outbox_delivery_timeout_seconds
    outboxMaxAttempts.value = store.settings.outbox_max_attempts ?? SETTINGS_DEFAULTS.outbox_max_attempts
  }
  try {
    serverVersion.value = await api.version.get()
  } catch {
    // non-fatal: version info is best-effort
  }
})

async function handleSave() {
  saving.value = true
  saved.value = false
  const paths = blockedPaths.value
    .split('\n')
    .map((p) => p.trim())
    .filter((p) => p.length > 0)
  const ok = await store.update({
    default_retention: retention.value,
    heartbeat_interval_seconds: heartbeatInterval.value,
    agent_offline_threshold_seconds: offlineThreshold.value,
    job_history_days: jobHistoryDays.value,
    health_threshold_failing: healthThresholdFailing.value / 100,
    health_threshold_warning: healthThresholdWarning.value / 100,
    max_heatmap_runs: maxHeatmapRuns.value,
    default_hook_timeout_seconds: defaultHookTimeout.value,
    file_browser_blocked_paths: paths,
    command_timeout_backup_seconds: cmdTimeoutBackup.value,
    command_timeout_restore_seconds: cmdTimeoutRestore.value,
    command_timeout_list_snapshots_seconds: cmdTimeoutListSnapshots.value,
    command_timeout_browse_snapshot_seconds: cmdTimeoutBrowseSnapshot.value,
    command_timeout_browse_filesystem_seconds: cmdTimeoutBrowseFs.value,
    command_timeout_default_seconds: cmdTimeoutDefault.value,
    outbox_spill_max_rows: outboxSpillMaxRows.value,
    outbox_spill_retention_seconds: outboxSpillRetentionSeconds.value,
    outbox_flush_interval_seconds: outboxFlushIntervalSeconds.value,
    outbox_delivery_timeout_seconds: outboxDeliveryTimeoutSeconds.value,
    outbox_max_attempts: outboxMaxAttempts.value,
  })
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

    <template v-else>
      <!-- Status messages -->
      <div v-if="store.error" class="rounded border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-400">
        {{ store.error }}
      </div>
      <div v-if="saved" class="rounded border border-green-500/20 bg-green-500/10 p-3 text-sm text-green-400">
        Settings saved successfully.
      </div>

      <!-- Global Settings section -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-6 text-lg font-semibold text-slate-100">Global Settings</h3>

        <p class="mb-4 text-sm text-slate-400">
          These settings control dashboard-wide behavior and agent defaults. Changes are pushed to all connected agents.
        </p>

        <div class="space-y-5">
          <!-- Heartbeat frequency -->
          <div>
            <label class="block text-sm font-medium text-slate-300">Heartbeat Interval</label>
            <p class="mt-0.5 text-xs text-slate-500">How often agents send heartbeats to the server.</p>
            <div class="mt-1 flex items-center gap-2">
              <input
                v-model.number="heartbeatInterval"
                type="number"
                min="5"
                max="600"
                class="w-24 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
              />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>

          <!-- Offline threshold -->
          <div>
            <label class="block text-sm font-medium text-slate-300">Agent Offline Threshold</label>
            <p class="mt-0.5 text-xs text-slate-500">Time since last heartbeat before an agent is shown as offline.</p>
            <div class="mt-1 flex items-center gap-2">
              <input
                v-model.number="offlineThreshold"
                type="number"
                min="30"
                max="3600"
                class="w-24 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
              />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>

          <!-- Job history window -->
          <div>
            <label class="block text-sm font-medium text-slate-300">Job History Window</label>
            <p class="mt-0.5 text-xs text-slate-500">Lookback window for agent health and success-rate calculations.</p>
            <div class="mt-1 flex items-center gap-2">
              <input
                v-model.number="jobHistoryDays"
                type="number"
                min="1"
                max="365"
                class="w-24 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
              />
              <span class="text-sm text-slate-500">days</span>
            </div>
          </div>

          <!-- Health thresholds -->
          <div>
            <label class="block text-sm font-medium text-slate-300">Agent Health Thresholds</label>
            <p class="mt-0.5 text-xs text-slate-500">Success-rate thresholds for agent health badge colors.</p>
            <div class="mt-1 grid grid-cols-2 gap-4">
              <div>
                <label class="block text-xs text-slate-500">Failing below</label>
                <div class="mt-1 flex items-center gap-2">
                  <input
                    v-model.number="healthThresholdFailing"
                    type="number"
                    min="0"
                    max="100"
                    step="1"
                    class="w-20 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
                  />
                  <span class="text-sm text-slate-500">%</span>
                </div>
              </div>
              <div>
                <label class="block text-xs text-slate-500">Warning below</label>
                <div class="mt-1 flex items-center gap-2">
                  <input
                    v-model.number="healthThresholdWarning"
                    type="number"
                    min="0"
                    max="100"
                    step="1"
                    class="w-20 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
                  />
                  <span class="text-sm text-slate-500">%</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Max heatmap runs -->
          <div>
            <label class="block text-sm font-medium text-slate-300">Max Heatmap Runs</label>
            <p class="mt-0.5 text-xs text-slate-500">Number of recent runs displayed in the run heatmap.</p>
            <div class="mt-1 flex items-center gap-2">
              <input
                v-model.number="maxHeatmapRuns"
                type="number"
                min="5"
                max="200"
                class="w-24 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
              />
              <span class="text-sm text-slate-500">runs</span>
            </div>
          </div>

          <!-- Default hook timeout -->
          <div>
            <label class="block text-sm font-medium text-slate-300">Default Hook Timeout</label>
            <p class="mt-0.5 text-xs text-slate-500">Timeout applied to pre/post hooks when none is explicitly set.</p>
            <div class="mt-1 flex items-center gap-2">
              <input
                v-model.number="defaultHookTimeout"
                type="number"
                min="5"
                max="3600"
                class="w-24 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
              />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>

          <!-- File browser blocked paths -->
          <div>
            <label class="block text-sm font-medium text-slate-300">File Browser Blocked Paths</label>
            <p class="mt-0.5 text-xs text-slate-500">Paths the file browser refuses to list (one per line).</p>
            <textarea
              v-model="blockedPaths"
              rows="5"
              class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 font-mono text-xs text-slate-300 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
              placeholder="/proc&#10;/sys&#10;/dev"
            />
          </div>
        </div>
      </div>

      <!-- Per-command Timeouts section -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-2 text-lg font-semibold text-slate-100">Agent Command Timeouts</h3>
        <p class="mb-4 text-sm text-slate-400">
          Maximum time the agent will wait for each gRPC command before cancelling it.
          Long-running commands like backup and restore should get generous values; lookup commands can be tighter.
          These act as global defaults — they can be overridden per agent on the agent detail page.
        </p>

        <div class="grid grid-cols-2 gap-5">
          <div>
            <label class="block text-sm font-medium text-slate-300">Backup</label>
            <p class="mt-0.5 text-xs text-slate-500">restic backup runs.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="cmdTimeoutBackup" type="number" min="60" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-slate-300">Restore</label>
            <p class="mt-0.5 text-xs text-slate-500">restic restore runs.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="cmdTimeoutRestore" type="number" min="60" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-slate-300">List Snapshots</label>
            <p class="mt-0.5 text-xs text-slate-500">restic snapshots lookups.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="cmdTimeoutListSnapshots" type="number" min="5" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-slate-300">Browse Snapshot</label>
            <p class="mt-0.5 text-xs text-slate-500">restic ls inside a snapshot.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="cmdTimeoutBrowseSnapshot" type="number" min="5" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-slate-300">Browse Filesystem</label>
            <p class="mt-0.5 text-xs text-slate-500">Local filesystem listings on the agent.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="cmdTimeoutBrowseFs" type="number" min="1" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-slate-300">Default (other)</label>
            <p class="mt-0.5 text-xs text-slate-500">Fallback for unknown command kinds.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="cmdTimeoutDefault" type="number" min="5" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Outbox tunables section -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-2 text-lg font-semibold text-slate-100">Agent Outbox</h3>
        <p class="mb-4 text-sm text-slate-400">
          Controls the agent's in-memory + SQLite delivery queue for job reports
          (and future job events). When the server is reachable, items are
          delivered immediately; when it is not, they spill to SQLite and are
          retried with backoff. These act as global defaults — they can be
          overridden per agent on the agent detail page.
        </p>
        <p class="mb-4 text-xs text-slate-500">
          Note: the in-memory channel capacity (<code class="font-mono">OUTBOX_MEMORY_MAX</code>)
          is an agent-side bootstrap env var because Go channels cannot be resized at runtime.
        </p>

        <div class="grid grid-cols-2 gap-5">
          <div>
            <label class="block text-sm font-medium text-slate-300">Spill Row Cap</label>
            <p class="mt-0.5 text-xs text-slate-500">Max rows in the SQLite spill table; oldest dropped above this.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="outboxSpillMaxRows" type="number" min="100" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">rows</span>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-slate-300">Spill Retention</label>
            <p class="mt-0.5 text-xs text-slate-500">TTL for spill rows; older entries are pruned daily.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="outboxSpillRetentionSeconds" type="number" min="60" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-slate-300">Flush Interval</label>
            <p class="mt-0.5 text-xs text-slate-500">How often the outbox tries to drain pending items.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="outboxFlushIntervalSeconds" type="number" min="1" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-slate-300">Delivery Timeout</label>
            <p class="mt-0.5 text-xs text-slate-500">Per-RPC timeout for delivering one outbox item.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="outboxDeliveryTimeoutSeconds" type="number" min="1" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">seconds</span>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-slate-300">Max Attempts</label>
            <p class="mt-0.5 text-xs text-slate-500">Drop a payload after this many failed sends.</p>
            <div class="mt-1 flex items-center gap-2">
              <input v-model.number="outboxMaxAttempts" type="number" min="1" class="w-28 rounded border border-surface-600 bg-surface-950 px-3 py-1.5 text-sm text-slate-300 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent" />
              <span class="text-sm text-slate-500">attempts</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Retention Policy section -->
      <div class="rounded border border-surface-700 bg-surface-900 p-6">
        <h3 class="mb-6 text-lg font-semibold text-slate-100">Default Retention Policy</h3>

        <p class="mb-4 text-sm text-slate-400">
          These defaults apply to backup plans that do not override retention settings.
        </p>

        <RetentionEditor v-model="retention" />
      </div>

      <!-- Save button -->
      <div class="flex justify-end">
        <button
          class="rounded bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 transition-colors hover:bg-accent/20 disabled:opacity-50"
          :disabled="saving"
          @click="handleSave"
        >
          {{ saving ? 'Saving...' : 'Save Settings' }}
        </button>
      </div>
    </template>

    <!-- Version info -->
    <div class="rounded border border-surface-700 bg-surface-900 p-6">
      <h3 class="mb-4 text-lg font-semibold text-slate-100">About</h3>
      <dl class="space-y-2 text-sm">
        <div class="flex justify-between">
          <dt class="text-slate-400">Frontend</dt>
          <dd class="font-mono text-slate-300">{{ appVersion }}</dd>
        </div>
        <template v-if="serverVersion">
          <div class="flex justify-between">
            <dt class="text-slate-400">Server</dt>
            <dd class="font-mono text-slate-300">{{ serverVersion.version }}</dd>
          </div>
          <div class="flex justify-between">
            <dt class="text-slate-400">Commit</dt>
            <dd class="font-mono text-slate-300">{{ serverVersion.commit }}</dd>
          </div>
          <div class="flex justify-between">
            <dt class="text-slate-400">Build Date</dt>
            <dd class="font-mono text-slate-300">{{ serverVersion.build_date }}</dd>
          </div>
        </template>
      </dl>
    </div>
  </div>
</template>
