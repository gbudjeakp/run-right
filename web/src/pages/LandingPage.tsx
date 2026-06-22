import { useState } from 'react'
import { Link } from 'react-router-dom'
import './LandingPage.css'
import LogoMark from '../components/LogoMark'
import PageNav from '../components/PageNav'

// ── Feature icons ─────────────────────────────────────────────────
function IconMetrics({ color = '#C23B22' }: { color?: string }) {
  return (
    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="3" y="13" width="4" height="8" rx=".5" fill={color} opacity=".5" />
      <rect x="10" y="8" width="4" height="13" rx=".5" fill={color} opacity=".75" />
      <rect x="17" y="3" width="4" height="18" rx=".5" fill={color} />
      <line x1="2" y1="21.5" x2="22" y2="21.5" stroke={color} strokeWidth="1.2" />
    </svg>
  )
}

function IconCloud({ color = '#C23B22' }: { color?: string }) {
  return (
    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M6.5 19a4.5 4.5 0 0 1-.5-9 5.5 5.5 0 0 1 10.8-1.5A4 4 0 1 1 18 19H6.5Z" stroke={color} strokeWidth="1.6" strokeLinejoin="round" />
    </svg>
  )
}

function IconExport({ color = '#C23B22' }: { color?: string }) {
  return (
    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="5" cy="12" r="2" stroke={color} strokeWidth="1.5" />
      <circle cx="19" cy="6" r="2" stroke={color} strokeWidth="1.5" />
      <circle cx="19" cy="18" r="2" stroke={color} strokeWidth="1.5" />
      <line x1="7" y1="11" x2="17" y2="7" stroke={color} strokeWidth="1.4" strokeLinecap="round" />
      <line x1="7" y1="13" x2="17" y2="17" stroke={color} strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  )
}

function IconServer({ color = '#C23B22' }: { color?: string }) {
  return (
    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="3" y="4" width="18" height="5" rx="1" stroke={color} strokeWidth="1.5" />
      <rect x="3" y="11" width="18" height="5" rx="1" stroke={color} strokeWidth="1.5" />
      <circle cx="18" cy="6.5" r="1" fill={color} />
      <circle cx="18" cy="13.5" r="1" fill={color} />
      <line x1="8" y1="20" x2="16" y2="20" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
      <line x1="12" y1="16" x2="12" y2="20" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  )
}

function GitHubIcon({ size = 18, color = 'currentColor' }: { size?: number; color?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill={color} xmlns="http://www.w3.org/2000/svg">
      <path d="M12 2C6.477 2 2 6.477 2 12c0 4.418 2.865 8.166 6.839 9.489.5.092.682-.217.682-.482 0-.237-.009-.868-.013-1.703-2.782.604-3.369-1.342-3.369-1.342-.454-1.155-1.11-1.462-1.11-1.462-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0 1 12 6.836a9.59 9.59 0 0 1 2.504.337c1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.163 22 16.418 22 12c0-5.523-4.477-10-10-10Z" />
    </svg>
  )
}

// ── Moon / Sun icons for dark-mode toggle ────────────────────────
interface Props {
  onEnter: () => void
  dark: boolean
  onToggleDark: () => void
}

