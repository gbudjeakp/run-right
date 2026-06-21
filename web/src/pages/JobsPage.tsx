import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { fetchJobs } from '../api'
import type { Job } from '../types'
import { DateRangePicker, inDateRange, EMPTY_RANGE } from '../components/DateRangePicker'
import type { DateRange } from '../components/DateRangePicker'
import { useDebounce } from '../hooks/useDebounce'

const PAGE_SIZES = [10, 20, 50]

type SortKey = 'latestDate' | 'medianCpu' | 'medianMem' | 'medianDuration' | 'avgCostDelta' | 'runCount'

interface JobGroup {
  jobId: string
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
  latestDate: string
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
    // Filter by start_time (when the job actually ran) so period presets
    // are meaningful even when all jobs were inserted into the DB today.
    if (!inDateRange(job.start_time, dateRange)) continue
    const arr = groups.get(job.job_id) ?? []
    arr.push(job)
    groups.set(job.job_id, arr)
  }
  return Array.from(groups.entries()).map(([jobId, runs]) => {
    const cpu   = runs.map(r => r.summary?.cpu_percent_p95 ?? 0).filter(v => v > 0)
    const mem   = runs.map(r => r.summary?.mem_used_gib_p95 ?? 0).filter(v => v > 0)
    const dur   = runs.map(r => r.duration_seconds ?? 0).filter(v => v > 0)
    const deltas = runs.map(r => r.recommendations?.[0]?.cost_delta_percent ?? 0)
    // Provider = where the job was detected running (not what's being recommended).
    const providers = runs.map(r => r.summary?.detected_machine?.provider ?? '').filter(Boolean)
    const detected  = runs.map(r => r.summary?.detected_machine?.id ?? '').filter(Boolean)
    const suggested = runs.map(r => r.recommendations?.[0]?.machine.id ?? '').filter(Boolean)
    // Collect every tier from every recommendation across all runs for filtering.
    const allTiers = runs.flatMap(r => r.recommendations?.map(rec => rec.tier) ?? []).filter(Boolean)
    const availableTiers = [...new Set(allTiers)]
    // Display tier: show the most-actionable signal first.
    const tier = availableTiers.includes('more-headroom')  ? 'more-headroom'
               : availableTiers.includes('cheaper-option') ? 'cheaper-option'
               : availableTiers[0] ?? ''
    // CI platform: most common across runs.
    const platforms = runs.map(r => r.summary?.ci_platform ?? '').filter(Boolean)
    const ciPlatform = mode(platforms) ?? ''
    // Latest date by when the job actually ran, not when it was DB-inserted.
    const latestDate = runs.map(r => r.start_time).sort().at(-1) ?? ''
    return {
      jobId,
      ciPlatform,
      provider:         mode(providers) ?? '',
      detectedMachine:  mode(detected)  ?? '',
      suggestedMachine: mode(suggested) ?? '',
      tier,
      availableTiers,
      runCount:        runs.length,
      medianCpu:       median(cpu),
      medianMem:       median(mem),
      medianDuration:  median(dur),
      avgCostDelta:    deltas.length ? deltas.reduce((a, b) => a + b, 0) / deltas.length : 0,
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

function SortIndicator({ col, sortKey, sortDir }: { col: SortKey; sortKey: SortKey; sortDir: 'asc' | 'desc' }) {
  if (sortKey !== col) return <span style={{ color: '#D4B896', marginLeft: 3 }}>⇅</span>
  return <span style={{ marginLeft: 3, color: '#C23B22' }}>{sortDir === 'asc' ? '↑' : '↓'}</span>
}

export default function JobsPage() {
  const [jobs, setJobs]       = useState<Job[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError]     = useState<string | null>(null)

  const [search,    setSearch]    = useState('')
  const debouncedSearch = useDebounce(search)
  const [ciPlatform, setCiPlatform] = useState('')
  const [provider,  setProvider]  = useState('')
  const [tier,      setTier]      = useState('')
  const [dateRange, setDateRange] = useState<DateRange>(EMPTY_RANGE)

  const [sortKey, setSortKey] = useState<SortKey>('latestDate')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')

  const [page,     setPage]     = useState(1)
  const [pageSize, setPageSize] = useState(20)

  const navigate = useNavigate()

  useEffect(() => {
    fetchJobs()
      .then(setJobs)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  // Reset to page 1 whenever filters or page-size change
  useEffect(() => { setPage(1) }, [debouncedSearch, ciPlatform, provider, tier, dateRange, pageSize])

  function toggleSort(key: SortKey) {
    if (sortKey === key) setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    else { setSortKey(key); setSortDir('desc') }
    setPage(1)
  }

  const groups = useMemo(() => groupJobs(jobs, dateRange), [jobs, dateRange])

  const filtered = useMemo(() => {
    let list = groups
    if (debouncedSearch) list = list.filter(g => g.jobId.toLowerCase().includes(debouncedSearch.toLowerCase()))
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
  }, [groups, debouncedSearch, ciPlatform, provider, tier, sortKey, sortDir])

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize))
  const paginated  = filtered.slice((page - 1) * pageSize, page * pageSize)

  if (loading) return <div className="empty">Loading jobs…</div>
  if (error)   return <div className="empty">Error: {error}</div>

  return (
    <div className="fadein">
      <h1>Jobs</h1>

      <div className="filter-bar">
        <input
          placeholder="Search job name…"
          value={search}
          onChange={e => setSearch(e.target.value)}
          style={{ minWidth: 200 }}
        />
        <select value={ciPlatform} onChange={e => setCiPlatform(e.target.value)}>
          <option value="">All CI platforms</option>
          <option value="github">GitHub Actions</option>
          <option value="jenkins">Jenkins</option>
          <option value="local">Local</option>
        </select>
        <select value={provider} onChange={e => setProvider(e.target.value)}>
          <option value="">All providers</option>
          <option value="aws">AWS</option>
          <option value="gcp">GCP</option>
          <option value="github">GitHub</option>
        </select>
        <select value={tier} onChange={e => setTier(e.target.value)}>
          <option value="">All tiers</option>
          <option value="right-sized">Right-sized</option>
          <option value="cheaper-option">Cheaper option</option>
          <option value="more-headroom">More headroom</option>
        </select>
        {(search || ciPlatform || provider || tier) && (
          <button
            onClick={() => { setSearch(''); setCiPlatform(''); setProvider(''); setTier(''); setDateRange(EMPTY_RANGE) }}
            style={{
              background: 'none', border: '1px solid #D4B896', color: '#9A7B5A',
              padding: '7px 14px', fontFamily: "'Bebas Neue', Impact, sans-serif",
              fontSize: 13, letterSpacing: 1, cursor: 'pointer',
            }}
          >
            Clear
          </button>
        )}
        <span style={{ marginLeft: 'auto', fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 13, letterSpacing: 1, color: '#9A7B5A', alignSelf: 'center' }}>
          {filtered.length} {filtered.length === 1 ? 'job' : 'jobs'}
        </span>
      </div>
      <div style={{ marginBottom: 20 }}>
        <DateRangePicker value={dateRange} onChange={r => { setDateRange(r); setPage(1) }} />
      </div>

      {jobs.length === 0 ? (
        <div className="empty">
          No jobs yet. Run <code>runright monitor</code> with <code>--export http</code> to send metrics here.
        </div>
      ) : filtered.length === 0 ? (
        <div className="empty" style={{ fontSize: 15 }}>No jobs match your filters.</div>
      ) : (
        <>
          <div className="card" style={{ marginBottom: 8 }}>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Job Name</th>
                    <th>CI</th>
                    <th>Provider</th>
                    <th>Running On</th>
                    <th>Best Fit</th>
                    <th>Tier</th>
                    <th style={{ cursor: 'pointer', userSelect: 'none' }} onClick={() => toggleSort('avgCostDelta')} title="Average cost delta across all runs of this job">
                      Avg Savings <SortIndicator col="avgCostDelta" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                    <th style={{ cursor: 'pointer', userSelect: 'none' }} onClick={() => toggleSort('medianCpu')} title="Median of each run's p95 CPU — the level sustained through 95% of a typical run">
                      CPU Usage <SortIndicator col="medianCpu" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                    <th style={{ cursor: 'pointer', userSelect: 'none' }} onClick={() => toggleSort('medianMem')} title="Median of each run's p95 memory">
                      Memory <SortIndicator col="medianMem" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                    <th style={{ cursor: 'pointer', userSelect: 'none' }} onClick={() => toggleSort('medianDuration')}>
                      Duration <SortIndicator col="medianDuration" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                    <th style={{ cursor: 'pointer', userSelect: 'none' }} onClick={() => toggleSort('runCount')}>
                      Runs <SortIndicator col="runCount" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                    <th style={{ cursor: 'pointer', userSelect: 'none' }} onClick={() => toggleSort('latestDate')}>
                      Last Run <SortIndicator col="latestDate" sortKey={sortKey} sortDir={sortDir} />
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {paginated.map(g => (
                    <tr key={g.jobId} style={{ cursor: 'pointer' }} onClick={() => navigate(`/app/jobs/group/${encodeURIComponent(g.jobId)}`)}>
                      <td className="td-job-name" title={g.jobId}><button className="link-btn">{g.jobId}</button></td>
                      <td>{g.ciPlatform && <span className={`badge badge-ci-${g.ciPlatform}`}>{g.ciPlatform === 'github' ? 'GitHub Actions' : g.ciPlatform === 'jenkins' ? 'Jenkins' : g.ciPlatform}</span>}</td>
                      <td>{g.provider && <span className={`badge badge-${g.provider}`}>{g.provider.toUpperCase()}</span>}</td>
                      <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{g.detectedMachine}</td>
                      <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{g.suggestedMachine}</td>
                      <td>{g.tier ? <span className={`badge badge-${g.tier}`}>{g.tier}</span> : null}</td>
                      <td><span className={deltaClass(g.avgCostDelta)}>{formatDelta(g.avgCostDelta)}</span></td>
                      <td>{g.medianCpu.toFixed(1)}%</td>
                      <td>{g.medianMem.toFixed(2)} GiB</td>
                      <td>{g.medianDuration.toFixed(0)}s</td>
                      <td>{g.runCount}</td>
                      <td>{new Date(g.latestDate).toLocaleDateString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <p style={{ fontSize: 12, color: '#9A7B5A', marginBottom: 16, fontFamily: 'Lato, sans-serif' }}>
            All metrics are medians across every run of that job. <strong>CPU Usage</strong> and <strong>Memory</strong> use the p95
            value from each run — the level sustained through 95% of samples — then take the median of those across runs.
            Click a row to see the full run history.
          </p>

          {/* Pagination */}
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 12, letterSpacing: 1.5, color: '#9A7B5A' }}>ROWS</span>
              {PAGE_SIZES.map(s => (
                <button key={s} onClick={() => setPageSize(s)} style={{
                  background: pageSize === s ? '#2C1A0E' : 'transparent',
                  color: pageSize === s ? '#FBF0DC' : '#9A7B5A',
                  border: '1px solid',
                  borderColor: pageSize === s ? '#2C1A0E' : '#D4B896',
                  padding: '4px 10px',
                  fontFamily: "'Bebas Neue', Impact, sans-serif",
                  fontSize: 13, letterSpacing: 1, cursor: 'pointer', transition: 'all .1s',
                }}>{s}</button>
              ))}
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <span style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 12, letterSpacing: 1, color: '#9A7B5A', marginRight: 8 }}>
                {(page - 1) * pageSize + 1}–{Math.min(page * pageSize, filtered.length)} of {filtered.length}
              </span>
              <button onClick={() => setPage(1)} disabled={page === 1} style={pgBtnStyle(page === 1)}>«</button>
              <button onClick={() => setPage(p => p - 1)} disabled={page === 1} style={pgBtnStyle(page === 1)}>‹</button>
              {pageNumbers(page, totalPages).map((p, i) =>
                p === null
                  ? <span key={`e-${i}`} style={{ color: '#9A7B5A', padding: '0 4px', fontFamily: "'Bebas Neue'" }}>…</span>
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

