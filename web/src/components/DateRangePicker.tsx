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

  const hasCustom = !!(value.from || value.to)

  return (
    <div className="date-range-picker">
      <span className="date-range-label">
        PERIOD
      </span>

      <div className="date-range-presets">
        {PRESETS.map(p => {
          const active = activePreset === p.key
          return (
            <button
              key={p.key}
              onClick={() => applyPreset(p.key, p.days)}
              className={`date-range-preset ${active ? 'is-active' : ''}`}
            >
              {p.label}
            </button>
          )
        })}
      </div>

      {/* Custom date inputs */}
      <div className="date-range-custom">
        <input
          type="date"
          value={value.from}
          onChange={e => handleInput('from', e.target.value)}
          className="rr-input date-range-input"
          title="From date"
        />
        <span className="date-range-sep">–</span>
        <input
          type="date"
          value={value.to}
          onChange={e => handleInput('to', e.target.value)}
          className="rr-input date-range-input"
          title="To date"
        />
        {hasCustom && activePreset === (null as unknown as Preset) && (
          <button
            onClick={() => applyPreset('all', null)}
            title="Clear custom range"
            className="date-range-clear"
          >
            ✕
          </button>
        )}
      </div>
    </div>
  )
}