const S = {
  // Layout
  page: {
    background: 'var(--lp-bg)',
    minHeight: '100vh',
    fontFamily: "'Lato', system-ui, sans-serif",
    color: 'var(--lp-text)',
  } as React.CSSProperties,

  // Nav
  nav: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '20px 60px',
    borderBottom: '1px solid var(--lp-border)',
    background: 'var(--lp-bg)',
  } as React.CSSProperties,

  navLogo: {
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 22,
    letterSpacing: 4,
    color: 'var(--lp-text)',
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    textDecoration: 'none',
  } as React.CSSProperties,

  navBtn: {
    background: 'none',
    border: '1.5px solid var(--lp-text)',
    color: 'var(--lp-text)',
    padding: '8px 22px',
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 14,
    letterSpacing: 2,
    cursor: 'pointer',
    transition: 'background .15s, color .15s, border-color .15s',
  } as React.CSSProperties,

  // Hero
  hero: {
    borderBottom: '1px solid var(--lp-border)',
  } as React.CSSProperties,

  heroInner: {
    maxWidth: 1100,
    margin: '0 auto',
    padding: '80px 60px',
    display: 'flex',
    alignItems: 'center',
    gap: 72,
  } as React.CSSProperties,

  heroLeft: {
    flex: 1,
    minWidth: 0,
  } as React.CSSProperties,

  heroRight: {
    flex: '0 0 320px',
    background: '#1A0F02',
    border: '1px solid #3a2510',
    borderLeft: '3px solid #B8860B',
    padding: '24px 28px',
    fontFamily: "'Courier New', monospace",
    fontSize: 13,
    color: '#E8C458',
    lineHeight: 1.8,
  } as React.CSSProperties,

  eyebrow: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 8,
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 11,
    letterSpacing: 4,
    color: '#9A7B5A',
    border: '1px solid var(--lp-border)',
    padding: '5px 14px',
    marginBottom: 24,
  } as React.CSSProperties,

  heroTitle: {
    fontFamily: "'Playfair Display', Georgia, serif",
    fontSize: 'clamp(36px, 4.5vw, 56px)',
    fontWeight: 900,
    lineHeight: 1.1,
    color: 'var(--lp-text)',
    marginBottom: 20,
    letterSpacing: '-0.5px',
  } as React.CSSProperties,

  heroSub: {
    fontSize: 16,
    color: 'var(--lp-text-mid)',
    lineHeight: 1.75,
    maxWidth: 440,
    margin: '0 0 32px',
    fontWeight: 400,
  } as React.CSSProperties,

  heroBtn: {
    display: 'inline-block',
    background: '#C23B22',
    color: '#FBF0DC',
    border: 'none',
    padding: '13px 38px',
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 15,
    letterSpacing: 3,
    cursor: 'pointer',
    transition: 'background .15s, transform .08s',
    textDecoration: 'none',
  } as React.CSSProperties,

  // Divider
  divider: {
    display: 'flex',
    alignItems: 'center',
    gap: 16,
    maxWidth: 700,
    margin: '0 auto',
    padding: '0 60px',
  } as React.CSSProperties,

  dividerLine: {
    flex: 1,
    height: 1,
    background: 'var(--lp-border)',
  } as React.CSSProperties,

  dividerText: {
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 13,
    letterSpacing: 3,
    color: 'var(--lp-text-light)',
    whiteSpace: 'nowrap' as const,
  },

  // Sections
  section: {
    maxWidth: 900,
    margin: '0 auto',
    padding: '72px 60px',
  } as React.CSSProperties,

  sectionTitle: {
    fontFamily: "'Playfair Display', Georgia, serif",
    fontSize: 32,
    fontWeight: 700,
    color: 'var(--lp-text)',
    textAlign: 'center' as const,
    marginBottom: 8,
  },

  sectionSub: {
    textAlign: 'center' as const,
    color: '#9A7B5A',
    marginBottom: 52,
    fontWeight: 400,
    fontSize: 17,
  },

  steps: {
    display: 'grid',
    gridTemplateColumns: 'repeat(3, 1fr)',
    gap: 28,
  } as React.CSSProperties,

  step: {
    background: 'var(--lp-paper)',
    border: '1px solid var(--lp-border)',
    padding: '28px 24px',
    boxShadow: '3px 3px 0 rgba(92,58,30,.12)',
    position: 'relative' as const,
  },

  stepNum: {
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 48,
    color: 'var(--lp-border)',
    lineHeight: 1,
    marginBottom: 12,
  } as React.CSSProperties,

  stepTitle: {
    fontFamily: "'Playfair Display', Georgia, serif",
    fontWeight: 700,
    fontSize: 18,
    marginBottom: 10,
    color: 'var(--lp-text)',
  } as React.CSSProperties,

  stepText: {
    fontSize: 15,
    color: 'var(--lp-text-mid)',
    lineHeight: 1.75,
    fontWeight: 400,
  } as React.CSSProperties,

  // Features strip
  featureStrip: {
    background: 'var(--lp-dark-strip)',
    padding: '64px 60px',
  } as React.CSSProperties,

  features: {
    maxWidth: 900,
    margin: '0 auto',
    display: 'grid',
    gridTemplateColumns: 'repeat(4, 1fr)',
    gap: 32,
  } as React.CSSProperties,

  feature: {
    textAlign: 'left' as const,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
  } as React.CSSProperties,

  featureIcon: {
    fontSize: 28,
    marginBottom: 12,
  } as React.CSSProperties,

  featureTitle: {
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 17,
    letterSpacing: 2,
    color: '#FBF0DC',
    marginBottom: 10,
  } as React.CSSProperties,

  featureText: {
    fontSize: 14,
    color: 'rgba(251,240,220,.65)',
    lineHeight: 1.75,
    fontWeight: 400,
  } as React.CSSProperties,

  // Code block
  codeBlock: {
    background: '#1A0F02',
    border: '1px solid #3a2510',
    borderLeft: '3px solid #B8860B',
    padding: '20px 24px',
    fontFamily: "'Courier New', monospace",
    fontSize: 13,
    color: '#D4A82A',
    lineHeight: 2,
    marginTop: 20,
    overflowX: 'auto' as const,
  },

  // CTA section
  ctaSection: {
    background: 'var(--lp-paper-alt)',
    borderTop: '1px solid var(--lp-border)',
    borderBottom: '1px solid var(--lp-border)',
    padding: '72px 60px',
    textAlign: 'center' as const,
  } as React.CSSProperties,

  // Footer
  footer: {
    padding: '28px 60px',
    borderTop: '1px solid var(--lp-border)',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 12,
    letterSpacing: 2,
    color: 'var(--lp-text-light)',
  } as React.CSSProperties,
}

