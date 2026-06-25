import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { fetchJobs } from '../api'
import type { Job, SavingsHistoryPoint } from '../types'
import { DateRangePicker, inDateRange, EMPTY_RANGE } from '../components/DateRangePicker'
import type { DateRange } from '../components/DateRangePicker'
import { useDebounce } from '../hooks/useDebounce'
import { formatFromUSD, useCurrencyPreference } from '../currency'
import {
  LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid,
} from 'recharts'

const PAGE_SIZES = [10, 20, 50]

type SortKey = 'latestDate' | 'medianCpu' | 'medianMem' | 'medianDuration' | 'avgCostDelta' | 'runCount'

interface JobGroup {
  jobId: string
  repository: string
  ciPlatform: string
  provider: string
  detectedMachine: string
  suggestedMachine: string
  tier: string          // most-actionable tier for display
  availableTiers: string[] // all tiers across every rec, for filtering
  runCount: number
  medianCpu: number
  medianMem: number
  medianDuration: number
  avgCostDelta: number
  avgSpotDelta: number
  spotRisk: string
  benefitLabel: string
  benefitTone: 'saving' | 'benefit' | 'neutral'
  monthlyCurrentSpend: number
  monthlySavings: number
  latestDate: string
}

function deriveBenefit(avgCostDelta: number, tier: string): { label: string; tone: 'saving' | 'benefit' | 'neutral' } {
  if (avgCostDelta < -0.5) {
    return { label: 'Cost reduction', tone: 'saving' }
  }
  if (tier === 'more-headroom' || avgCostDelta > 0.5) {
    return { label: 'Performance / stability', tone: 'benefit' }
  }
  return { label: 'Right-sized fit', tone: 'neutral' }
}

function round2(n: number): number {
  return Math.round(n * 100) / 100
}

function formatShortDate(input: string): string {
  const d = new Date(input)
  if (Number.isNaN(d.getTime())) return input.slice(0, 10)
  return d.toLocaleDateString(undefined, { month: '2-digit', day: '2-digit' })
}

function median(values: number[]): number {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 0 ? (sorted[mid - 1] + sorted[mid]) / 2 : sorted[mid]
}

function mode<T>(values: T[]): T | undefined {
  if (values.length === 0) return undefined
  const counts = new Map<string, { val: T; count: number }>()
  for (const v of values) {
    const key = String(v)
    const entry = counts.get(key)
    if (entry) entry.count++
    else counts.set(key, { val: v, count: 1 })
  }
  return [...counts.values()].sort((a, b) => b.count - a.count)[0].val
}

