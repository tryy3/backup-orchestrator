// Mirrors server data models from docs/data-models.md

export interface Agent {
  id: string
  name: string
  hostname: string
  os: string
  status: 'pending' | 'approved' | 'rejected'
  agent_version: string
  restic_version: string
  rclone_version: string
  rclone_config: string
  last_heartbeat: string | null
  last_job_at: string | null
  config_version: number
  config_applied_at: string | null
  created_at: string
  updated_at: string
}

export interface Repository {
  id: string
  name: string
  scope: 'local' | 'global'
  agent_id: string | null
  type: 'local' | 'rclone' | 'sftp' | 's3' | 'b2' | 'rest' | 'azure' | 'gs'
  path: string
  password: string
  created_at: string
  updated_at: string
}

export interface RepositoryCreate {
  name: string
  scope: 'local' | 'global'
  agent_id?: string
  type: string
  path: string
  password: string
}

export interface RetentionPolicy {
  keep_last: number
  keep_hourly: number
  keep_daily: number
  keep_weekly: number
  keep_monthly: number
  keep_yearly: number
}

export interface Script {
  id: string
  name: string
  type: 'command'
  command: string
  timeout: number
  on_error: 'abort' | 'continue'
  created_at: string
  updated_at: string
}

export interface ScriptCreate {
  name: string
  type: 'command'
  command: string
  timeout: number
  on_error: 'abort' | 'continue'
}

export interface PlanHook {
  id: string
  backup_plan_id: string
  on_event: 'pre_backup' | 'post_backup' | 'on_success' | 'on_failure' | 'pre_restore' | 'post_restore'
  sort_order: number
  script_id: string | null
  type: string | null
  command: string | null
  timeout: number | null
  on_error: string | null
  created_at: string
  updated_at: string
}

export interface PlanHookCreate {
  on_event: string
  sort_order: number
  script_id?: string
  type?: string
  command?: string
  timeout?: number
  on_error?: string
}

export interface BackupPlan {
  id: string
  name: string
  agent_id: string
  paths: string[]
  excludes: string[]
  tags: string[]
  repository_ids: string[]
  schedule: string
  forget_after_backup: boolean
  prune_after_forget: boolean
  prune_schedule: string
  retention: RetentionPolicy | null
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface BackupPlanCreate {
  name: string
  agent_id: string
  paths: string[]
  excludes?: string[]
  tags?: string[]
  repository_ids: string[]
  schedule: string
  forget_after_backup?: boolean
  prune_after_forget?: boolean
  prune_schedule?: string
  retention?: RetentionPolicy | null
  enabled?: boolean
}

export interface Job {
  id: string
  agent_id: string
  plan_id: string | null
  plan_name: string
  type: 'backup' | 'forget' | 'prune' | 'restore'
  trigger: 'scheduled' | 'manual'
  status: 'planned' | 'running' | 'success' | 'partial' | 'failed'
  started_at: string
  finished_at: string | null
  log_tail: string | null
  created_at: string
}

export interface JobRepositoryResult {
  id: string
  job_id: string
  repository_id: string
  repository_name: string
  status: 'success' | 'failed' | 'skipped'
  snapshot_id: string | null
  error: string | null
  files_new: number
  files_changed: number
  files_unmodified: number
  bytes_added: number
  total_bytes: number
  duration_ms: number
}

export interface JobHookResult {
  id: string
  job_id: string
  hook_name: string
  phase: string
  status: 'success' | 'failed' | 'skipped'
  error: string | null
  duration_ms: number
}

export interface LogEntry {
  timestamp: string
  level: 'info' | 'warn' | 'error' | 'debug'
  source: string
  message: string
  attributes?: Record<string, string>
}

export interface JobDetail extends Job {
  repository_results: JobRepositoryResult[]
  hook_results: JobHookResult[]
  log_entries: LogEntry[]
}

export interface SnapshotInfo {
  id: string
  long_id: string
  time: string
  hostname: string
  tags: string[]
  paths: string[]
}

export interface Settings {
  default_retention: RetentionPolicy
}

export interface ServerVersion {
  version: string
  commit: string
  build_date: string
}

export interface BrowseRequest {
  repository_id: string
  snapshot_id: string
  path: string
}

export interface RestoreRequest {
  repository_id: string
  snapshot_id: string
  paths: string[]
  target: string
}

// WebSocket event payloads
export interface JobCreatedEvent {
  id: string
  agent_id: string
  plan_id: string | null
  plan_name: string
  type: string
  trigger: string
  status: string
  started_at: string
  created_at: string
}

export interface JobStartedEvent {
  job_id: string
  agent_id: string
  plan_id: string
  plan_name: string
  started_at: string
  progress_percent: number
}

export interface JobProgressEvent {
  agent_id: string
  plan_name: string
  progress_percent: number
  started_at: string
}

export interface JobCompletedEvent {
  id: string
  agent_id: string
  plan_id: string | null
  plan_name: string
  type: string
  trigger: string
  status: string
  started_at: string
  finished_at: string | null
  created_at: string
}

export interface TriggerResponse {
  success: boolean
  error: string
  job_id: string
}

// Typed mapping of WebSocket event names to their payload types.
export interface WebSocketEventMap {
  'agent.connected': { agent_id: string; hostname: string }
  'agent.disconnected': { agent_id: string }
  'agent.heartbeat': { agent_id: string; timestamp: string }
  'agent.registered': Agent
  'job.created': JobCreatedEvent
  'job.started': JobStartedEvent
  'job.progress': JobProgressEvent
  'job.completed': JobCompletedEvent
}
