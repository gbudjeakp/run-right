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
}

export interface SavingsSummary {
  total_jobs: number
  jobs_with_savings: number
  estimated_monthly_savings: number
  projected_annual_savings: number
  avg_waste_percent: number
}

export interface Job {
  id: number
  job_id: string
  start_time: string
  end_time: string
  duration_seconds: number
  summary: MetricsSummary
  recommendations: Recommendation[]
  status: string
  created_at: string
}