function groupJobs(jobs: Job[], dateRange: DateRange): JobGroup[] {
  const groups = new Map<string, Job[]>()
  for (const job of jobs) {
    const arr = groups.get(job.job_id) ?? []
    arr.push(job)
    groups.set(job.job_id, arr)
  }
  
  // Calculate period length in days (default to 90 if not specified)
  const periodDays = dateRange.from && dateRange.to
    ? (new Date(dateRange.to).getTime() - new Date(dateRange.from).getTime()) / (1000 * 60 * 60 * 24)
    : 90
  
  return Array.from(groups.entries()).flatMap(([jobId, runs]) => {
    // Keep the true latest run stable (does not change with selected period).
    const latestOverallDate = runs.map(r => r.start_time).sort().at(-1) ?? ''

    // Date filter now applies to each job's latest run date.
    if (!inDateRange(latestOverallDate, dateRange)) return []

    // Metrics/savings still represent only runs in the selected period.
    const runsInRange = runs.filter((r) => inDateRange(r.start_time, dateRange))
    if (runsInRange.length === 0) return []

    const cpu   = runsInRange.map(r => r.summary?.cpu_percent_p95 ?? 0).filter(v => v > 0)
    const mem   = runsInRange.map(r => r.summary?.mem_used_gib_p95 ?? 0).filter(v => v > 0)
    const dur   = runsInRange.map(r => r.duration_seconds ?? 0).filter(v => v > 0)
    const deltas = runsInRange.map(r => r.recommendations?.[0]?.cost_delta_percent ?? 0)
    const spotDeltas = runsInRange
      .map((r) => r.recommendations?.[0]?.spot_delta_percent)
      .filter((v): v is number => typeof v === 'number')
    const spotRisks = runsInRange.map(r => r.recommendations?.[0]?.spot_risk ?? '').filter(Boolean)
    // Provider = where the job was detected running (not what's being recommended).
    const providers = runsInRange.map(r => r.summary?.detected_machine?.provider ?? '').filter(Boolean)
    const detected  = runsInRange.map(r => r.summary?.detected_machine?.id ?? '').filter(Boolean)
    const suggested = runsInRange.map(r => r.recommendations?.[0]?.machine.id ?? '').filter(Boolean)
    // Collect every tier from every recommendation across all runs for filtering.
    const allTiers = runsInRange.flatMap(r => r.recommendations?.map(rec => rec.tier) ?? []).filter(Boolean)
    const availableTiers = [...new Set(allTiers)]
    // Display tier: show the most-actionable signal first.
    const tier = availableTiers.includes('more-headroom')  ? 'more-headroom'
               : availableTiers.includes('cheaper-option') ? 'cheaper-option'
               : availableTiers[0] ?? ''
    // CI platform: most common across runs.
    const platforms = runsInRange.map(r => r.summary?.ci_platform ?? '').filter(Boolean)
    const ciPlatform = mode(platforms) ?? ''
    // Repository: most common across runs.
    const repos = runsInRange.map(r => r.repository ?? '').filter(Boolean)
    const repository = mode(repos) ?? ''
    // Latest date remains the true latest run for this job.
    const latestDate = latestOverallDate

    const avgCostDelta = deltas.length ? deltas.reduce((a, b) => a + b, 0) / deltas.length : 0
    const avgSpotDelta = spotDeltas.length ? spotDeltas.reduce((a, b) => a + b, 0) / spotDeltas.length : 0
    const spotRisk = mode(spotRisks) ?? ''
    const benefit = deriveBenefit(avgCostDelta, tier)
    
    // Aggregate current spend and savings across all runs, weighted by actual frequency.
    // The backend calculates savings assuming 720 hours/month (24/7).
    // We scale by actual runtime: (duration_seconds * runs_per_month) / (3600 * 720)
    const runsPerMonth = runsInRange.length * 30 / Math.max(periodDays, 1)
    const monthlyCurrentSpend = runsInRange.reduce((total, run) => {
      const rec = run.recommendations?.[0]
      if (!rec || rec.current_monthly_usd <= 0) return total
      const durationHours = run.duration_seconds / 3600
      const scaleFactor = (durationHours * runsPerMonth) / 720
      return total + (rec.current_monthly_usd * scaleFactor)
    }, 0)
    const monthlySavings = runsInRange.reduce((total, run) => {
      const rec = run.recommendations?.[0]
      if (!rec || rec.cost_delta_percent >= -0.5) return total
      const durationHours = run.duration_seconds / 3600
      const scaleFactor = (durationHours * runsPerMonth) / 720 // scale to actual usage
      const adjustedSavings = Math.max(0, rec.current_monthly_usd - rec.estimated_monthly_usd) * scaleFactor
      return total + adjustedSavings
    }, 0)
    return {
      jobId,
      repository,
      ciPlatform,
      provider:         mode(providers) ?? '',
      detectedMachine:  mode(detected)  ?? '',
      suggestedMachine: mode(suggested) ?? '',
      tier,
      availableTiers,
      runCount:        runsInRange.length,
      medianCpu:       median(cpu),
      medianMem:       median(mem),
      medianDuration:  median(dur),
      avgCostDelta,
      avgSpotDelta,
      spotRisk,
      benefitLabel: benefit.label,
      benefitTone: benefit.tone,
      monthlyCurrentSpend,
      monthlySavings,
      latestDate,
    }
  })
}