function CITabs() {
  const [active, setActive] = useState('github')

  const tabs = [
    { id: 'github',     label: 'GitHub Actions' },
    { id: 'gitlab',     label: 'GitLab CI'      },
    { id: 'circleci',   label: 'CircleCI'        },
    { id: 'bitbucket',  label: 'Bitbucket'       },
    { id: 'jenkins',    label: 'Jenkins'         },
  ]

  const INSTALL = `curl -fsSL "https://github.com/gbudjeakp/run-right/releases/latest/download/runright_linux_amd64" -o runright && chmod +x runright`

  const snippets: Record<string, { label: string; code: string }> = {
    github: {
      label: '.github/workflows/ci.yml',
      code:
`- uses: gbudjeakp/run-right@v1
  with:
    run: make build
    export: file,http
    http-url: \${{ vars.RUNRIGHT_URL }}
  env:
    RUNRIGHT_API_KEY: \${{ secrets.RUNRIGHT_API_KEY }}`,
    },
    gitlab: {
      label: '.gitlab-ci.yml',
      code:
`build:
  before_script:
    - ${INSTALL}
    - ./runright monitor --export http --http-url "$RUNRIGHT_URL" &
    - echo $! > .runright.pid
  script:
    - make build
  after_script:
    - kill $(cat .runright.pid) 2>/dev/null || true`,
    },
    circleci: {
      label: '.circleci/config.yml',
      code:
`jobs:
  build:
    docker:
      - image: cimg/base:stable
    steps:
      - checkout
      - run:
          name: Install RunRight
          command: ${INSTALL}
      - run:
          name: Build
          command: |
            ./runright monitor --export http --http-url "$RUNRIGHT_URL" &
            echo $! > .runright.pid
            make build
            kill $(cat .runright.pid) 2>/dev/null || true`,
    },
    bitbucket: {
      label: 'bitbucket-pipelines.yml',
      code:
`pipelines:
  default:
    - step:
        script:
          - ${INSTALL}
          - ./runright monitor --export http --http-url "$RUNRIGHT_URL" &
          - echo $! > .runright.pid
          - make build
          - kill $(cat .runright.pid) 2>/dev/null || true`,
    },
    jenkins: {
      label: 'Jenkinsfile',
      code:
`pipeline {
  agent any
  environment {
    RUNRIGHT_URL     = credentials('runright-url')
    RUNRIGHT_API_KEY = credentials('runright-api-key')
  }
  stages {
    stage('Build') {
      steps {
        sh """
          ${INSTALL}
          ./runright monitor --export http --http-url \\$RUNRIGHT_URL &
          echo \\$! > .runright.pid
          make build
          kill \\$(cat .runright.pid) 2>/dev/null || true
        """
      }
    }
  }
}`,
    },
  }

  const current = snippets[active]

  return (
    <div>
      {/* Tab bar */}
      <div style={{
        display: 'flex',
        borderBottom: '2px solid #D4B896',
        marginBottom: 0,
        flexWrap: 'wrap' as const,
        gap: 0,
      }}>
        {tabs.map(t => (
          <button
            key={t.id}
            onClick={() => setActive(t.id)}
            style={{
              background: active === t.id ? '#2C1A0E' : 'transparent',
              color: active === t.id ? '#FBF0DC' : '#9A7B5A',
              border: 'none',
              borderBottom: active === t.id ? '2px solid #2C1A0E' : '2px solid transparent',
              padding: '9px 18px',
              fontFamily: "'Bebas Neue', Impact, sans-serif",
              fontSize: 13,
              letterSpacing: 1.5,
              cursor: 'pointer',
              marginBottom: -2,
              transition: 'color .12s, background .12s',
            }}
            onMouseEnter={e => { if (active !== t.id) (e.currentTarget).style.color = '#2C1A0E' }}
            onMouseLeave={e => { if (active !== t.id) (e.currentTarget).style.color = '#9A7B5A' }}
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* Code block */}
      <CopyBlock label={current.label} code={current.code} />
    </div>
  )
}


function CopyBlock({ label, code }: { label: string; code: string }) {
  const [copied, setCopied] = useState(false)

  function copy() {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <div style={{ position: 'relative', marginTop: 16 }}>
      <div style={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        background: '#2C1A0E',
        padding: '7px 16px',
        borderLeft: '3px solid #B8860B',
      }}>
        <span style={{
          fontFamily: "'Bebas Neue', Impact, sans-serif",
          fontSize: 11,
          letterSpacing: 2,
          color: '#9A7B5A',
        }}>{label}</span>
        <button
          onClick={copy}
          aria-label={copied ? 'Copied to clipboard' : 'Copy to clipboard'}
          aria-live="polite"
          style={{
            background: copied ? '#2E7D32' : 'transparent',
            border: `1px solid ${copied ? '#2E7D32' : '#4a3520'}`,
            color: copied ? '#FBF0DC' : '#9A7B5A',
            padding: '3px 12px',
            fontFamily: "'Bebas Neue', Impact, sans-serif",
            fontSize: 11,
            letterSpacing: 1.5,
            cursor: 'pointer',
            transition: 'all .15s',
          }}
          onMouseEnter={e => { if (!copied) { (e.currentTarget).style.borderColor = '#B8860B'; (e.currentTarget).style.color = '#D4A82A' } }}
          onMouseLeave={e => { if (!copied) { (e.currentTarget).style.borderColor = '#4a3520'; (e.currentTarget).style.color = '#9A7B5A' } }}
        >
          {copied ? '✓ Copied' : 'Copy'}
        </button>
      </div>
      <pre style={{
        background: '#1A0F02',
        borderLeft: '3px solid #B8860B',
        borderRight: '1px solid #3a2510',
        borderBottom: '1px solid #3a2510',
        padding: '16px 20px',
        fontFamily: "'Courier New', monospace",
        fontSize: 13,
        color: '#D4A82A',
        lineHeight: 1.9,
        margin: 0,
        overflowX: 'auto',
        whiteSpace: 'pre',
      }}>{code}</pre>
    </div>
  )
}

// ── Savings Calculator ────────────────────────────────────────────
const CALC_MACHINES = [
  { id: 'gh4',  label: 'GitHub Actions — ubuntu-latest (4 vCPU)',    pricePerMin: 0.008,    savingsPct: 0.20 },
  { id: 'gh8',  label: 'GitHub Actions — 8-core runner',              pricePerMin: 0.016,    savingsPct: 0.38 },
  { id: 'gh16', label: 'GitHub Actions — 16-core runner',             pricePerMin: 0.064,    savingsPct: 0.52 },
  { id: 'c5xl', label: 'Self-hosted EC2 c5.xlarge (4c / 8 GB)',       pricePerMin: 0.00283,  savingsPct: 0.42 },
  { id: 'c52x', label: 'Self-hosted EC2 c5.2xlarge (8c / 16 GB)',     pricePerMin: 0.00567,  savingsPct: 0.52 },
  { id: 'm5xl', label: 'Self-hosted EC2 m5.xlarge (4c / 16 GB)',      pricePerMin: 0.00320,  savingsPct: 0.36 },
  { id: 'm52x', label: 'Self-hosted EC2 m5.2xlarge (8c / 32 GB)',     pricePerMin: 0.00640,  savingsPct: 0.50 },
  { id: 'n2s2', label: 'Self-hosted GCP n2-standard-2 (2c / 8 GB)',   pricePerMin: 0.00163,  savingsPct: 0.30 },
  { id: 'n2s4', label: 'Self-hosted GCP n2-standard-4 (4c / 16 GB)', pricePerMin: 0.00325,  savingsPct: 0.40 },
  { id: 'n2s8', label: 'Self-hosted GCP n2-standard-8 (8c / 32 GB)', pricePerMin: 0.00651,  savingsPct: 0.50 },
]

function SavingsCalc() {
  const [machineIdx, setMachineIdx] = useState(0)
  const [duration, setDuration] = useState(12)
  const [jobsPerDay, setJobsPerDay] = useState(80)

  const m = CALC_MACHINES[machineIdx]
  const currentMonthly = m.pricePerMin * duration * jobsPerDay * 30
  const savings = currentMonthly * m.savingsPct
  const recommended = currentMonthly - savings

  const fmt = (n: number) =>
    n >= 1000 ? `$${(n / 1000).toFixed(1)}k` : `$${n.toFixed(0)}`

  return (
    <div style={{ borderTop: '1px solid var(--lp-border)', borderBottom: '1px solid var(--lp-border)' }}>
      <div style={{ maxWidth: 900, margin: '0 auto', padding: '72px 60px' }} className="lp-section">

        <p style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 11, letterSpacing: 5, color: '#9A7B5A', margin: '0 0 12px', textAlign: 'center' }}>
          SAVINGS CALCULATOR
        </p>
        <h2 style={{ fontFamily: "'Playfair Display', Georgia, serif", fontSize: 'clamp(28px, 3.5vw, 40px)', fontWeight: 900, color: 'var(--lp-text)', textAlign: 'center', marginBottom: 8, letterSpacing: '-0.5px' }}>
          How much could you save?
        </h2>
        <p style={{ textAlign: 'center', color: 'var(--lp-text-mid)', fontSize: 15, marginBottom: 48 }}>
          Based on typical CI job utilisation (p95 CPU ~30%, p95 RAM ~40%).
        </p>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 48, alignItems: 'start' }} className="lp-calc-grid">

          {/* Controls */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 28 }}>

            <div>
              <label style={{ display: 'block', fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 11, letterSpacing: 3, color: '#9A7B5A', marginBottom: 10 }}>
                CURRENT MACHINE
              </label>
              <select
                value={machineIdx}
                onChange={e => setMachineIdx(Number(e.target.value))}
                style={{
                  width: '100%',
                  background: 'var(--lp-paper)',
                  border: '1.5px solid var(--lp-border)',
                  color: 'var(--lp-text)',
                  padding: '10px 14px',
                  fontFamily: "'Lato', system-ui, sans-serif",
                  fontSize: 14,
                  appearance: 'none' as const,
                  cursor: 'pointer',
                }}
              >
                {CALC_MACHINES.map((mc, i) => (
                  <option key={mc.id} value={i}>{mc.label}</option>
                ))}
              </select>
            </div>

            <div>
              <label style={{ display: 'flex', justifyContent: 'space-between', fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 11, letterSpacing: 3, color: '#9A7B5A', marginBottom: 10 }}>
                <span>AVG JOB DURATION</span>
                <span style={{ color: 'var(--lp-text)' }}>{duration} min</span>
              </label>
              <input type="range" min={1} max={60} value={duration} onChange={e => setDuration(Number(e.target.value))}
                style={{ width: '100%', accentColor: '#C23B22', cursor: 'pointer' }} />
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: 'var(--lp-text-light)', marginTop: 4 }}>
                <span>1 min</span><span>60 min</span>
              </div>
            </div>

            <div>
              <label style={{ display: 'flex', justifyContent: 'space-between', fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 11, letterSpacing: 3, color: '#9A7B5A', marginBottom: 10 }}>
                <span>JOBS PER DAY</span>
                <span style={{ color: 'var(--lp-text)' }}>{jobsPerDay}</span>
              </label>
              <input type="range" min={1} max={500} value={jobsPerDay} onChange={e => setJobsPerDay(Number(e.target.value))}
                style={{ width: '100%', accentColor: '#C23B22', cursor: 'pointer' }} />
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: 'var(--lp-text-light)', marginTop: 4 }}>
                <span>1</span><span>500</span>
              </div>
            </div>

          </div>

          {/* Output */}
          <div style={{ background: 'var(--lp-paper)', border: '1.5px solid var(--lp-border)', padding: '32px 28px', boxShadow: '4px 4px 0 rgba(44,26,14,.08)' }}>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 20, marginBottom: 24, paddingBottom: 24, borderBottom: '1px solid var(--lp-border)' }}>
              <div>
                <p style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 10, letterSpacing: 3, color: '#9A7B5A', margin: '0 0 6px' }}>CURRENT / MO</p>
                <p style={{ fontFamily: "'Playfair Display', serif", fontWeight: 900, fontSize: 32, color: 'var(--lp-text)', margin: 0, lineHeight: 1 }}>{fmt(currentMonthly)}</p>
              </div>
              <div>
                <p style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 10, letterSpacing: 3, color: '#C23B22', margin: '0 0 6px' }}>SAVINGS / MO</p>
                <p style={{ fontFamily: "'Playfair Display', serif", fontWeight: 900, fontSize: 32, color: '#C23B22', margin: 0, lineHeight: 1 }}>{fmt(savings)}</p>
              </div>
            </div>

            <div style={{ marginBottom: 24 }}>
              <p style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 10, letterSpacing: 3, color: '#9A7B5A', margin: '0 0 6px' }}>AFTER RUNRIGHT / MO</p>
              <p style={{ fontFamily: "'Playfair Display', serif", fontWeight: 900, fontSize: 28, color: 'var(--lp-text)', margin: '0 0 4px', lineHeight: 1 }}>{fmt(recommended)}</p>
              <p style={{ fontSize: 12, color: '#9A7B5A', margin: 0 }}>~{Math.round(m.savingsPct * 100)}% reduction on right-sized machines</p>
            </div>

            <div style={{ background: 'var(--lp-paper-alt)', padding: '12px 14px', borderLeft: '3px solid #C23B22', fontSize: 13, color: 'var(--lp-text-mid)', lineHeight: 1.6 }}>
              Estimates use p95 usage at ~30% CPU / ~40% RAM headroom. Actual savings vary by workload.
            </div>

          </div>
        </div>
      </div>
    </div>
  )
}

