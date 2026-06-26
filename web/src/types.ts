export interface MachineType {
  id: string
  provider: 'aws' | 'gcp' | 'github'
  family: string
  series: string
  vcpus: number
  memory_gib: number
  network_gbps: number
  storage_type: string
  architecture: string
  on_demand_price_per_hour: number
  tags: string[]
}

export interface MetricsSummary {
  job_id: string
  start_time: string
  end_time: string
  duration_seconds: number
  ci_platform?: string
  detected_machine?: MachineType
  detected_machine_confidence?: number
  detected_machine_confidence_level?: 'high' | 'medium' | 'low' | 'unknown'
  detected_machine_match_reason?: string
  runtime_storage_class?: 'ssd' | 'hdd' | 'unknown' | string
  cpu_percent_peak: number
  cpu_percent_avg: number
  cpu_percent_p95: number
  mem_used_gib_peak: number
  mem_used_gib_avg: number
  mem_used_gib_p95: number
  mem_total_gib: number
  process_count_peak: number
  thread_count_peak: number
  disk_read_mbs_peak: number
  disk_write_mbs_peak: number
  net_rx_mbs_peak: number
  net_tx_mbs_peak: number
  sample_count: number
}

export interface Recommendation {
  machine: MachineType
  tier: 'right-sized' | 'cheaper-option' | 'more-headroom'
  estimated_monthly_usd: number
  spot_monthly_usd?: number
  current_monthly_usd: number
  cost_delta_percent: number
  spot_delta_percent?: number
  required_vcpus: number
  required_memory_gib: number
  reasoning: string
  duration_regression_pct?: number
  duration_risk_note?: string
  spot_risk?: string
}

export interface SavingsSummary {
  total_jobs: number
  jobs_with_savings: number
  estimated_monthly_savings: number
  projected_annual_savings: number
  avg_waste_percent: number
}

export interface SavingsHistoryPoint {
  date: string
  job_count: number
  monthly_savings: number
}

export interface Job {
  id: number
  job_id: string
  repository?: string
  start_time: string
  end_time: string
  duration_seconds: number
  summary: MetricsSummary
  recommendations: Recommendation[]
  status: string
  created_at: string
}

export interface RepoSummary {
  repository: string
  job_count: number
  stale_count: number
  snoozed_count: number
  monthly_savings_usd: number
  annual_savings_usd: number
  last_seen: string
}

export interface JobSummaryRow {
  job_id: string
  repository: string
  run_count: number
  last_seen: string
  stale: boolean
  stale_days: number
  snoozed_until?: string
  snooze_reason?: string
  archived: boolean
  latest_summary: MetricsSummary
  latest_recommendations: Recommendation[]
  monthly_savings_usd: number
}

export interface PolicyRule {
  repository: string
  job_id: string
  max_cost_per_hour: number
  enabled: boolean
  updated_at: string
}

export interface PolicyEvaluation {
  repository: string
  job_id: string
  detected_price_per_hour: number
  effective_max_cost_per_hour: number
  violated: boolean
  source_scope: 'none' | 'global' | 'repository' | 'job'
  matched_policy?: PolicyRule
}

export interface NotificationEvents {
  policy_violation: boolean
  high_waste: boolean
  daily_summary: boolean
}

export interface AlertRuleConfig {
  id: string
  name: string
  type: 'event' | 'threshold'
  event?: 'policy_violation' | 'high_waste' | 'daily_summary'
  scope: 'global' | 'repository' | 'job'
  repository: string
  jobId: string
  metric: 'max_cost_per_hour' | 'waste_percent' | 'monthly_savings_drop_percent'
  threshold: number
  destinationIds: string[]
  enabled: boolean
}

export interface TeamsNotificationSettings {
  enabled: boolean
  destinations: Array<{
    id: string
    name: string
    webhook_url: string
    has_secret?: boolean
  }>
}

export interface WebhooksNotificationSettings {
  enabled: boolean
  destinations: Array<{
    id: string
    name: string
    url: string
    has_secret?: boolean
    headers?: Record<string, string>
  }>
}

export interface SlackNotificationSettings {
  enabled: boolean
  webhook_url: string
  channel: string
  mention: string
  destinations: Array<{
    id: string
    name: string
    webhook_url: string
    has_secret?: boolean
    channel: string
    mention: string
  }>
}

export interface EmailNotificationSettings {
  enabled: boolean
  recipients: string[]
  subject_prefix: string
}

export interface NotificationSettings {
  enabled: boolean
  events: NotificationEvents
  slack: SlackNotificationSettings
  teams: TeamsNotificationSettings
  webhooks: WebhooksNotificationSettings
  rules: AlertRuleConfig[]
  email: EmailNotificationSettings
}

export interface DeliveryLog {
  id: number
  rule_id: string
  destination_id: string
  channel: 'slack' | 'teams' | 'webhook'
  job_id: string
  repository: string
  status: 'delivered' | 'failed' | 'permanently_failed'
  error_message?: string
  attempts: number
  max_attempts: number
  sent_at: string
}

export interface OwnershipEntry {
  repository: string
  team_name: string
  destination_ids: string[]
  created_at: string
  updated_at: string
}