function deltaClass(pct: number) {
  if (pct < -0.5) return 'delta-negative'
  if (pct > 0.5)  return 'delta-positive'
  return 'delta-neutral'
}
function formatDelta(pct: number) {
  return `${pct > 0 ? '+' : ''}${pct.toFixed(1)}%`
}

function spotRiskStyle(risk: string): React.CSSProperties {
  switch (risk) {
    case 'low':
      return { color: '#2E7D32', borderColor: '#2E7D32', background: 'rgba(46,125,50,.08)' }
    case 'high':
      return { color: '#C23B22', borderColor: '#C23B22', background: 'rgba(194,59,34,.08)' }
    default:
      return { color: '#9A7B5A', borderColor: '#9A7B5A', background: 'rgba(154,123,90,.08)' }
  }
}

function benefitStyle(tone: 'saving' | 'benefit' | 'neutral'): React.CSSProperties {
  switch (tone) {
    case 'saving':
      return { color: '#2E7D32', borderColor: '#2E7D32', background: 'rgba(46,125,50,.08)' }
    case 'benefit':
      return { color: '#1B3361', borderColor: '#1B3361', background: 'rgba(27,51,97,.08)' }
    default:
      return { color: '#9A7B5A', borderColor: '#9A7B5A', background: 'rgba(154,123,90,.08)' }
  }
}

const CI_LABELS: Record<string, string> = {
  github:    'GitHub Actions',
  gitlab:    'GitLab CI',
  circleci:  'CircleCI',
  bitbucket: 'Bitbucket',
  azure:     'Azure Pipelines',
  jenkins:   'Jenkins',
  local:     'Local',
}
function ciLabel(platform: string) {
  return CI_LABELS[platform] ?? platform
}

function SortIndicator({ col, sortKey, sortDir }: { col: SortKey; sortKey: SortKey; sortDir: 'asc' | 'desc' }) {
  if (sortKey !== col) return <span style={{ color: '#D4B896', marginLeft: 3 }}>⇅</span>
  return <span style={{ marginLeft: 3, color: '#C23B22' }}>{sortDir === 'asc' ? '↑' : '↓'}</span>
}