export default function LandingPage({ onEnter, dark, onToggleDark }: Props) {
  return (
    <div style={S.page} className={`lp-root${dark ? ' lp-dark' : ''}`}>

      {/* Nav */}
      <PageNav dark={dark} onToggleDark={onToggleDark} onSignIn={onEnter} />

      {/* Hero */}
      <section style={S.hero} aria-label="Hero" className="lp-hero">
        <div style={S.heroInner} className="lp-hero-inner">

          {/* Left — copy */}
          <div style={S.heroLeft}>
            <p style={S.eyebrow}>OPEN SOURCE · FREE · AWS &amp; GCP</p>
            <h1 style={S.heroTitle}>
              Stop guessing at<br />
              <em>CI machine sizes.</em>
            </h1>
            <p style={S.heroSub}>
              RunRight monitors every CI job and maps your actual p95 usage to the
              right AWS or GCP instance type — automatically, on every run.
            </p>
            <Link
              to="/install"
              style={S.heroBtn}
              onMouseEnter={e => { (e.currentTarget as HTMLElement).style.background = '#9B2D17'; (e.currentTarget as HTMLElement).style.transform = 'translateY(-1px)' }}
              onMouseLeave={e => { (e.currentTarget as HTMLElement).style.background = '#C23B22'; (e.currentTarget as HTMLElement).style.transform = 'none' }}
            >
              Install →
            </Link>
          </div>

          {/* Right — recommendation visual */}
          <div style={S.heroRight} className="lp-hero-visual">
            <div style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 10, letterSpacing: 3, color: '#C4A882', marginBottom: 14, paddingBottom: 14, borderBottom: '1px solid #4a2e18' }}>
              RUNRIGHT · build-and-test
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
              <span style={{ color: '#C4A882' }}>p95 cpu</span>
              <span>1.2 cores</span>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 20, paddingBottom: 20, borderBottom: '1px solid #4a2e18' }}>
              <span style={{ color: '#C4A882' }}>p95 ram</span>
              <span>1.8 GB</span>
            </div>
            <div style={{ marginBottom: 16, paddingBottom: 16, borderBottom: '1px solid #4a2e18' }}>
              <div style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 10, letterSpacing: 3, color: '#C4A882', marginBottom: 6 }}>CURRENT</div>
              <div style={{ color: '#FBF0DC' }}>c5.xlarge</div>
              <div style={{ fontSize: 11, color: '#C4A882' }}>4 vCPU · 8 GB · $0.0194/min</div>
            </div>
            <div style={{ marginBottom: 20, paddingBottom: 20, borderBottom: '1px solid #4a2e18' }}>
              <div style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 10, letterSpacing: 3, color: '#C23B22', marginBottom: 6 }}>RECOMMENDED ↓ 58%</div>
              <div style={{ color: '#FBF0DC', fontWeight: 700 }}>t4g.small</div>
              <div style={{ fontSize: 11, color: '#C4A882' }}>2 vCPU · 2 GB · $0.0042/min</div>
            </div>
            <div style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 10, letterSpacing: 3, color: '#C4A882', marginBottom: 4 }}>MONTHLY SAVINGS</div>
            <div style={{ fontFamily: "'Playfair Display', serif", fontSize: 26, fontWeight: 900, color: '#C23B22', letterSpacing: '-0.5px', lineHeight: 1 }}>~$18.40</div>
          </div>

        </div>
      </section>

      {/* Social proof bar */}
      <div style={{
        borderTop: '1px solid var(--lp-border)',
        borderBottom: '1px solid var(--lp-border)',
        background: 'var(--lp-paper)',
        padding: '13px 60px',
        display: 'flex',
        justifyContent: 'center',
        gap: 48,
        flexWrap: 'wrap' as const,
      }} className="lp-proof-bar">
        {['160+ AWS instances', '60+ GCP instances', 'Open Source', 'Self-Hosted'].map(s => (
          <span key={s} style={{
            fontFamily: "'Bebas Neue', Impact, sans-serif",
            fontSize: 12,
            letterSpacing: 2.5,
            color: '#9A7B5A',
          }}>
            ❖ {s}
          </span>
        ))}
      </div>

      {/* Divider ornament */}
      <div style={S.divider}>
        <div style={S.dividerLine} />
        <span style={S.dividerText}>✦ HOW IT WORKS ✦</span>
        <div style={S.dividerLine} />
      </div>

      {/* How it works */}
      <div style={{ borderTop: '1px solid var(--lp-border)', ...S.section }} className="lp-section">
        <p style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 11, letterSpacing: 4, color: '#9A7B5A', textAlign: 'center' as const, marginBottom: 12 }}>HOW IT WORKS</p>
        <h2 style={{ ...S.sectionTitle, marginBottom: 48 }}>Three steps. No code changes.</h2>
        <div style={S.steps} className="lp-steps">
          <div style={S.step}>
            <div style={S.stepNum}>01</div>
            <div style={S.stepTitle}>Monitor Every Run</div>
            <p style={S.stepText}>
              Add one line to your GitHub Action. RunRight samples CPU, memory,
              disk I/O, and threads throughout your build and stores a snapshot
              for every run, automatically.
            </p>
          </div>
          <div style={S.step}>
            <div style={S.stepNum}>02</div>
            <div style={S.stepTitle}>Build a History</div>
            <p style={S.stepText}>
              The dashboard accumulates job history over time. As your build grows
              or shrinks, the data reflects it. No one-off audits, no spreadsheets.
              Everything is already there.
            </p>
          </div>
          <div style={S.step}>
            <div style={S.stepNum}>03</div>
            <div style={S.stepTitle}>Get Accurate Recommendations</div>
            <p style={S.stepText}>
              RunRight scores every instance in the AWS and GCP catalog against
              your real workload history and ranks options by cost, headroom, and fit.
              Pick the right machine with confidence.
            </p>
          </div>
        </div>
      </div>

      {/* Features strip */}
      <div style={S.featureStrip} className="lp-feature-strip">
        <div style={S.features} className="lp-features">
          {[
            { Icon: IconMetrics, title: 'Real Metrics',  text: 'CPU, memory, disk I/O and threads sampled live, not guessed.' },
            { Icon: IconCloud,   title: 'AWS + GCP',     text: 'Hundreds of instance types, kept fresh automatically.' },
            { Icon: IconExport,  title: 'Any Exporter',  text: 'File, OTLP, Prometheus, HTTP. Plug into your existing stack.' },
            { Icon: IconServer,  title: 'Self-hosted',   text: 'Your data stays on your infra. No third-party SaaS.' },
          ].map(f => (
            <div key={f.title} style={S.feature}>
              <div style={{ marginBottom: 14 }}><f.Icon /></div>
              <div style={S.featureTitle}>{f.title}</div>
              <p style={S.featureText}>{f.text}</p>
            </div>
          ))}
        </div>
      </div>

      {/* Works Everywhere */}
      <div style={{ background: 'var(--lp-works-bg)', borderTop: '1px solid var(--lp-border)', borderBottom: '1px solid var(--lp-border)', padding: '72px 60px' }} className="lp-works">
        <div style={{ maxWidth: 900, margin: '0 auto' }}>
          <h2 style={S.sectionTitle}>Works Everywhere You Run CI</h2>
          <p style={S.sectionSub}>Bare VMs, containers, K8s pods, DinD — detection adapts to the environment automatically.</p>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 20 }} className="lp-card-grid">

            <div style={{ background: 'var(--lp-paper)', border: '1px solid var(--lp-border)', padding: '28px', boxShadow: '3px 3px 0 rgba(92,58,30,.1)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 14 }}>
                <div style={{ fontFamily: "'Playfair Display', Georgia, serif", fontWeight: 700, fontSize: 18, color: 'var(--lp-text)' }}>EC2 / GCP VMs</div>
                <span style={{ fontFamily: "'Bebas Neue'", fontSize: 10, letterSpacing: 2, color: '#2E7D32', border: '1px solid #2E7D32', padding: '2px 7px' }}>ZERO CONFIG</span>
              </div>
              <p style={{ fontSize: 14, color: 'var(--lp-text-mid)', lineHeight: 1.85, margin: 0 }}>
                Reads vCPU count and total memory from the OS, then matches against the catalog within a 2 GiB tolerance. Works on GitHub-hosted runners, self-hosted EC2, and GCP VMs.
              </p>
            </div>

            <div style={{ background: 'var(--lp-paper)', border: '1px solid var(--lp-border)', padding: '28px', boxShadow: '3px 3px 0 rgba(92,58,30,.1)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 14 }}>
                <div style={{ fontFamily: "'Playfair Display', Georgia, serif", fontWeight: 700, fontSize: 18, color: 'var(--lp-text)' }}>Kubernetes + DinD Pods</div>
                <span style={{ fontFamily: "'Bebas Neue'", fontSize: 10, letterSpacing: 2, color: '#1B3361', border: '1px solid #1B3361', padding: '2px 7px' }}>CONTAINER-AWARE</span>
              </div>
              <p style={{ fontSize: 14, color: 'var(--lp-text-mid)', lineHeight: 1.85, marginBottom: 14 }}>
                Reads cgroup v2 (<code style={{ fontFamily: 'monospace', background: 'var(--lp-paper-alt)', padding: '1px 4px', fontSize: 12 }}>cpu.max</code>, <code style={{ fontFamily: 'monospace', background: 'var(--lp-paper-alt)', padding: '1px 4px', fontSize: 12 }}>memory.max</code>) or cgroup v1 equivalents. Reflects your pod's actual CPU quota and memory limit, not the underlying node's resources.
              </p>
              <pre style={{ background: '#1A0F02', padding: '10px 14px', fontFamily: "'Courier New', monospace", fontSize: 11, color: '#D4A82A', borderLeft: '3px solid #B8860B', lineHeight: 1.85, margin: 0, overflowX: 'auto' }}>{`# K8s Downward API override (optional)
env:
  - name: RUNRIGHT_VCPUS
    valueFrom:
      resourceFieldRef:
        resource: limits.cpu
  - name: RUNRIGHT_MEMORY_GIB
    valueFrom:
      resourceFieldRef:
        resource: limits.memory
        divisor: 1Gi`}</pre>
            </div>

            <div style={{ background: 'var(--lp-paper)', border: '1px solid var(--lp-border)', padding: '28px', boxShadow: '3px 3px 0 rgba(92,58,30,.1)', gridColumn: '1 / -1' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 14 }}>
                <div style={{ fontFamily: "'Playfair Display', Georgia, serif", fontWeight: 700, fontSize: 18, color: 'var(--lp-text)' }}>Any Environment</div>
                <span style={{ fontFamily: "'Bebas Neue'", fontSize: 10, letterSpacing: 2, color: '#9A7B5A', border: '1px solid #9A7B5A', padding: '2px 7px' }}>MANUAL OVERRIDE</span>
              </div>
              <p style={{ fontSize: 14, color: 'var(--lp-text-mid)', lineHeight: 1.85, marginBottom: 14 }}>
                Skip auto-detection with two env vars. Works for Azure VMs, ARM instances, on-prem, or any environment where OS-level resource counts are unreliable.
              </p>
              <pre style={{ background: '#1A0F02', padding: '10px 14px', fontFamily: "'Courier New', monospace", fontSize: 12, color: '#D4A82A', borderLeft: '3px solid #B8860B', lineHeight: 1.85, margin: 0 }}>{`RUNRIGHT_VCPUS=4\nRUNRIGHT_MEMORY_GIB=16`}</pre>
            </div>

          </div>
        </div>
      </div>

      {/* CI integrations */}
      <div style={{ ...S.section, paddingBottom: 56 }} className="lp-section">
        <h2 style={{ ...S.sectionTitle, textAlign: 'left', marginBottom: 6 }}>Works in any CI</h2>
        <p style={{ ...S.sectionSub, textAlign: 'left', marginBottom: 24 }}>
          Native GitHub Action or a two-line shell snippet. Pick whichever fits your stack.
        </p>
        <CITabs />

        {/* Install section */}
        <div style={{ borderTop: '1px solid var(--lp-border)', marginTop: 48 }}>
          <p style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 13, letterSpacing: 4, color: '#9A7B5A', marginBottom: 8, marginTop: 48 }}>SELF-HOST THE DASHBOARD</p>
          <h2 style={{ ...S.sectionTitle, marginBottom: 6 }}>Install in minutes.</h2>
          <p style={{ ...S.sectionSub, marginBottom: 40 }}>A Go binary + a Postgres database. One Docker Compose file brings it all up.</p>

          <div style={{ display: 'flex', flexDirection: 'column' as const, gap: 0 }}>

            {/* Step 01 */}
            <div style={{ display: 'flex', gap: 28, paddingBottom: 36, borderBottom: '1px solid var(--lp-border)' }} className="lp-install-step">
              <div style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 52, color: '#D4B896', lineHeight: 1, flexShrink: 0, width: 52 }} className="lp-install-num">01</div>
              <div style={{ flex: 1 }}>
                <div style={{ fontFamily: "'Playfair Display', Georgia, serif", fontWeight: 700, fontSize: 18, color: 'var(--lp-text)', marginBottom: 8 }}>Generate an API key</div>
                <p style={{ fontSize: 14, color: 'var(--lp-text-mid)', lineHeight: 1.85, marginBottom: 14 }}>RunRight authenticates the agent to the backend with a single shared secret. Generate one now and keep it in your CI secrets.</p>
                <CopyBlock label="" code={`export RUNRIGHT_API_KEY=$(openssl rand -hex 32)`} />
              </div>
            </div>

            {/* Step 02 */}
            <div style={{ display: 'flex', gap: 28, paddingTop: 36, paddingBottom: 36, borderBottom: '1px solid var(--lp-border)' }} className="lp-install-step">
              <div style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 52, color: '#D4B896', lineHeight: 1, flexShrink: 0, width: 52 }} className="lp-install-num">02</div>
              <div style={{ flex: 1 }}>
                <div style={{ fontFamily: "'Playfair Display', Georgia, serif", fontWeight: 700, fontSize: 18, color: 'var(--lp-text)', marginBottom: 8 }}>Start everything with Docker</div>
                <p style={{ fontSize: 14, color: 'var(--lp-text-mid)', lineHeight: 1.85, marginBottom: 14 }}>One command starts the backend, dashboard, and Postgres. The dashboard is available at <code style={{ fontFamily: 'monospace', background: 'var(--lp-paper-alt)', padding: '1px 4px', fontSize: 12 }}>localhost:3000</code> once it's up.</p>
                <CopyBlock label="" code={`docker compose up -d`} />
              </div>
            </div>

            {/* Step 03 */}
            <div style={{ display: 'flex', gap: 28, paddingTop: 36 }} className="lp-install-step">
              <div style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 52, color: '#D4B896', lineHeight: 1, flexShrink: 0, width: 52 }} className="lp-install-num">03</div>
              <div style={{ flex: 1 }}>
                <div style={{ fontFamily: "'Playfair Display', Georgia, serif", fontWeight: 700, fontSize: 18, color: 'var(--lp-text)', marginBottom: 8 }}>Add the step to your CI workflow</div>
                <p style={{ fontSize: 14, color: 'var(--lp-text-mid)', lineHeight: 1.85, marginBottom: 14 }}>Drop the RunRight step into any job above. It runs as a background process, samples every few seconds, and posts a summary when the job finishes. Your existing steps are untouched.</p>
                <CopyBlock label=".github/workflows/ci.yml" code={`- uses: gbudjeakp/run-right@v1\n  with:\n    run: make build\n    export: file,http\n    http-url: \${{ vars.RUNRIGHT_URL }}\n  env:\n    RUNRIGHT_API_KEY: \${{ secrets.RUNRIGHT_API_KEY }}`} />
              </div>
            </div>

          </div>
        </div>
      </div>

      {/* Compare teaser */}
      <div style={{ borderTop: '1px solid var(--lp-border)', padding: '48px 60px', textAlign: 'center' as const }}>
        <p style={{ fontSize: 15, color: 'var(--lp-text-mid)', margin: '0 0 14px', lineHeight: 1.7 }}>
          How does RunRight compare to Datadog, Grafana, Sentry, and AWS Cost Tools?
        </p>
        <Link
          to="/compare"
          style={{
            fontFamily: "'Bebas Neue', Impact, sans-serif",
            fontSize: 13,
            letterSpacing: 2,
            color: 'var(--lp-text)',
            textDecoration: 'none',
            borderBottom: '1px solid var(--lp-border)',
            paddingBottom: 2,
          }}
        >
          See the full comparison →
        </Link>
      </div>

      {/* Savings Calculator */}
      <SavingsCalc />

      {/* Free Forever / Pricing */}
      <div style={{ background: 'var(--lp-dark-strip)', padding: '52px 60px', textAlign: 'center' as const }}>
        <p style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 12, letterSpacing: 4, color: 'rgba(251,240,220,.4)', margin: '0 0 12px' }}>PRICING</p>
        <div style={{ fontFamily: "'Playfair Display', Georgia, serif", fontWeight: 900, fontSize: 'clamp(36px, 5vw, 52px)', color: '#FBF0DC', lineHeight: 1, marginBottom: 14 }}>It's free.</div>
        <p style={{ fontSize: 15, color: 'rgba(251,240,220,.6)', margin: '0 0 28px', lineHeight: 1.7 }}>No seats, no usage caps, no SaaS. Self-hosted on your infrastructure.</p>
        <Link
          to="/pricing"
          style={{
            fontFamily: "'Bebas Neue', Impact, sans-serif",
            fontSize: 13,
            letterSpacing: 2,
            color: '#FBF0DC',
            textDecoration: 'none',
            borderBottom: '1px solid rgba(251,240,220,.3)',
            paddingBottom: 2,
          }}
        >
          See full pricing →
        </Link>
      </div>

      {/* FAQ */}
      <div style={{ ...S.section, paddingTop: 64, paddingBottom: 64 }} className="lp-section">
        <h2 style={{ ...S.sectionTitle, marginBottom: 36 }}>Quick answers</h2>
        <div style={{ maxWidth: 680, margin: '0 auto', display: 'flex', flexDirection: 'column' as const, gap: 0 }}>
          {[
            {
              q: 'Does RunRight slow down my builds?',
              a: 'No. It polls /proc every 5s as a background sidecar. Under 5 MB RSS, under 0.1% CPU on the sampler. Your build time is not affected.',
            },
            {
              q: 'Do I need to change my application code?',
              a: 'No. RunRight wraps your existing CI step. No SDK, no annotations, no config file.',
            },
            {
              q: 'I already use Datadog. Do I need the RunRight backend?',
              a: 'No. Use --export otlp with OTEL_EXPORTER_OTLP_ENDPOINT pointing at your existing collector. The backend and Postgres are only needed for the self-hosted RunRight dashboard.',
            },
          ].map((item, i, arr) => (
            <div key={item.q} style={{
              padding: '22px 0',
              borderBottom: i < arr.length - 1 ? '1px solid var(--lp-border)' : 'none',
            }}>
              <div style={{ fontFamily: "'Lato', sans-serif", fontWeight: 700, fontSize: 15, color: 'var(--lp-text)', marginBottom: 6 }}>{item.q}</div>
              <p style={{ fontSize: 14, color: 'var(--lp-text-mid)', lineHeight: 1.8, margin: 0 }}>{item.a}</p>
            </div>
          ))}
        </div>
      </div>

      {/* CTA */}
      <div style={S.ctaSection} className="lp-cta">
        <p style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 12, letterSpacing: 4, color: '#9A7B5A', marginBottom: 16 }}>
          READY TO START SAVING?
        </p>
        <h2 style={{ ...S.sectionTitle, marginBottom: 28 }}>
          Right-size your CI today.
        </h2>
        <button
          style={S.heroBtn}
          onClick={onEnter}
          onMouseEnter={e => { (e.currentTarget as HTMLElement).style.background = '#9B2D17'; (e.currentTarget as HTMLElement).style.transform = 'translate(2px,2px)'; (e.currentTarget as HTMLElement).style.boxShadow = '2px 2px 0 rgba(92,58,30,.2)' }}
          onMouseLeave={e => { (e.currentTarget as HTMLElement).style.background = '#C23B22'; (e.currentTarget as HTMLElement).style.transform = 'none'; (e.currentTarget as HTMLElement).style.boxShadow = '4px 4px 0 rgba(92,58,30,.2)' }}
        >
          Open Dashboard →
        </button>
      </div>

      {/* Footer */}
      <footer style={S.footer} className="lp-footer">
        <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}><LogoMark size={16} color="#9A7B5A" /> RUNRIGHT</span>
        <span>BUILT BY <a href="https://github.com/gbudjeakp" target="_blank" rel="noopener noreferrer" style={{ color: '#9A7B5A', textDecoration: 'none', fontFamily: "'Bebas Neue', Impact, sans-serif", letterSpacing: 1 }} onMouseEnter={e => (e.currentTarget.style.color = '#C23B22')} onMouseLeave={e => (e.currentTarget.style.color = '#9A7B5A')}>SEBASTIAN GBUDJE</a></span>
        <span>MIT LICENSE</span>
        <a
          href="https://github.com/gbudjeakp/run-right"
          target="_blank"
          rel="noopener noreferrer"
          aria-label="RunRight on GitHub"
          style={{ color: '#9A7B5A', textDecoration: 'none', letterSpacing: 2, display: 'flex', alignItems: 'center', gap: 7 }}
          onMouseEnter={e => (e.currentTarget.style.color = '#C23B22')}
          onMouseLeave={e => (e.currentTarget.style.color = '#9A7B5A')}
        >
          <GitHubIcon size={14} color="currentColor" />
          GITHUB
        </a>
      </footer>
    </div>
  )
}
