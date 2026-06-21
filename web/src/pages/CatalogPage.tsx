import { useEffect, useState, useMemo } from 'react'
import { fetchCatalog } from '../api'
import type { MachineType } from '../types'
import { useDebounce } from '../hooks/useDebounce'

export default function CatalogPage() {
  const [machines, setMachines] = useState<MachineType[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search)
  const [provider, setProvider] = useState('')
  const [arch, setArch] = useState('')
  const [sortKey, setSortKey] = useState<keyof MachineType>('on_demand_price_per_hour')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')
  const [page,     setPage]     = useState(1)
  const [pageSize, setPageSize] = useState(10)

  useEffect(() => {
    fetchCatalog(provider || undefined)
      .then(setMachines)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [provider])

  const filtered = useMemo(() => {
    let list = machines
    if (debouncedSearch) {
      const q = debouncedSearch.toLowerCase()
      list = list.filter((m) => m.id.toLowerCase().includes(q) || m.family.toLowerCase().includes(q) || m.series.toLowerCase().includes(q))
    }
    if (arch) list = list.filter((m) => m.architecture === arch)
    return [...list].sort((a, b) => {
      const av = a[sortKey] as number | string
      const bv = b[sortKey] as number | string
      if (av < bv) return sortDir === 'asc' ? -1 : 1
      if (av > bv) return sortDir === 'asc' ? 1 : -1
      return 0
    })
  }, [machines, debouncedSearch, arch, sortKey, sortDir])

  useEffect(() => { setPage(1) }, [filtered, pageSize])

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize))
  const paginated  = filtered.slice((page - 1) * pageSize, page * pageSize)

  function toggleSort(key: keyof MachineType) {
    if (sortKey === key) setSortDir((d) => d === 'asc' ? 'desc' : 'asc')
    else { setSortKey(key); setSortDir('asc') }
  }

  function SortIndicator({ col }: { col: keyof MachineType }) {
    if (sortKey !== col) return null
    return <span style={{ marginLeft: 4 }}>{sortDir === 'asc' ? '↑' : '↓'}</span>
  }

  if (loading) return <div className="empty">Loading catalog…</div>
  if (error) return <div className="empty">Error: {error}</div>

  return (
    <div className="fadein">
      <h1>Machine Catalog</h1>
      <div className="filter-bar">
        <input
          placeholder="Search by ID, family, series…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ minWidth: 240 }}
        />
        <select value={provider} onChange={(e) => { setProvider(e.target.value); setLoading(true) }}>
          <option value="">All providers</option>
          <option value="aws">AWS</option>
          <option value="gcp">GCP</option>
          <option value="github">GitHub</option>
        </select>
        <select value={arch} onChange={(e) => setArch(e.target.value)}>
          <option value="">All architectures</option>
          <option value="x86_64">x86_64</option>
          <option value="arm64">arm64</option>
        </select>
        <span style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 14, letterSpacing: 2, color: '#9A7B5A', alignSelf: 'center' }}>
          {filtered.length} machines
        </span>
      </div>
      <div className="card">
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th style={{ cursor: 'pointer' }} onClick={() => toggleSort('id')}>ID <SortIndicator col="id" /></th>
                <th>Provider</th>
                <th>Family</th>
                <th style={{ cursor: 'pointer' }} onClick={() => toggleSort('vcpus')}>vCPUs <SortIndicator col="vcpus" /></th>
                <th style={{ cursor: 'pointer' }} onClick={() => toggleSort('memory_gib')}>Memory <SortIndicator col="memory_gib" /></th>
                <th>Network</th>
                <th>Arch</th>
                <th style={{ cursor: 'pointer' }} onClick={() => toggleSort('on_demand_price_per_hour')}>$/hr <SortIndicator col="on_demand_price_per_hour" /></th>
                <th>$/month</th>
                <th>Tags</th>
              </tr>
            </thead>
            <tbody>
              {paginated.map((m) => (
                <tr key={`${m.provider}-${m.id}`}>
                  <td><code>{m.id}</code></td>
                  <td><span className={`badge badge-${m.provider}`}>{m.provider.toUpperCase()}</span></td>
                  <td>{m.family}</td>
                  <td>{m.vcpus}</td>
                  <td>{m.memory_gib} GiB</td>
                  <td>{m.network_gbps > 0 ? `${m.network_gbps} Gbps` : null}</td>
                  <td>{m.architecture}</td>
                  <td>${m.on_demand_price_per_hour.toFixed(4)}</td>
                  <td>${(m.on_demand_price_per_hour * 720).toFixed(2)}</td>
                  <td style={{ fontSize: 11, color: '#718096' }}>{m.tags?.join(', ')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {totalPages > 1 && (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12, marginTop: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 12, letterSpacing: 1.5, color: '#9A7B5A' }}>ROWS</span>
            {CAT_PAGE_SIZES.map(s => (
              <button key={s} onClick={() => setPageSize(s)} style={{
                background:  pageSize === s ? '#2C1A0E' : 'transparent',
                color:       pageSize === s ? '#FBF0DC' : '#9A7B5A',
                border:      '1px solid',
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
            <button onClick={() => setPage(1)} disabled={page === 1} style={catPgBtn(page === 1)}>«</button>
            <button onClick={() => setPage(p => p - 1)} disabled={page === 1} style={catPgBtn(page === 1)}>‹</button>
            {catPageNums(page, totalPages).map((p, i) =>
              p === null
                ? <span key={`e-${i}`} style={{ color: '#9A7B5A', padding: '0 4px', fontFamily: "'Bebas Neue'" }}>…</span>
                : <button key={p} onClick={() => setPage(p)} style={catPgBtn(false, p === page)}>{p}</button>
            )}
            <button onClick={() => setPage(p => p + 1)} disabled={page === totalPages} style={catPgBtn(page === totalPages)}>›</button>
            <button onClick={() => setPage(totalPages)} disabled={page === totalPages} style={catPgBtn(page === totalPages)}>»</button>
          </div>
        </div>
      )}
    </div>
  )
}

const CAT_PAGE_SIZES = [10, 20, 50, 100]

function catPgBtn(disabled: boolean, active = false): React.CSSProperties {
  return {
    background:  active ? '#C23B22' : 'transparent',
    color:       active ? '#FBF0DC' : disabled ? '#D4B896' : '#6B4226',
    border:      '1px solid',
    borderColor: active ? '#C23B22' : '#D4B896',
    padding: '4px 10px',
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 14,
    cursor:  disabled ? 'default' : 'pointer',
    opacity: disabled ? 0.4 : 1,
    minWidth: 32,
    transition: 'all .1s',
    boxShadow: active ? '2px 2px 0 rgba(92,58,30,.15)' : 'none',
  }
}

function catPageNums(current: number, total: number): (number | null)[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)
  const pages: (number | null)[] = [1]
  if (current > 3) pages.push(null)
  for (let p = Math.max(2, current - 1); p <= Math.min(total - 1, current + 1); p++) pages.push(p)
  if (current < total - 2) pages.push(null)
  pages.push(total)
  return pages
}
