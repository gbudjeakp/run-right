import { useState, useEffect } from 'react'
import { CURRENCY_OPTIONS, type CurrencyCode, useCurrencyPreference } from '../currency'

export default function SettingsPage() {
  const { currency, setCurrency } = useCurrencyPreference()
  const [otelEndpoint, setOtelEndpoint] = useState('')
  const [preferredCurrency, setPreferredCurrency] = useState<CurrencyCode>(currency)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    setOtelEndpoint(localStorage.getItem('runright_otel_endpoint') ?? '')
  }, [])

  useEffect(() => {
    setPreferredCurrency(currency)
  }, [currency])

  function save(e: React.FormEvent) {
    e.preventDefault()
    if (otelEndpoint) localStorage.setItem('runright_otel_endpoint', otelEndpoint)
    else localStorage.removeItem('runright_otel_endpoint')
    setCurrency(preferredCurrency)
    setSaved(true)
    setTimeout(() => setSaved(false), 2500)
  }


  return (
    <div className="fadein">
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-7 tracking-tight">Settings</h1>
      <div className="rr-card">
        <form className="settings-form" onSubmit={save}>
          <div className="form-group">
            <label>OpenTelemetry Endpoint</label>
            <input
              type="text"
              placeholder="http://localhost:4317"
              value={otelEndpoint}
              onChange={(e) => setOtelEndpoint(e.target.value)}
            />
            <p className="text-xs text-[var(--text-light)] mt-1.5">
              Set this in your CI job as <code>OTEL_EXPORTER_OTLP_ENDPOINT</code> and pass <code>--export otlp</code> to <code>runright monitor</code>.
            </p>
          </div>
          <div className="form-group">
            <label>Display Currency</label>
            <select
              className="rr-select"
              value={preferredCurrency}
              onChange={(e) => setPreferredCurrency(e.target.value as CurrencyCode)}
            >
              {CURRENCY_OPTIONS.map((option) => (
                <option key={option.code} value={option.code}>{option.label}</option>
              ))}
            </select>
            <p className="text-xs text-[var(--text-light)] mt-1.5">
              Affects all dashboard money values. Conversion is applied from USD using built-in reference rates.
            </p>
          </div>
          <div className="flex items-center gap-4 flex-wrap">
            <button className="btn-rr" type="submit">Save</button>
              {saved && <span className="font-deco text-[15px] tracking-[2px] text-[#2E7D32]">Saved</span>}
          </div>
        </form>
      </div>

      <div className="rr-card">
        <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-3">Usage</h2>
        <p className="text-sm text-[var(--text-light)] leading-relaxed">RunRight works with GitHub Actions, GitLab CI, Jenkins, CircleCI, and any CI platform. Here's an example:</p>
        <pre className="bg-[#1A0F02] border border-[#3a2510] border-l-[3px] border-l-gold px-4 py-4 mt-3 text-xs overflow-x-auto text-gold-light font-mono leading-loose">{`# Standalone mode
- uses: sgbudje/runright@v1
  with:
    step: start

- name: Your build step
  run: make build

- uses: sgbudje/runright@v1
  with:
    step: stop
    http-url: https://your-runright-backend.example.com
    export: file,http`}
        </pre>
        <p className="text-sm text-[var(--text-light)] leading-relaxed mt-4">Or use wrapper mode:</p>
        <pre className="bg-[#1A0F02] border border-[#3a2510] border-l-[3px] border-l-gold px-4 py-4 mt-3 text-xs overflow-x-auto text-gold-light font-mono leading-loose">{`# Wrapper mode
- uses: sgbudje/runright@v1
  with:
    run: make build
    export: file,otlp
  env:
    OTEL_EXPORTER_OTLP_ENDPOINT: http://your-grafana-alloy:4317`}
        </pre>
      </div>
    </div>
  )
}
