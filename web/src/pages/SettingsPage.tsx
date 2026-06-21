import { useState, useEffect } from 'react'

export default function SettingsPage() {
  const [otelEndpoint, setOtelEndpoint] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    setOtelEndpoint(localStorage.getItem('runright_otel_endpoint') ?? '')
  }, [])

  function save(e: React.FormEvent) {
    e.preventDefault()
    if (otelEndpoint) localStorage.setItem('runright_otel_endpoint', otelEndpoint)
    else localStorage.removeItem('runright_otel_endpoint')
    setSaved(true)
    setTimeout(() => setSaved(false), 2500)
  }

  return (
    <div className="fadein">
      <h1>Settings</h1>
      <div className="card">
        <form className="settings-form" onSubmit={save}>
          <div className="form-group">
            <label>OpenTelemetry Endpoint</label>
            <input
              type="text"
              placeholder="http://localhost:4317"
              value={otelEndpoint}
              onChange={(e) => setOtelEndpoint(e.target.value)}
            />
            <p style={{ fontSize: 12, color: '#718096', marginTop: 6 }}>
              Set this in your CI job as <code>OTEL_EXPORTER_OTLP_ENDPOINT</code> and pass <code>--export otlp</code> to <code>runright monitor</code>.
            </p>
          </div>
          <button className="btn" type="submit">Save</button>
          {saved && <span style={{ marginLeft: 16, fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 15, letterSpacing: 2, color: '#2E7D32' }}>✓ Saved</span>}
        </form>
      </div>

      <div className="card">
        <h2>Usage</h2>
        <p style={{ fontSize: 14, color: '#a0aec0', lineHeight: 1.7 }}>
          Add to your GitHub Actions workflow:
        </p>
        <pre style={{ background: '#1A0F02', border: '1px solid #3a2510', borderLeft: '3px solid #B8860B', padding: 16, marginTop: 12, fontSize: 12, overflowX: 'auto', color: '#D4A82A', fontFamily: "'Courier New', monospace", lineHeight: 2 }}>{`# Standalone mode
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
        <p style={{ fontSize: 14, color: '#a0aec0', lineHeight: 1.7, marginTop: 16 }}>
          Or use wrapper mode:
        </p>
        <pre style={{ background: '#1A0F02', border: '1px solid #3a2510', borderLeft: '3px solid #B8860B', padding: 16, marginTop: 12, fontSize: 12, overflowX: 'auto', color: '#D4A82A', fontFamily: "'Courier New', monospace", lineHeight: 2 }}>{`# Wrapper mode
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
