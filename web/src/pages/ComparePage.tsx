import { useState } from 'react'
import { Link } from 'react-router-dom'
import LogoMark from '../components/LogoMark'
import PageNav from '../components/PageNav'
import './LandingPage.css'
import './ComparePage.css'

type Row = [string, string, string, string, string, string]

const TOOLS = ['Datadog CI', 'Grafana /\nPrometheus', 'Sentry /\nElastic APM', 'AWS & GCP\nCost Tools', 'RunRight']

const ROWS: Row[] = [
  ['Pipeline duration & trends',                  '✓', '✓',  '—',      '—',        '—'],
  ['Error & trace tracking',                      '✓', '—',  '✓',      '—',        '—'],
  ['Raw CPU / memory graphs',                     '✓', '✓',  '—',      '—',        '✓'],
  ['Works on ephemeral CI machines',              '—', '—',  '—',      '—',        '✓'],
  ['Maps usage to a specific instance SKU',       '—', '—',  '—',      'partial',  '✓'],
  ['Recommends cheaper alternative + cost delta', '—', '—',  '—',      'partial',  '✓'],
  ['No code changes needed',                      '—', '✓',  '—',      '✓',        '✓'],
  ['Self-hosted, no SaaS',                        '—', '✓',  '—',      '—',        '✓'],
]

export default function ComparePage() {
  const [dark, setDark] = useState(false)

  return (
    <div className={`lp-root cp-root${dark ? ' lp-dark' : ''}`}>

      {/* Nav */}
      <PageNav dark={dark} onToggleDark={() => setDark(d => !d)} />

      {/* Hero */}
      <section className="cp-hero">
        <p className="cp-eyebrow">VS THE ALTERNATIVES</p>
        <h1 className="cp-title">Built for a problem others don't solve.</h1>
        <p className="cp-sub">
          Datadog, Sentry, and AWS Cost Tools answer <em>"what happened?"</em>{' '}
          RunRight answers <em>"am I using the right machine?"</em>
        </p>
      </section>

      {/* Content */}
      <div className="cp-content">

        {/* Comparison table */}
        <div className="cp-table-scroll">
          <table className="cp-table">
            <thead>
              <tr>
                <th>FEATURE</th>
                {TOOLS.map((t, i) => (
                  <th key={t} className={i === TOOLS.length - 1 ? 'col-rr' : ''}>{t}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {ROWS.map(([label, ...vals], ri) => (
                <tr key={ri}>
                  <td>{label}</td>
                  {vals.map((v, ci) => {
                    const isRR = ci === TOOLS.length - 1
                    const cls = [
                      isRR ? 'col-rr' : '',
                      v === '✓' && !isRR ? 'check-other' : '',
                      v === 'partial' ? 'partial' : '',
                    ].filter(Boolean).join(' ')
                    return (
                      <td key={ci} className={cls || undefined}>
                        {v === 'partial' ? 'Partial' : v}
                      </td>
                    )
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Callout — dark brown, not red */}
        <div className="cp-callout">
          <div className="cp-callout-label">Why cloud-native tools miss CI machines</div>
          <p>
            AWS Cost Optimizer and GCP Recommender require weeks of utilization history on{' '}
            <strong>long-running instances</strong>. CI machines are <strong>ephemeral</strong> —
            they spin up, run a 5–30 minute job, then terminate. The cloud has no visibility into
            what your GitHub Actions runner or Jenkins agent actually did.
            RunRight captures the entire run in real time and produces a recommendation the
            moment the job finishes. Use it alongside Datadog or Sentry via OTLP — you don't
            have to choose.
          </p>
        </div>

        {/* CTA */}
        <div className="cp-cta">
          <Link to="/login" className="cp-cta-btn">Open Dashboard →</Link>
        </div>

      </div>

      {/* Footer */}
      <footer className="cp-footer">
        <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <LogoMark size={14} color="#9A7B5A" /> RUNRIGHT
        </span>
        <Link to="/pricing" className="cp-footer-link">Pricing</Link>
        <Link to="/" className="cp-footer-link">Back to home</Link>
      </footer>

    </div>
  )
}
