import { useState, useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import './LandingPage.css'
import './InstallPage.css'

// ── Copy block ────────────────────────────────────────────────────────────────
function CopyBlock({ label, code }: { label: string; code: string }) {
  const [copied, setCopied] = useState(false)
  function copy() {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }
  return (
    <div className="ip-code-block">
      <div className="ip-code-header">
        <span className="ip-code-label">{label}</span>
        <button className={`ip-copy-btn${copied ? ' copied' : ''}`} onClick={copy} aria-label={copied ? 'Copied' : 'Copy'}>
          {copied ? '✓ Copied' : 'Copy'}
        </button>
      </div>
      <pre className="ip-pre">{code}</pre>
    </div>
  )
}

// ── Callout ───────────────────────────────────────────────────────────────────
function Callout({ type, children }: { type: 'info' | 'warn' | 'tip'; children: React.ReactNode }) {
  const cls = { info: 'ip-callout-info', warn: 'ip-callout-warn', tip: 'ip-callout-tip' }
  const labels = { info: 'Note', warn: 'Important', tip: 'Tip' }
  return (
    <div className={`ip-callout ${cls[type]}`}>
      <span className="ip-callout-label">{labels[type]}</span>
      {children}
    </div>
  )
}

// ── Simple table ──────────────────────────────────────────────────────────────
function EnvTable({ rows }: { rows: { name: string; where: string; required: string; description: string }[] }) {
  return (
    <div className="ip-table-wrap">
      <table className="ip-table">
        <thead>
          <tr>{['Variable', 'Where', 'Required', 'Description'].map(h => <th key={h}>{h}</th>)}</tr>
        </thead>
        <tbody>
          {rows.map(r => (
            <tr key={r.name}>
              <td><code>{r.name}</code></td>
              <td>{r.where}</td>
              <td className={r.required === 'No' ? 'ip-ok' : 'ip-warn-text'}>{r.required}</td>
              <td>{r.description}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ── Sidebar nav data ──────────────────────────────────────────────────────────
const NAV = [
  {
    group: 'Getting Started',
    items: [
      { id: 'overview',       label: 'Overview' },
      { id: 'no-backend',     label: 'Quick Start (file-only)' },
    ],
  },
  {
    group: 'CI Platforms',
    items: [
      { id: 'github-actions', label: 'GitHub Actions' },
      { id: 'gitlab-ci',      label: 'GitLab CI' },
      { id: 'jenkins',        label: 'Jenkins' },
    ],
  },
  {
    group: 'Infrastructure',
    items: [
      { id: 'kubernetes',     label: 'Kubernetes / Self-Hosted' },
    ],
  },
  {
    group: 'Backend',
    items: [
      { id: 'backend-setup',  label: 'Self-Hosting the Backend' },
      { id: 'api-key',        label: 'API Key' },
    ],
  },
  {
    group: 'Reference',
    items: [
      { id: 'env-vars',       label: 'Environment Variables' },
      { id: 'cli',            label: 'CLI Reference' },
    ],
  },
]

// ── Main page ─────────────────────────────────────────────────────────────────
export default function InstallPage() {
  const [dark, setDark] = useState(false)
  const [activeId, setActiveId] = useState('overview')
  const contentRef = useRef<HTMLDivElement>(null)

  // Scroll-spy: update active nav item as user scrolls
  useEffect(() => {
    const allIds = NAV.flatMap(g => g.items.map(i => i.id))
    const observer = new IntersectionObserver(
      entries => {
        // pick the topmost visible section
        const visible = entries.filter(e => e.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
        if (visible.length > 0) setActiveId(visible[0].target.id)
      },
      { rootMargin: '-20% 0px -60% 0px', threshold: 0 }
    )
    allIds.forEach(id => {
      const el = document.getElementById(id)
      if (el) observer.observe(el)
    })
    return () => observer.disconnect()
  }, [])

  return (
    <div className={`lp-root${dark ? ' lp-dark' : ''} ip-root`}>

      {/* ── Top nav ── */}
      <header className="ip-topnav lp-nav">
        <Link to="/" className="ip-logo">RUNRIGHT</Link>
        <div className="ip-topnav-right">
          <Link to="/" className="ip-topnav-link">Home</Link>
          <a href="https://github.com/gbudjeakp/run-right" target="_blank" rel="noopener noreferrer" className="ip-topnav-link">GitHub</a>
          <button
            className="lp-theme-toggle ip-theme-btn"
            onClick={() => setDark(d => !d)}
            aria-label={dark ? 'Light mode' : 'Dark mode'}
          >{dark ? '☀' : '☽'}</button>
        </div>
      </header>

      {/* ── Body (sidebar + content) ── */}
      <div className="ip-body">

        {/* Sidebar */}
        <aside className="ip-sidebar">
          <p className="ip-sidebar-eyebrow">Documentation</p>
          {NAV.map(group => (
            <div key={group.group} className="ip-nav-group">
              <p className="ip-nav-group-label">{group.group}</p>
              {group.items.map(item => (
                <a
                  key={item.id}
                  href={`#${item.id}`}
                  className={`ip-nav-item${activeId === item.id ? ' active' : ''}`}
                >
                  {item.label}
                </a>
              ))}
            </div>
          ))}
        </aside>

        {/* Main content */}
        <main className="ip-content" ref={contentRef}>

          {/* ── Overview ─────────────────────────────────────────────────── */}
          <section id="overview" className="ip-section">
            <p className="ip-eyebrow">Documentation</p>
            <h1 className="ip-h1">Install RunRight</h1>
            <p className="ip-lead">
              Drop one step into any CI workflow. RunRight monitors your job in the background,
              detects the machine it ran on, and recommends a right-sized alternative — saving
              you money without guesswork.
            </p>
            <p className="ip-lead" style={{ marginTop: 0 }}>
              No backend or API key needed to start. Just add one step, and recommendations
              appear in your job log and step summary.
            </p>
            <div className="ip-overview-grid">
              {[
                { title: '2 min setup', body: 'One action step. No sidecar, no DaemonSet, no infra changes.' },
                { title: 'No backend required', body: 'Recommendations work with file export alone. Backend is optional.' },
                { title: 'API key optional', body: 'Auth is disabled by default. Only enable it when you self-host.' },
                { title: 'Any runner', body: 'GitHub-hosted, self-hosted, GitLab, and Kubernetes runners are all supported.' },
              ].map(c => (
                <div key={c.title} className="ip-overview-card">
                  <strong className="ip-overview-title">{c.title}</strong>
                  <p className="ip-overview-body">{c.body}</p>
                </div>
              ))}
            </div>
          </section>

          {/* ── Quick Start ──────────────────────────────────────────────── */}
          <section id="no-backend" className="ip-section">
            <h2 className="ip-h2">Quick Start — No Backend Needed</h2>
            <p className="ip-p">
              The fastest path: <strong>file export only</strong>. No server, no database, no
              API key. The agent writes a <code>metrics-summary.json</code> artifact and prints
              recommendations directly to the job log and step summary.
            </p>

            <h3 className="ip-h3">Wrapper mode (recommended)</h3>
            <p className="ip-p">Wrap any command with <code>run:</code> — RunRight monitors it start to finish.</p>
            <CopyBlock label=".github/workflows/ci.yml" code={`- uses: gbudjeakp/run-right@v1
  with:
    run: make build        # ← your command here
    # export defaults to "file" — no backend needed`} />

            <h3 className="ip-h3">Standalone mode</h3>
            <p className="ip-p">Use <code>step: start</code> / <code>step: stop</code> to span multiple steps.</p>
            <CopyBlock label=".github/workflows/ci.yml" code={`- uses: gbudjeakp/run-right@v1
  with:
    step: start

- run: make build
- run: make test
- run: make lint

- uses: gbudjeakp/run-right@v1
  with:
    step: stop`} />

            <Callout type="tip">
              Recommendations appear in the <strong>Job Summary</strong> tab of your GitHub Actions
              run — no extra setup required.
            </Callout>
          </section>

          {/* ── GitHub Actions ───────────────────────────────────────────── */}
          <section id="github-actions" className="ip-section">
            <h2 className="ip-h2">GitHub Actions</h2>
            <p className="ip-p">All inputs and their defaults:</p>
            <CopyBlock label=".github/workflows/ci.yml" code={`- uses: gbudjeakp/run-right@v1
  with:
    # ── Mode (pick one) ───────────────────────────────────
    run: ""                # wrap a single command
    step: ""               # "start" or "stop" for multi-step

    # ── Agent settings ────────────────────────────────────
    interval: "5s"         # metrics sampling interval
    duration: "0"          # max run time (0 = unlimited)
    job-id: "\${{ github.run_id }}-\${{ github.run_attempt }}"

    # ── Export ────────────────────────────────────────────
    export: "file"         # file | http | file,http | otlp | prometheus
    output-dir: "\${{ github.workspace }}/.runright"
    upload-artifact: "true"

    # ── Backend (only needed when export includes "http") ─
    http-url: ""           # e.g. https://runright.yourcompany.com
    pr-comment: "true"     # post recommendations as a PR comment
    github-token: "\${{ github.token }}"
  env:
    RUNRIGHT_API_KEY: \${{ secrets.RUNRIGHT_API_KEY }}  # only when backend has auth enabled`} />

            <h3 className="ip-h3">Using action outputs</h3>
            <p className="ip-p">Consume results in downstream steps:</p>
            <CopyBlock label=".github/workflows/ci.yml" code={`- uses: gbudjeakp/run-right@v1
  id: sizing
  with:
    run: make build
    export: file,http
    http-url: \${{ vars.RUNRIGHT_URL }}

- run: echo "Suggested: \${{ steps.sizing.outputs.suggested-machine }}"
- run: echo "Detected:  \${{ steps.sizing.outputs.detected-machine }}"
- run: |
    echo '\${{ steps.sizing.outputs.recommendation-json }}' | jq '.[0]'`} />
          </section>

          {/* ── GitLab CI ────────────────────────────────────────────────── */}
          <section id="gitlab-ci" className="ip-section">
            <h2 className="ip-h2">GitLab CI</h2>
            <p className="ip-p">
              Install the binary in <code>before_script</code> and send SIGTERM in
              <code> after_script</code> — which always runs even when the job fails,
              so data is captured on OOM kills and runner disconnects too.
            </p>
            <CopyBlock label=".gitlab-ci.yml" code={`variables:
  RUNRIGHT_URL: "\${RUNRIGHT_URL}"   # set in CI/CD Variables

build:
  before_script:
    - curl -fsSL https://github.com/gbudjeakp/run-right/releases/latest/download/runright_linux_amd64 \\
        -o /usr/local/bin/runright && chmod +x /usr/local/bin/runright
    - mkdir -p .runright
    - runright monitor --export file,http --http-url "\$RUNRIGHT_URL" \\
        --output-dir .runright --job-id "\$CI_JOB_NAME-\$CI_PIPELINE_ID" &
    - echo \$! > .runright/monitor.pid

  script:
    - make build

  after_script:
    - kill \$(cat .runright/monitor.pid 2>/dev/null) 2>/dev/null || true

  artifacts:
    paths: [.runright/]
    expire_in: 30 days`} />

            <Callout type="info">
              Set <code>RUNRIGHT_URL</code> and optionally <code>RUNRIGHT_API_KEY</code> as
              masked variables in <strong>Settings → CI/CD → Variables</strong>.
            </Callout>
          </section>

          {/* ── Jenkins ────────────────────────────────────────────── */}
          <section id="jenkins" className="ip-section">
            <h2 className="ip-h2">Jenkins</h2>
            <p className="ip-p">
              Use Jenkins credentials bindings to inject <code>RUNRIGHT_URL</code> and
              <code> RUNRIGHT_API_KEY</code>, then run the monitor as a background process
              inside a <code>sh</code> block. Since Jenkins doesn't have an
              <code> after_script</code>, kill the monitor at the end of the same shell block.
            </p>
            <CopyBlock label="Jenkinsfile" code={`pipeline {
  agent any
  environment {
    RUNRIGHT_URL     = credentials('runright-url')
    RUNRIGHT_API_KEY = credentials('runright-api-key')
  }
  stages {
    stage('Build') {
      steps {
        sh """
          curl -fsSL "https://github.com/gbudjeakp/run-right/releases/latest/download/runright_linux_amd64" \\
              -o runright && chmod +x runright
          ./runright monitor --export http --http-url \\$RUNRIGHT_URL &
          echo \\$! > .runright.pid
          make build
          kill \\$(cat .runright.pid) 2>/dev/null || true
        """
      }
    }
  }
}`} />
            <Callout type="info">
              Add credentials in <strong>Manage Jenkins → Credentials</strong> as
              "Secret text" entries with IDs <code>runright-url</code> and
              <code> runright-api-key</code>.
            </Callout>
          </section>

          {/* ── Kubernetes ───────────────────────────────────────────────── */}
          <section id="kubernetes" className="ip-section">
            <h2 className="ip-h2">Kubernetes / Self-Hosted Runners</h2>
            <p className="ip-p">
              RunRight runs <em>inside</em> your CI job — not as a DaemonSet or sidecar.
              It auto-detects CPU and memory limits from <strong>cgroup v2</strong> with no
              extra config on any K8s-hosted runner.
            </p>

            <h3 className="ip-h3">GitHub Actions self-hosted on K8s</h3>
            <CopyBlock label=".github/workflows/ci.yml" code={`jobs:
  build:
    runs-on: self-hosted
    steps:
      - uses: actions/checkout@v4
      - uses: gbudjeakp/run-right@v1
        with:
          run: make build
          export: file,http
          http-url: \${{ vars.RUNRIGHT_URL }}
        env:
          RUNRIGHT_API_KEY: \${{ secrets.RUNRIGHT_API_KEY }}`} />

            <h3 className="ip-h3">Optional: Downward API for guaranteed limit detection</h3>
            <p className="ip-p">
              If cgroup namespacing hides the container limits, inject them explicitly via
              <code> RUNRIGHT_VCPUS</code> and <code>RUNRIGHT_MEMORY_GIB</code>:
            </p>
            <CopyBlock label="runner-pod.yaml (spec.containers[].env)" code={`env:
  - name: RUNRIGHT_VCPUS
    valueFrom:
      resourceFieldRef:
        resource: limits.cpu
  - name: RUNRIGHT_MEMORY_GIB
    valueFrom:
      resourceFieldRef:
        resource: limits.memory
        divisor: "1Gi"
  - name: RUNRIGHT_API_KEY
    valueFrom:
      secretKeyRef:
        name: runright-secrets
        key: api-key`} />

            <h3 className="ip-h3">GitLab Runner on K8s</h3>
            <CopyBlock label=".gitlab-ci.yml" code={`build:
  variables:
    RUNRIGHT_API_KEY: \$RUNRIGHT_API_KEY   # from K8s secret via CI variable
  before_script:
    - curl -fsSL ...runright_linux_amd64 -o runright && chmod +x runright
    - ./runright monitor --export http --http-url "\$RUNRIGHT_URL" &
    - echo \$! > .pid
  script:
    - make build
  after_script:
    - kill \$(cat .pid 2>/dev/null) 2>/dev/null || true`} />
          </section>

          {/* ── Backend Setup ────────────────────────────────────────────── */}
          <section id="backend-setup" className="ip-section">
            <h2 className="ip-h2">Self-Hosting the Backend</h2>
            <p className="ip-p">
              The backend is the recommended way to use RunRight at team scale. It gives you
              persistent job history, a shared dashboard, trend charts, and PR comment
              recommendations that update on every push. It's a single Go binary + Postgres —
              spin it up with Docker Compose.
            </p>
            <Callout type="tip">
              Self-hosting takes about two minutes with Docker Compose. Once running, point
              every workflow at it with <code>http-url</code> and all job data flows to a
              single place your whole team can view.
            </Callout>
            <CopyBlock label="docker-compose.yml" code={`services:
  runright:
    image: ghcr.io/gbudjeakp/run-right:latest
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgres://runright:runright@db:5432/runright?sslmode=disable
      RUNRIGHT_API_KEY: \${RUNRIGHT_API_KEY:-}   # leave unset → auth disabled
    depends_on:
      db: { condition: service_healthy }

  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB:       runright
      POSTGRES_USER:     runright
      POSTGRES_PASSWORD: runright
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U runright"]
      interval: 5s
      retries: 10

  dashboard:
    image: ghcr.io/gbudjeakp/run-right-dashboard:latest
    ports:
      - "3000:3000"

volumes:
  pgdata:`} />
            <CopyBlock label="terminal" code={`docker compose up -d

# optional: seed with demo data
go run ./cmd/seed/ --url http://localhost:8080`} />
          </section>

          {/* ── API Key ──────────────────────────────────────────────────── */}
          <section id="api-key" className="ip-section">
            <h2 className="ip-h2">API Key — When You Need It</h2>
            <p className="ip-p">
              The API key is a <em>server-side guard</em>. It has nothing to do with getting
              recommendations — you get full machine sizing output without ever setting one.
              It only matters when you self-host the backend and want to restrict who can
              write to your dashboard.
            </p>

            <div className="ip-table-wrap">
              <table className="ip-table">
                <thead>
                  <tr>
                    <th>Scenario</th><th>Need API key?</th><th>Why</th>
                  </tr>
                </thead>
                <tbody>
                  {[
                    ['File export only (no backend)', 'No', 'Data stays local — nothing authenticates'],
                    ['Backend, RUNRIGHT_API_KEY unset', 'No', 'Auth is disabled — dev mode'],
                    ['Backend, RUNRIGHT_API_KEY set', 'Yes — set the same key in CI', 'Agent sends it as Authorization: Bearer'],
                    ['Dashboard login', 'Yes — if backend has a key', 'Login screen checks it against the server'],
                  ].map(([s, n, w]) => (
                    <tr key={s}>
                      <td>{s}</td>
                      <td className={n.startsWith('No') ? 'ip-ok' : 'ip-warn-text'} style={{ whiteSpace: 'nowrap', fontWeight: 600 }}>{n}</td>
                      <td>{w}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <p className="ip-p" style={{ marginTop: 20 }}>
              The agent reads <code>RUNRIGHT_API_KEY</code> from the environment automatically — no flag needed.
            </p>
            <CopyBlock label=".github/workflows/ci.yml" code={`# Settings → Secrets → Actions → New repository secret
# Name: RUNRIGHT_API_KEY   Value: your-secret-here

- uses: gbudjeakp/run-right@v1
  with:
    run: make build
    export: file,http
    http-url: \${{ vars.RUNRIGHT_URL }}
  env:
    RUNRIGHT_API_KEY: \${{ secrets.RUNRIGHT_API_KEY }}`} />

            <Callout type="warn">
              Treat the API key like a password. Use your platform's secret store (GitHub Secrets,
              GitLab Variables, K8s Secrets) and never commit it to source control.
            </Callout>
          </section>

          {/* ── Env Vars ─────────────────────────────────────────────────── */}
          <section id="env-vars" className="ip-section">
            <h2 className="ip-h2">Environment Variables</h2>
            <EnvTable rows={[
              { name: 'RUNRIGHT_API_KEY',       where: 'Server + CI agent',  required: 'No',           description: 'Shared secret for auth. Set on the server to require auth; set in CI so the agent can post results. If unset everywhere, auth is disabled.' },
              { name: 'DATABASE_URL',           where: 'Server only',        required: 'Yes (server)', description: 'Postgres DSN. Example: postgres://runright:runright@localhost:5432/runright?sslmode=disable' },
              { name: 'RUNRIGHT_VCPUS',         where: 'CI agent (K8s)',     required: 'No',           description: 'vCPU count for machine detection. Inject via Downward API when cgroup detection is unreliable.' },
              { name: 'RUNRIGHT_MEMORY_GIB',    where: 'CI agent (K8s)',     required: 'No',           description: 'Memory limit in GiB for machine detection. Inject via Downward API. Overrides cgroup-based detection when set.' },
              { name: 'OTEL_EXPORTER_OTLP_ENDPOINT', where: 'CI agent',     required: 'No',           description: 'OTLP collector endpoint for the otlp export backend. Example: http://localhost:4317' },
            ]} />
          </section>

          {/* ── CLI Reference ────────────────────────────────────────────── */}
          <section id="cli" className="ip-section">
            <h2 className="ip-h2">CLI Reference</h2>

            <h3 className="ip-h3">runright monitor</h3>
            <CopyBlock label="flags" code={`runright monitor [flags]

  --export           string     Export backends, comma-separated: file,http,otlp,prometheus (default "file")
  --http-url         string     Backend base URL for http export
  --interval         duration   Sampling interval (default 5s)
  --duration         duration   Stop after this duration (0 = run until SIGTERM/SIGINT)
  --job-id           string     Job identifier (default: timestamp-based ID)
  --output-dir       string     Directory for file output (default ".")
  --prometheus-port  int        Port for Prometheus /metrics endpoint (default 9090)`} />

            <h3 className="ip-h3">runright recommend</h3>
            <CopyBlock label="flags" code={`runright recommend [flags]

  --metrics   string   Path to metrics-summary.json (default "metrics-summary.json")
  --provider  string   Filter by provider: aws, gcp, or github (default: all)
  --format    string   Output format: table, json, or markdown (default "table")`} />

            <h3 className="ip-h3">runright serve</h3>
            <CopyBlock label="flags" code={`runright serve [flags]

  --port  int   HTTP port (default 8080)`} />
          </section>

        </main>
      </div>

      {/* Footer */}
      <footer className="ip-footer lp-footer">
        <span>RUNRIGHT</span>
        <div style={{ display: 'flex', gap: 24 }}>
          <Link to="/" className="ip-footer-link">Home</Link>
          <a href="https://github.com/gbudjeakp/run-right" target="_blank" rel="noopener noreferrer" className="ip-footer-link">GitHub</a>
        </div>
      </footer>
    </div>
  )
}
