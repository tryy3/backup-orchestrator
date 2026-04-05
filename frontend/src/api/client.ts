import type {
  Agent,
  Repository,
  RepositoryCreate,
  Script,
  ScriptCreate,
  BackupPlan,
  BackupPlanCreate,
  PlanHook,
  PlanHookCreate,
  Job,
  JobDetail,
  SnapshotInfo,
  Settings,
  BrowseRequest,
  RestoreRequest,
} from '../types/api'

const BASE_URL = import.meta.env.VITE_API_URL ?? '/api'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`${res.status}: ${body}`)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

// Agents
export const agents = {
  list: () => request<Agent[]>('/agents'),
  get: (id: string) => request<Agent>(`/agents/${id}`),
  approve: (id: string) => request<Agent>(`/agents/${id}/approve`, { method: 'POST' }),
  reject: (id: string) => request<Agent>(`/agents/${id}/reject`, { method: 'POST' }),
  remove: (id: string) => request<void>(`/agents/${id}`, { method: 'DELETE' }),
  updateRclone: (id: string, config: string) =>
    request<void>(`/agents/${id}/rclone`, {
      method: 'PUT',
      body: JSON.stringify({ rclone_config: config }),
    }),
}

// Repositories
export const repositories = {
  list: (params?: { scope?: string; agent_id?: string }) => {
    const query = new URLSearchParams()
    if (params?.scope) query.set('scope', params.scope)
    if (params?.agent_id) query.set('agent_id', params.agent_id)
    const qs = query.toString()
    return request<Repository[]>(`/repositories${qs ? `?${qs}` : ''}`)
  },
  get: (id: string) => request<Repository>(`/repositories/${id}`),
  create: (data: RepositoryCreate) =>
    request<Repository>('/repositories', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Partial<RepositoryCreate>) =>
    request<Repository>(`/repositories/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  remove: (id: string) => request<void>(`/repositories/${id}`, { method: 'DELETE' }),
}

// Scripts
export const scripts = {
  list: () => request<Script[]>('/scripts'),
  get: (id: string) => request<Script>(`/scripts/${id}`),
  create: (data: ScriptCreate) =>
    request<Script>('/scripts', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Partial<ScriptCreate>) =>
    request<Script>(`/scripts/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  remove: (id: string) => request<void>(`/scripts/${id}`, { method: 'DELETE' }),
}

// Backup Plans
export const plans = {
  list: (params?: { agent_id?: string }) => {
    const query = new URLSearchParams()
    if (params?.agent_id) query.set('agent_id', params.agent_id)
    const qs = query.toString()
    return request<BackupPlan[]>(`/plans${qs ? `?${qs}` : ''}`)
  },
  get: (id: string) => request<BackupPlan>(`/plans/${id}`),
  create: (data: BackupPlanCreate) =>
    request<BackupPlan>('/plans', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Partial<BackupPlanCreate>) =>
    request<BackupPlan>(`/plans/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  remove: (id: string) => request<void>(`/plans/${id}`, { method: 'DELETE' }),
  trigger: (id: string) => request<void>(`/plans/${id}/trigger`, { method: 'POST' }),
}

// Plan Hooks
export const hooks = {
  list: (planId: string) => request<PlanHook[]>(`/plans/${planId}/hooks`),
  create: (planId: string, data: PlanHookCreate) =>
    request<PlanHook>(`/plans/${planId}/hooks`, { method: 'POST', body: JSON.stringify(data) }),
  update: (planId: string, hookId: string, data: Partial<PlanHookCreate>) =>
    request<PlanHook>(`/plans/${planId}/hooks/${hookId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  remove: (planId: string, hookId: string) =>
    request<void>(`/plans/${planId}/hooks/${hookId}`, { method: 'DELETE' }),
  reorder: (planId: string, hookIds: string[]) =>
    request<void>(`/plans/${planId}/hooks/reorder`, {
      method: 'PUT',
      body: JSON.stringify({ hook_ids: hookIds }),
    }),
}

// Jobs
export const jobs = {
  list: (params?: { agent_id?: string; plan_id?: string; status?: string }) => {
    const query = new URLSearchParams()
    if (params?.agent_id) query.set('agent_id', params.agent_id)
    if (params?.plan_id) query.set('plan_id', params.plan_id)
    if (params?.status) query.set('status', params.status)
    const qs = query.toString()
    return request<Job[]>(`/jobs${qs ? `?${qs}` : ''}`)
  },
  get: (id: string) => request<JobDetail>(`/jobs/${id}`),
}

// Snapshots
export const snapshots = {
  list: (agentId: string, repoId: string) =>
    request<SnapshotInfo[]>(`/agents/${agentId}/snapshots?repo=${repoId}`),
  browse: (agentId: string, data: BrowseRequest) =>
    request<unknown>(`/agents/${agentId}/snapshots/browse`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  restore: (agentId: string, data: RestoreRequest) =>
    request<void>(`/agents/${agentId}/restore`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
}

// Settings
export const settings = {
  get: () => request<Settings>('/settings'),
  update: (data: Settings) =>
    request<Settings>('/settings', { method: 'PUT', body: JSON.stringify(data) }),
}
