import axios from 'axios'
import type {
  Job, MachineType, SavingsSummary, SavingsHistoryPoint, RepoSummary, JobSummaryRow,
  PolicyRule, PolicyEvaluation, NotificationSettings, DeliveryLog, OwnershipEntry,
  SSOProvider, SSOUser, SSOConfig,
} from './types'

const api = axios.create({
  baseURL: '/api/v1',
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true, // send the HttpOnly session cookie automatically
})

// If the backend session cookie is missing/expired (common after backend restarts),
// reset client auth state and force a clean login.
api.interceptors.response.use(
  (response) => response,
  (error) => {
    const status = error?.response?.status
    const reqURL = String(error?.config?.url ?? '')
    const isAuthEndpoint = reqURL.includes('/auth')

    if (status === 401 && !isAuthEndpoint && typeof window !== 'undefined' && !import.meta.env.DEV) {
      localStorage.setItem('rr-auth', 'false')
      if (window.location.pathname !== '/login') {
        window.location.replace('/login')
      }
    }

    return Promise.reject(error)
  },
)

export const login = (apiKey: string): Promise<void> =>
  api.post('/auth', { api_key: apiKey }).then(() => undefined)

export const logout = (): Promise<void> =>
  api.post('/auth/logout').then(() => undefined)

export const fetchJobs = (repository?: string): Promise<Job[]> =>
  api.get<Job[]>('/jobs', { params: repository ? { repository } : {} }).then((r) => r.data ?? [])

export const fetchJob = (id: number): Promise<Job> =>
  api.get<Job>(`/jobs/${id}`).then((r) => r.data)

export const fetchCatalog = (provider?: string): Promise<MachineType[]> =>
  api.get<MachineType[]>('/catalog', { params: provider ? { provider } : {} }).then((r) => r.data ?? [])

export const fetchSavings = (repository?: string): Promise<SavingsSummary> =>
  api.get<SavingsSummary>('/savings', { params: repository ? { repository } : {} }).then((r) => r.data)

export const fetchSavingsHistory = (): Promise<SavingsHistoryPoint[]> =>
  api.get<SavingsHistoryPoint[]>('/savings/history').then((r) => r.data ?? [])

export const fetchJobTrend = (jobId: string, window = 10): Promise<unknown> =>
  api.get(`/jobs/${encodeURIComponent(jobId)}/trend`, { params: { window } }).then((r) => r.data)

// ── Repo-centric API ─────────────────────────────────────────────────────────

export const fetchRepos = (): Promise<RepoSummary[]> =>
  api.get<RepoSummary[]>('/repos').then((r) => r.data ?? [])

export const fetchRepoJobs = (repository: string, includeArchived = false): Promise<JobSummaryRow[]> =>
  api
    .get<JobSummaryRow[]>('/repo-jobs', {
      params: { repository, ...(includeArchived ? { include_archived: 'true' } : {}) },
    })
    .then((r) => r.data ?? [])

export const fetchIsolatedJobs = (includeArchived = false): Promise<JobSummaryRow[]> =>
  api
    .get<JobSummaryRow[]>('/isolated-jobs', {
      params: includeArchived ? { include_archived: 'true' } : {},
    })
    .then((r) => r.data ?? [])

export const upsertJobMeta = (payload: {
  job_id: string
  repository: string
  snoozed_until?: string | null
  snooze_reason?: string
  archived?: boolean
  stale_days?: number
}): Promise<void> => api.put('/job-meta', payload).then(() => undefined)

export const deleteJobRuns = (jobId: string, repository: string): Promise<{ deleted_runs: number }> =>
  api
    .delete<{ deleted_runs: number }>('/job-runs', { params: { job_id: jobId, repository } })
    .then((r) => r.data)

export const fetchPolicies = (repository?: string): Promise<PolicyRule[]> =>
  api.get<PolicyRule[]>('/policies', { params: repository ? { repository } : {} }).then((r) => r.data ?? [])

export const upsertPolicy = (payload: {
  repository: string
  job_id?: string
  max_cost_per_hour: number
  enabled?: boolean
}): Promise<void> => api.put('/policies', payload).then(() => undefined)

export const deletePolicy = (repository: string, jobId = ''): Promise<void> =>
  api.delete('/policies', { params: { repository, job_id: jobId } }).then(() => undefined)

export const evaluatePolicy = (payload: {
  repository: string
  job_id: string
  detected_price_per_hour: number
}): Promise<PolicyEvaluation> => api.post<PolicyEvaluation>('/policies/evaluate', payload).then((r) => r.data)

export const fetchNotificationSettings = (): Promise<NotificationSettings> =>
  api.get<NotificationSettings>('/notifications/settings').then((r) => r.data)

export const upsertNotificationSettings = (payload: NotificationSettings): Promise<void> =>
  api.put('/notifications/settings', payload).then(() => undefined)

export const sendTestNotification = (): Promise<void> =>
  api.post('/notifications/test').then(() => undefined)

export const fetchDeliveryLogs = (ruleId?: string, limit = 50): Promise<DeliveryLog[]> =>
  api
    .get<DeliveryLog[]>('/notifications/deliveries', {
      params: { ...(ruleId ? { rule_id: ruleId } : {}), limit },
    })
    .then((r) => r.data ?? [])

export const fetchOwnership = (repository?: string): Promise<OwnershipEntry[]> =>
  api.get<OwnershipEntry[]>('/ownership', { params: repository ? { repository } : {} }).then((r) => r.data ?? [])

export const upsertOwnership = (payload: { repository: string; team_name: string; destination_ids: string[] }): Promise<void> =>
  api.put('/ownership', payload).then(() => undefined)

export const deleteOwnership = (repository: string, teamName: string): Promise<void> =>
  api.delete('/ownership', { params: { repository, team_name: teamName } }).then(() => undefined)

export interface UserSettings {
  otel_endpoint: string
  allowed_machine_ids: string[]
  allowed_series: string[]
  allowed_families: string[]
}

export const fetchUserSettings = (): Promise<UserSettings> =>
  api.get<UserSettings>('/user-settings').then((r) => r.data)

export const upsertUserSettings = (payload: UserSettings): Promise<void> =>
  api.put('/user-settings', payload).then(() => undefined)

// ── SSO API ─────────────────────────────────────────────────────────────────

export const fetchSSOProviders = (): Promise<SSOProvider[]> =>
  api.get<{ providers: SSOProvider[] }>('/sso/providers').then((r) => r.data?.providers ?? [])

export const fetchSSOMe = (): Promise<SSOUser> =>
  api.get<SSOUser>('/sso/me').then((r) => r.data)

export const ssoLogout = (): Promise<void> =>
  api.post('/sso/logout').then(() => undefined)

// Admin SSO config management
export const fetchSSOConfigs = (): Promise<SSOConfig[]> =>
  api.get<{ configs: SSOConfig[] }>('/sso/configs').then((r) => r.data?.configs ?? [])

export const upsertSSOConfig = (payload: Partial<SSOConfig>): Promise<{ id: number }> =>
  api.put<{ id: number; status: string }>('/sso/configs', payload).then((r) => ({ id: r.data.id }))

export const deleteSSOConfig = (id: number): Promise<void> =>
  api.delete('/sso/configs', { data: { id } }).then(() => undefined)

export const testSSOConfig = (payload: Partial<SSOConfig>): Promise<{ valid: boolean; message?: string; error?: string }> =>
  api.post<{ valid: boolean; message?: string; error?: string }>('/sso/configs/test', payload).then((r) => r.data)