export default function JobsPage() {
  const { currency } = useCurrencyPreference()
  const [jobs, setJobs]       = useState<Job[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError]     = useState<string | null>(null)

  const [search,    setSearch]    = useState('')
  const debouncedSearch = useDebounce(search)
  const [repository, setRepository] = useState('')
  const [ciPlatform, setCiPlatform] = useState('')
  const [provider,  setProvider]  = useState('')
  const [tier,      setTier]      = useState('')
  const [dateRange, setDateRange] = useState<DateRange>(EMPTY_RANGE)

  const [sortKey, setSortKey] = useState<SortKey>('latestDate')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')

  const [page,     setPage]     = useState(1)
  const [pageSize, setPageSize] = useState(10)

  const navigate = useNavigate()

  useEffect(() => {
    fetchJobs()
      .then(setJobs)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  // Reset to page 1 whenever filters or page-size change
  useEffect(() => { setPage(1) }, [debouncedSearch, repository, ciPlatform, provider, tier, dateRange, pageSize])

  function toggleSort(key: SortKey) {
    if (sortKey === key) setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    else { setSortKey(key); setSortDir('desc') }
    setPage(1)
  }

  const groups = useMemo(() => groupJobs(jobs, dateRange), [jobs, dateRange])

  const filtered = useMemo(() => {
    let list = groups
    if (debouncedSearch) list = list.filter(g => g.jobId.toLowerCase().includes(debouncedSearch.toLowerCase()))
    if (repository) list = list.filter(g => g.repository === repository)
    if (ciPlatform) list = list.filter(g => g.ciPlatform === ciPlatform)
    if (provider)   list = list.filter(g => g.provider === provider)
    if (tier)       list = list.filter(g => g.availableTiers.includes(tier))
    return [...list].sort((a, b) => {
      let av = 0, bv = 0
      switch (sortKey) {
        case 'latestDate':    av = new Date(a.latestDate).getTime(); bv = new Date(b.latestDate).getTime(); break
        case 'medianCpu':     av = a.medianCpu;     bv = b.medianCpu;     break
        case 'medianMem':     av = a.medianMem;     bv = b.medianMem;     break
        case 'medianDuration':av = a.medianDuration;bv = b.medianDuration;break
        case 'avgCostDelta':  av = a.avgCostDelta;  bv = b.avgCostDelta;  break
        case 'runCount':      av = a.runCount;      bv = b.runCount;      break
      }
      return sortDir === 'asc' ? av - bv : bv - av
    })
  }, [groups, debouncedSearch, repository, ciPlatform, provider, tier, sortKey, sortDir])

  const filteredRuns = useMemo(() => {
    return jobs.filter((job) => {
      if (!inDateRange(job.start_time, dateRange)) return false
      if (debouncedSearch && !job.job_id.toLowerCase().includes(debouncedSearch.toLowerCase())) return false
      if (repository && (job.repository ?? '') !== repository) return false
      if (ciPlatform && (job.summary?.ci_platform ?? '') !== ciPlatform) return false
      if (provider && (job.summary?.detected_machine?.provider ?? '') !== provider) return false
      if (tier && !(job.recommendations ?? []).some((rec) => rec.tier === tier)) return false
      return true
    })
  }, [jobs, dateRange, debouncedSearch, repository, ciPlatform, provider, tier])

  const savingsSnapshot = useMemo(() => {
    const totalJobs = filtered.length
    const savingsJobs = filtered.filter((g) => g.monthlySavings > 0)
    const jobsWithSavings = savingsJobs.length
    const estimatedCurrentMonthlySpend = round2(filtered.reduce((sum, g) => sum + g.monthlyCurrentSpend, 0))
    const estimatedCurrentAnnualSpend = round2(estimatedCurrentMonthlySpend * 12)
    const estimatedMonthlySavings = round2(filtered.reduce((sum, g) => sum + g.monthlySavings, 0))
    const projectedAnnualSavings = round2(estimatedMonthlySavings * 12)
    const avgWastePercent = jobsWithSavings > 0
      ? savingsJobs.reduce((sum, g) => sum + Math.abs(Math.min(g.avgCostDelta, 0)), 0) / jobsWithSavings
      : 0
    return {
      totalJobs,
      jobsWithSavings,
      estimatedCurrentMonthlySpend,
      estimatedCurrentAnnualSpend,
      estimatedMonthlySavings,
      projectedAnnualSavings,
      avgWastePercent,
    }
  }, [filtered])

  const filteredSavingsHistory = useMemo<SavingsHistoryPoint[]>(() => {
    const byDay = new Map<string, { monthly: number; jobs: Set<string> }>()
    for (const run of filteredRuns) {
      const day = new Date(run.start_time).toISOString().slice(0, 10)
      const rec = run.recommendations?.[0]
      const monthly = rec && rec.cost_delta_percent < -0.5
        ? Math.max(0, rec.current_monthly_usd - rec.estimated_monthly_usd)
        : 0
      const prev = byDay.get(day) ?? { monthly: 0, jobs: new Set<string>() }
      prev.monthly += monthly
      prev.jobs.add(run.job_id)
      byDay.set(day, prev)
    }
    return [...byDay.entries()]
      .sort((a, b) => a[0].localeCompare(b[0]))
      .map(([date, data]) => ({
        date,
        job_count: data.jobs.size,
        monthly_savings: round2(data.monthly),
      }))
  }, [filteredRuns])

  // Derive unique repositories from all loaded groups for the filter dropdown.
  const allRepositories = useMemo(() =>
    [...new Set(groups.map(g => g.repository).filter(Boolean))].sort()
  , [groups])

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize))
  const paginated  = filtered.slice((page - 1) * pageSize, page * pageSize)

  if (loading) return <div className="empty">Loading jobs…</div>
  if (error)   return <div className="empty">Error: {error}</div>

  return (
    <div className="fadein">
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-7 tracking-tight">Jobs</h1>

      {/* Savings banner */}
      {filtered.length > 0 && (
        <div className="flex flex-wrap gap-6 sm:gap-8 items-center rounded-md px-5 py-4 mb-5"
          style={{ background: 'linear-gradient(90deg,#2C1A0E,#3D2510)', color: '#FBF0DC' }}>
          <div>
            <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>EST. CURRENT MONTHLY SPEND</div>
            <div className="font-deco text-3xl" style={{ color: '#FBF0DC' }}>
              {formatFromUSD(savingsSnapshot.estimatedCurrentMonthlySpend, currency)}
            </div>
          </div>
          <div>
            <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>EST. CURRENT ANNUAL SPEND</div>
            <div className="font-deco text-3xl" style={{ color: '#FBF0DC' }}>
              {formatFromUSD(savingsSnapshot.estimatedCurrentAnnualSpend, currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 })}
            </div>
          </div>
          <div>
            <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>POTENTIAL MONTHLY SAVINGS</div>
            <div className="font-deco text-3xl" style={{ color: '#E8C458' }}>
              {formatFromUSD(savingsSnapshot.estimatedMonthlySavings, currency)}
            </div>
          </div>
          <div>
            <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>POTENTIAL ANNUAL SAVINGS</div>
            <div className="font-deco text-3xl" style={{ color: '#FBF0DC' }}>
              {formatFromUSD(savingsSnapshot.projectedAnnualSavings, currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 })}
            </div>
          </div>
          <div>
            <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>OVER-PROVISIONED JOBS</div>
            <div className="font-deco text-3xl">
              {savingsSnapshot.jobsWithSavings} <span className="font-deco text-sm" style={{ color: '#C4A882' }}>of {savingsSnapshot.totalJobs}</span>
            </div>
          </div>
          <div>
            <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>AVG OVER-PROVISION</div>
            <div className="font-deco text-3xl text-red">{savingsSnapshot.avgWastePercent.toFixed(1)}%</div>
          </div>
        </div>
      )}

      {/* Savings-over-time chart */}
      {filteredSavingsHistory.length >= 2 && (
        <div className="rr-card !mb-5 !p-4">
          <div className="font-deco text-[12px] tracking-[2px] mb-3" style={{ color: '#C4A882' }}>
            SAVINGS OVER TIME (90 DAYS)
          </div>
          <ResponsiveContainer width="100%" height={160}>
            <LineChart data={filteredSavingsHistory} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#3a2510" />
              <XAxis dataKey="date" tick={{ fontSize: 10, fill: '#9A7B5A' }} tickFormatter={(d: string) => formatShortDate(d)} />
              <YAxis
                tick={{ fontSize: 10, fill: '#9A7B5A' }}
                tickFormatter={(v: number) => formatFromUSD(v, currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 })}
                width={80}
              />
              <Tooltip
                contentStyle={{ background: '#2C1A0E', border: '1px solid #3a2510', fontSize: 12 }}
                labelFormatter={(label) => formatShortDate(String(label))}
                formatter={(v) => [`${formatFromUSD(Number(v ?? 0), currency)}/mo`, 'Potential saving']}
              />
              <Line type="monotone" dataKey="monthly_savings" stroke="#E8C458" dot={false} strokeWidth={2} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}

      {/* Filter bar */}
      <div className="flex flex-wrap gap-2 mb-5 items-start sm:items-center">
        <input
          className="rr-input !w-full sm:!w-auto min-w-0 sm:min-w-[180px] flex-1 sm:flex-none sm:w-[200px]"
          placeholder="Search job name…"
          value={search}
          onChange={e => setSearch(e.target.value)}
        />
        {allRepositories.length > 0 && (
          <select className="rr-select flex-1 sm:flex-none" value={repository} onChange={e => setRepository(e.target.value)}>
            <option value="">All repositories</option>
            {allRepositories.map(r => <option key={r} value={r}>{r}</option>)}
          </select>
        )}
        <select className="rr-select flex-1 sm:flex-none" value={ciPlatform} onChange={e => setCiPlatform(e.target.value)}>
          <option value="">All CI platforms</option>
          <option value="github">GitHub Actions</option>
          <option value="gitlab">GitLab CI</option>
          <option value="circleci">CircleCI</option>
          <option value="bitbucket">Bitbucket Pipelines</option>
          <option value="azure">Azure Pipelines</option>
          <option value="jenkins">Jenkins</option>
          <option value="local">Local</option>
        </select>
        <select className="rr-select flex-1 sm:flex-none" value={provider} onChange={e => setProvider(e.target.value)}>
          <option value="">All providers</option>
          <option value="aws">AWS</option>
          <option value="gcp">GCP</option>
          <option value="github">GitHub</option>
        </select>
        <select className="rr-select flex-1 sm:flex-none" value={tier} onChange={e => setTier(e.target.value)}>
          <option value="">All tiers</option>
          <option value="right-sized">Right-sized</option>
          <option value="cheaper-option">Cheaper option</option>
          <option value="more-headroom">More headroom</option>
        </select>
        {(search || repository || ciPlatform || provider || tier) && (
          <button
            onClick={() => { setSearch(''); setRepository(''); setCiPlatform(''); setProvider(''); setTier(''); setDateRange(EMPTY_RANGE) }}
            className="border border-[var(--border)] text-[var(--text-light)] font-deco text-[13px] tracking-[1px] px-3 py-2 bg-transparent hover:border-[var(--border-dark)] hover:text-[var(--text-mid)] transition-colors cursor-pointer"
          >
            Clear
          </button>
        )}
        <span className="w-full sm:w-auto sm:ml-auto font-deco text-[13px] tracking-[1px] text-[var(--text-light)] self-center whitespace-nowrap text-right sm:text-left">
          {filtered.length} {filtered.length === 1 ? 'job' : 'jobs'}
        </span>
      </div>

      <div className="mb-5">
        <DateRangePicker value={dateRange} onChange={r => { setDateRange(r); setPage(1) }} />
      </div>

      {jobs.length === 0 ? (
        <div className="empty">
          No jobs yet. Run <code>runright monitor</code> with <code>--export http</code> to send metrics here.
        </div>
      ) : filtered.length === 0 ? (
        <div className="empty text-base">No jobs match your filters.</div>
      ) : (
        <>
          <div className="sm:hidden grid gap-3 mb-4">
            {paginated.map(g => (
              <button
                key={g.jobId}
                onClick={() => navigate(`/app/jobs/group/${encodeURIComponent(g.jobId)}`)}
                className="text-left bg-paper border border-[var(--border)] rounded-lg px-4 py-3 shadow-rr"
              >
                <div className="flex items-start justify-between gap-3 mb-2">
                  <div>
                    <div className="font-mono text-[13px] text-[var(--text)] break-all">{g.jobId}</div>
                    <div className="text-xs text-[var(--text-light)] mt-1 break-all">{g.repository || 'No repository'}</div>
                  </div>
                  <span className={`badge badge-${g.tier}`}>{g.tier}</span>
                </div>
                <div className="grid grid-cols-2 gap-2 text-xs text-[var(--text-mid)]">
                  <div><span className="text-[var(--text-light)]">Savings</span><div className={deltaClass(g.avgCostDelta)}>{formatDelta(g.avgCostDelta)}</div></div>
                  <div><span className="text-[var(--text-light)]">Benefit</span><div><span className="badge" style={benefitStyle(g.benefitTone)}>{g.benefitLabel}</span></div></div>
                  <div><span className="text-[var(--text-light)]">Runs</span><div>{g.runCount}</div></div>
                  <div><span className="text-[var(--text-light)]">CPU</span><div>{g.medianCpu.toFixed(1)}%</div></div>
                  <div><span className="text-[var(--text-light)]">Mem</span><div>{g.medianMem.toFixed(2)} GiB</div></div>
                  <div><span className="text-[var(--text-light)]">Spot</span><div>{g.spotRisk ? <span className="badge" style={spotRiskStyle(g.spotRisk)}>{g.spotRisk}</span> : '—'}</div></div>
                  <div className="col-span-2"><span className="text-[var(--text-light)]">Running On</span><div className="font-mono text-[12px] break-all">{g.detectedMachine}</div></div>
                  <div className="col-span-2"><span className="text-[var(--text-light)]">Best Fit</span><div className="font-mono text-[12px] break-all">{g.suggestedMachine}</div></div>
                </div>
              </button>
            ))}
          </div>

          <div className="rr-card !mb-2 !p-0 hidden sm:block">
            <div className="table-wrap">
              <table className="rr-table">
                <thead>
                  <tr>
                    <th>Job Name</th>
                    <th className="hidden sm:table-cell">Repository</th>
                    <th className="hidden md:table-cell">CI</th>
                    <th className="hidden md:table-cell">Provider</th>
                    <th className="hidden lg:table-cell">Running On</th>
                    <th className="hidden lg:table-cell">Best Fit</th>
                    <th>Tier</th>
                    <th className="cursor-pointer select-none" onClick={() => toggleSort('avgCostDelta')} title="Average cost delta across all runs">
                      Avg Savings <SortIndicator col="avgCostDelta" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                    <th className="hidden lg:table-cell">Benefit</th>
                    <th className="hidden lg:table-cell">Spot</th>
                    <th className="cursor-pointer select-none hidden xl:table-cell" onClick={() => toggleSort('medianCpu')} title="Median p95 CPU">
                      CPU <SortIndicator col="medianCpu" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                    <th className="cursor-pointer select-none hidden xl:table-cell" onClick={() => toggleSort('medianMem')} title="Median p95 memory">
                      Mem <SortIndicator col="medianMem" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                    <th className="cursor-pointer select-none hidden lg:table-cell" onClick={() => toggleSort('medianDuration')}>
                      Dur <SortIndicator col="medianDuration" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                    <th className="cursor-pointer select-none hidden sm:table-cell" onClick={() => toggleSort('runCount')}>
                      Runs <SortIndicator col="runCount" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                    <th className="cursor-pointer select-none" onClick={() => toggleSort('latestDate')}>
                      Last Run <SortIndicator col="latestDate" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {paginated.map(g => (
                    <tr key={g.jobId} className="cursor-pointer" onClick={() => navigate(`/app/jobs/group/${encodeURIComponent(g.jobId)}`)}>
                      <td className="td-job-name" title={g.jobId}><button className="link-btn font-mono text-[13px]">{g.jobId}</button></td>
                      <td className="hidden sm:table-cell whitespace-nowrap font-mono text-xs text-[var(--text-light)]" title={g.repository || undefined}>
                        {g.repository
                          ? <a href={`https://github.com/${g.repository}`} target="_blank" rel="noreferrer"
                              className="text-[var(--text-light)] no-underline hover:text-[var(--red)]"
                              onClick={e => e.stopPropagation()}>
                              {g.repository}
                            </a>
                          : <span className="text-[var(--border)]">—</span>}
                      </td>
                      <td className="hidden md:table-cell">{g.ciPlatform && <span className={`badge badge-ci-${g.ciPlatform}`}>{ciLabel(g.ciPlatform)}</span>}</td>
                      <td className="hidden md:table-cell">{g.provider && <span className={`badge badge-${g.provider}`}>{g.provider.toUpperCase()}</span>}</td>
                      <td className="hidden lg:table-cell font-mono text-[13px]">{g.detectedMachine}</td>
                      <td className="hidden lg:table-cell font-mono text-[13px]">{g.suggestedMachine}</td>
                      <td>{g.tier ? <span className={`badge badge-${g.tier}`}>{g.tier}</span> : null}</td>
                      <td><span className={deltaClass(g.avgCostDelta)}>{formatDelta(g.avgCostDelta)}</span></td>
                      <td className="hidden lg:table-cell"><span className="badge" style={benefitStyle(g.benefitTone)}>{g.benefitLabel}</span></td>
                      <td className="hidden lg:table-cell">
                        {g.spotRisk ? (
                          <span className="badge" style={spotRiskStyle(g.spotRisk)} title={g.avgSpotDelta ? `avg spot delta ${formatDelta(g.avgSpotDelta)}` : undefined}>
                            {g.spotRisk}
                          </span>
                        ) : (
                          <span className="text-[var(--border)]">—</span>
                        )}
                      </td>
                      <td className="hidden xl:table-cell">{g.medianCpu.toFixed(1)}%</td>
                      <td className="hidden xl:table-cell">{g.medianMem.toFixed(2)} GiB</td>
                      <td className="hidden lg:table-cell">{g.medianDuration.toFixed(0)}s</td>
                      <td className="hidden sm:table-cell">{g.runCount}</td>
                      <td>{new Date(g.latestDate).toLocaleDateString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <p className="text-xs text-[var(--text-light)] mb-4 font-sans">
            All metrics are medians across every run of that job. <strong>CPU</strong> and <strong>Memory</strong> use the p95
            value from each run, then take the median of those across runs. Click a row to see the full run history.
          </p>

          {/* Pagination */}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <span className="font-deco text-[12px] tracking-[1.5px] text-[var(--text-light)]">ROWS</span>
              {PAGE_SIZES.map(s => (
                <button key={s} onClick={() => setPageSize(s)} style={pgBtnStyle(false, pageSize === s)}>{s}</button>
              ))}
            </div>
            <div className="flex items-center gap-1.5 flex-wrap">
              <span className="font-deco text-[12px] tracking-[1px] text-[var(--text-light)] mr-2">
                {(page - 1) * pageSize + 1}–{Math.min(page * pageSize, filtered.length)} of {filtered.length}
              </span>
              <button onClick={() => setPage(1)} disabled={page === 1} style={pgBtnStyle(page === 1)}>«</button>
              <button onClick={() => setPage(p => p - 1)} disabled={page === 1} style={pgBtnStyle(page === 1)}>‹</button>
              {pageNumbers(page, totalPages).map((p, i) =>
                p === null
                  ? <span key={`e-${i}`} className="text-[var(--text-light)] px-1 font-deco">…</span>
                  : <button key={p} onClick={() => setPage(p)} style={pgBtnStyle(false, p === page)}>{p}</button>
              )}
              <button onClick={() => setPage(p => p + 1)} disabled={page === totalPages} style={pgBtnStyle(page === totalPages)}>›</button>
              <button onClick={() => setPage(totalPages)} disabled={page === totalPages} style={pgBtnStyle(page === totalPages)}>»</button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

function pgBtnStyle(disabled: boolean, active = false): React.CSSProperties {
  return {
    background: active ? '#C23B22' : 'transparent',
    color: active ? '#FBF0DC' : disabled ? '#D4B896' : '#6B4226',
    border: '1px solid',
    borderColor: active ? '#C23B22' : '#D4B896',
    padding: '4px 10px',
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 14,
    cursor: disabled ? 'default' : 'pointer',
    opacity: disabled ? 0.4 : 1,
    minWidth: 32,
    transition: 'all .1s',
    boxShadow: active ? '2px 2px 0 rgba(92,58,30,.15)' : 'none',
  }
}

function pageNumbers(current: number, total: number): (number | null)[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)
  const pages: (number | null)[] = [1]
  if (current > 3) pages.push(null)
  for (let p = Math.max(2, current - 1); p <= Math.min(total - 1, current + 1); p++) pages.push(p)
  if (current < total - 2) pages.push(null)
  pages.push(total)
  return pages
}

