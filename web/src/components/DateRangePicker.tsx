import { useState } from 'react'

export interface DateRange {
  from: string // 'YYYY-MM-DD' or '' (open)
  to: string   // 'YYYY-MM-DD' or '' (open)
}

export const EMPTY_RANGE: DateRange = { from: '', to: '' }

/** Returns true when an ISO timestamp falls within the range (both ends inclusive). */
export function inDateRange(isoDate: string, range: DateRange): boolean {
  if (!range.from && !range.to) return true
  const d = isoDate.slice(0, 10) // 'YYYY-MM-DD'
  if (range.from && d < range.from) return false
  if (range.to   && d > range.to)   return false
  return true
}

type Preset = '7d' | '30d' | '90d' | '1y' | 'all'

const PRESETS: { key: Preset; label: string; days: number | null }[] = [
  { key: '7d',  label: '7D',  days: 7 },
  { key: '30d', label: '30D', days: 30 },
  { key: '90d', label: '90D', days: 90 },
  { key: '1y',  label: '1Y',  days: 365 },
  { key: 'all', label: 'ALL', days: null },
]

function toDateStr(d: Date): string {
  return d.toISOString().slice(0, 10)
}

export function DateRangePicker({
  value,
  onChange,
}: {
  value: DateRange
  onChange: (r: DateRange) => void
}) {
  const [activePreset, setActivePreset] = useState<Preset>('all')

  function applyPreset(key: Preset, days: number | null) {
    setActivePreset(key)
    if (days === null) {
      onChange(EMPTY_RANGE)
    } else {
      const now  = new Date()
      const from = new Date(now)
      from.setDate(from.getDate() - days)
      onChange({ from: toDateStr(from), to: toDateStr(now) })
    }
  }

  function handleInput(field: 'from' | 'to', val: string) {
    setActivePreset(null as unknown as Preset) // deactivate preset on manual edit
    onChange({ ...value, [field]: val })
  }

  const inputStyle: React.CSSProperties = {
    background: '#FFFDF7',
    border: '1px solid #D4B896',
    borderBottom: '2px solid #B8946A',
    color: '#2C1A0E',
    fontFamily: 'Lato, sans-serif',
    fontSize: 13,
    padding: '6px 8px',
    outline: 'none',
    colorScheme: 'light' as React.CSSProperties['colorScheme'],
    cursor: 'pointer',
  }

  const hasCustom = !!(value.from || value.to)

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
      <span style={{
        fontFamily: "'Bebas Neue', Impact, sans-serif",
        fontSize: 12, letterSpacing: 1.5, color: '#9A7B5A',
      }}>
        PERIOD
      </span>

      {PRESETS.map(p => {
        const active = activePreset === p.key
        return (
          <button
            key={p.key}
            onClick={() => applyPreset(p.key, p.days)}
            style={{
              background:   active ? '#2C1A0E' : 'transparent',
              color:        active ? '#FBF0DC' : '#9A7B5A',
              border:       '1px solid',
              borderColor:  active ? '#2C1A0E' : '#D4B896',
              padding:      '4px 10px',
              fontFamily:   "'Bebas Neue', Impact, sans-serif",
              fontSize: 13, letterSpacing: 1, cursor: 'pointer',
              transition:   'all .1s',
            }}
          >
            {p.label}
          </button>
        )
      })}

      {/* Custom date inputs */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 5, marginLeft: 4 }}>
        <input
          type="date"
          value={value.from}
          onChange={e => handleInput('from', e.target.value)}
          style={inputStyle}
          title="From date"
        />
        <span style={{ color: '#9A7B5A', fontSize: 14 }}>–</span>
        <input
          type="date"
          value={value.to}
          onChange={e => handleInput('to', e.target.value)}
          style={inputStyle}
          title="To date"
        />
        {hasCustom && activePreset === (null as unknown as Preset) && (
          <button
            onClick={() => applyPreset('all', null)}
            title="Clear custom range"
            style={{
              background: 'none', border: '1px solid #D4B896', color: '#9A7B5A',
              padding: '4px 8px', fontFamily: "'Bebas Neue'", fontSize: 12, cursor: 'pointer',
              lineHeight: 1,
            }}
          >
            ✕
          </button>
        )}
      </div>
    </div>
  )
}
