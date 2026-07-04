import { useState, useEffect } from 'react'
import { fetchSSOConfigs, upsertSSOConfig, deleteSSOConfig, testSSOConfig } from '../api'
import type { SSOConfig, SSOProviderType } from '../types'

const PROVIDER_OPTIONS: { value: SSOProviderType; label: string; description: string }[] = [
  { value: 'google', label: 'Google', description: 'Google Workspace / Gmail accounts' },
  { value: 'github', label: 'GitHub', description: 'GitHub OAuth' },
  { value: 'azuread', label: 'Azure AD', description: 'Microsoft Entra ID (Azure AD)' },
  { value: 'okta', label: 'Okta', description: 'Okta OIDC' },
  { value: 'oidc', label: 'Generic OIDC', description: 'Any OpenID Connect provider' },
  { value: 'saml', label: 'SAML 2.0', description: 'Enterprise SAML identity provider' },
]

const ROLE_OPTIONS = [
  { value: 'viewer', label: 'Viewer', description: 'Read-only access' },
  { value: 'admin', label: 'Admin', description: 'Full access' },
]

export default function SSOSettingsPage() {
  const [configs, setConfigs] = useState<SSOConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<Partial<SSOConfig> | null>(null)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ valid: boolean; message?: string } | null>(null)

  useEffect(() => {
    loadConfigs()
  }, [])

  async function loadConfigs() {
    try {
      const data = await fetchSSOConfigs()
      setConfigs(data)
    } catch {
      setError('Failed to load SSO configurations')
    } finally {
      setLoading(false)
    }
  }

  function startNew() {
    setEditing({
      provider_type: 'google',
      name: '',
      enabled: false,
      client_id: '',
      client_secret: '',
      issuer_url: '',
      scopes: 'email,profile',
      allowed_domains: '',
      default_role: 'viewer',
    })
    setTestResult(null)
  }

  function startEdit(config: SSOConfig) {
    setEditing({ ...config })
    setTestResult(null)
  }

  async function handleSave() {
    if (!editing) return
    setSaving(true)
    setError('')
    try {
      await upsertSSOConfig(editing)
      await loadConfigs()
      setEditing(null)
    } catch {
      setError('Failed to save configuration')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: number) {
    if (!confirm('Are you sure you want to delete this SSO provider?')) return
    try {
      await deleteSSOConfig(id)
      await loadConfigs()
    } catch {
      setError('Failed to delete configuration')
    }
  }

  async function handleTest() {
    if (!editing) return
    setTesting(true)
    setTestResult(null)
    try {
      const result = await testSSOConfig(editing)
      setTestResult(result)
    } catch {
      setTestResult({ valid: false, message: 'Test failed - check your configuration' })
    } finally {
      setTesting(false)
    }
  }

  const isSAML = editing?.provider_type === 'saml'
  const needsIssuerURL = ['okta', 'oidc'].includes(editing?.provider_type ?? '')

  return (
    <div className="fadein">
      <div className="flex items-center justify-between mb-7">
        <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] tracking-tight">
          SSO Configuration
        </h1>
        {!editing && (
          <button onClick={startNew} className="btn-rr">
            Add Provider
          </button>
        )}
      </div>

      {error && (
        <div className="bg-red/10 border border-red text-red px-4 py-3 mb-5 text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <p className="text-sm text-[var(--text-light)]">Loading SSO configurations...</p>
      ) : editing ? (
        /* Edit Form */
        <div className="rr-card">
          <h2 className="font-serif text-lg font-bold text-[var(--text)] mb-5">
            {editing.id ? 'Edit Provider' : 'New SSO Provider'}
          </h2>

          <div className="space-y-5">
            {/* Provider Type */}
            <div className="form-group">
              <label>Provider Type</label>
              <select
                className="rr-select"
                value={editing.provider_type}
                onChange={e => setEditing({ ...editing, provider_type: e.target.value as SSOProviderType })}
                disabled={!!editing.id}
              >
                {PROVIDER_OPTIONS.map(opt => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
              <p className="text-xs text-[var(--text-light)] mt-1">
                {PROVIDER_OPTIONS.find(o => o.value === editing.provider_type)?.description}
              </p>
            </div>

            {/* Display Name */}
            <div className="form-group">
              <label>Display Name</label>
              <input
                type="text"
                className="rr-input"
                placeholder="e.g., Company Google SSO"
                value={editing.name ?? ''}
                onChange={e => setEditing({ ...editing, name: e.target.value })}
              />
            </div>

            {/* Enabled Toggle */}
            <div className="form-group">
              <label className="flex items-center gap-3 cursor-pointer">
                <input
                  type="checkbox"
                  checked={editing.enabled ?? false}
                  onChange={e => setEditing({ ...editing, enabled: e.target.checked })}
                  className="w-4 h-4"
                />
                <span>Enabled</span>
              </label>
              <p className="text-xs text-[var(--text-light)] mt-1">
                When enabled, this provider will appear on the login page
              </p>
            </div>

            {/* OAuth/OIDC Settings */}
            {!isSAML && (
              <>
                <div className="form-group">
                  <label>Client ID</label>
                  <input
                    type="text"
                    className="rr-input"
                    placeholder="OAuth Client ID"
                    value={editing.client_id ?? ''}
                    onChange={e => setEditing({ ...editing, client_id: e.target.value })}
                  />
                </div>

                <div className="form-group">
                  <label>Client Secret</label>
                  <input
                    type="password"
                    className="rr-input"
                    placeholder={editing.id ? '••••••••' : 'OAuth Client Secret'}
                    value={editing.client_secret ?? ''}
                    onChange={e => setEditing({ ...editing, client_secret: e.target.value })}
                  />
                  {editing.id && (
                    <p className="text-xs text-[var(--text-light)] mt-1">
                      Leave blank to keep existing secret
                    </p>
                  )}
                </div>

                {needsIssuerURL && (
                  <div className="form-group">
                    <label>Issuer URL</label>
                    <input
                      type="url"
                      className="rr-input"
                      placeholder="https://your-org.okta.com"
                      value={editing.issuer_url ?? ''}
                      onChange={e => setEditing({ ...editing, issuer_url: e.target.value })}
                    />
                    <p className="text-xs text-[var(--text-light)] mt-1">
                      OIDC discovery will be performed at this URL
                    </p>
                  </div>
                )}

                <div className="form-group">
                  <label>Scopes</label>
                  <input
                    type="text"
                    className="rr-input"
                    placeholder="email,profile,openid"
                    value={editing.scopes ?? ''}
                    onChange={e => setEditing({ ...editing, scopes: e.target.value })}
                  />
                  <p className="text-xs text-[var(--text-light)] mt-1">
                    Comma-separated OAuth scopes to request
                  </p>
                </div>
              </>
            )}

            {/* SAML Settings */}
            {isSAML && (
              <>
                <div className="form-group">
                  <label>IDP Metadata URL</label>
                  <input
                    type="url"
                    className="rr-input"
                    placeholder="https://idp.example.com/metadata"
                    value={editing.idp_metadata_url ?? ''}
                    onChange={e => setEditing({ ...editing, idp_metadata_url: e.target.value })}
                  />
                  <p className="text-xs text-[var(--text-light)] mt-1">
                    URL where the IDP metadata XML can be fetched
                  </p>
                </div>

                <div className="form-group">
                  <label>SP Entity ID</label>
                  <input
                    type="text"
                    className="rr-input"
                    placeholder="https://runright.example.com"
                    value={editing.sp_entity_id ?? ''}
                    onChange={e => setEditing({ ...editing, sp_entity_id: e.target.value })}
                  />
                  <p className="text-xs text-[var(--text-light)] mt-1">
                    Service Provider entity ID (usually your RunRight base URL)
                  </p>
                </div>
              </>
            )}

            {/* Access Control */}
            <div className="border-t border-[var(--border)] pt-5">
              <h3 className="font-deco text-[13px] tracking-[2px] text-[var(--text-mid)] mb-4 uppercase">
                Access Control
              </h3>

              <div className="form-group">
                <label>Allowed Email Domains</label>
                <input
                  type="text"
                  className="rr-input"
                  placeholder="example.com, company.org"
                  value={editing.allowed_domains ?? ''}
                  onChange={e => setEditing({ ...editing, allowed_domains: e.target.value })}
                />
                <p className="text-xs text-[var(--text-light)] mt-1">
                  Comma-separated list of allowed email domains. Leave blank to allow all.
                </p>
              </div>

              <div className="form-group">
                <label>Default Role</label>
                <select
                  className="rr-select"
                  value={editing.default_role ?? 'viewer'}
                  onChange={e => setEditing({ ...editing, default_role: e.target.value })}
                >
                  {ROLE_OPTIONS.map(opt => (
                    <option key={opt.value} value={opt.value}>{opt.label}</option>
                  ))}
                </select>
                <p className="text-xs text-[var(--text-light)] mt-1">
                  Role assigned to new users on first login
                </p>
              </div>
            </div>

            {/* Test Result */}
            {testResult && (
              <div className={`px-4 py-3 text-sm ${testResult.valid ? 'bg-green-50 border border-green-300 text-green-700' : 'bg-red/10 border border-red text-red'}`}>
                {testResult.valid ? '✓ Configuration is valid' : `✗ ${testResult.message}`}
              </div>
            )}

            {/* Actions */}
            <div className="flex items-center gap-3 pt-3">
              <button
                onClick={handleSave}
                disabled={saving || !editing.name}
                className="btn-rr"
              >
                {saving ? 'Saving...' : 'Save'}
              </button>
              <button
                onClick={handleTest}
                disabled={testing || !editing.client_id && !isSAML}
                className="btn-rr-secondary"
              >
                {testing ? 'Testing...' : 'Test Configuration'}
              </button>
              <button
                onClick={() => setEditing(null)}
                className="btn-rr-ghost"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      ) : (
        /* Provider List */
        <div className="space-y-4">
          {configs.length === 0 ? (
            <div className="rr-card text-center py-10">
              <p className="text-[var(--text-light)] mb-4">No SSO providers configured</p>
              <button onClick={startNew} className="btn-rr">
                Add Your First Provider
              </button>
            </div>
          ) : (
            configs.map(config => (
              <div key={config.id} className="rr-card flex items-center justify-between">
                <div>
                  <div className="flex items-center gap-3 mb-1">
                    <span className="font-serif font-bold text-[var(--text)]">{config.name}</span>
                    <span className="text-xs font-deco tracking-wider px-2 py-0.5 bg-[var(--cream-alt)] text-[var(--text-mid)] uppercase">
                      {config.provider_type}
                    </span>
                    {config.enabled ? (
                      <span className="text-xs font-deco tracking-wider px-2 py-0.5 bg-green-100 text-green-700 uppercase">
                        Active
                      </span>
                    ) : (
                      <span className="text-xs font-deco tracking-wider px-2 py-0.5 bg-[var(--border)] text-[var(--text-light)] uppercase">
                        Disabled
                      </span>
                    )}
                  </div>
                  <p className="text-sm text-[var(--text-light)]">
                    {config.allowed_domains ? `Domains: ${config.allowed_domains}` : 'All domains allowed'}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => startEdit(config)}
                    className="btn-rr-secondary text-sm"
                  >
                    Edit
                  </button>
                  <button
                    onClick={() => config.id && handleDelete(config.id)}
                    className="btn-rr-ghost text-sm text-red hover:bg-red/10"
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      )}

      {/* Documentation */}
      <div className="rr-card mt-8">
        <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-3">Setup Guide</h2>
        <div className="text-sm text-[var(--text-light)] leading-relaxed space-y-3">
          <p><strong>Google:</strong> Create OAuth credentials at <a href="https://console.cloud.google.com/apis/credentials" target="_blank" rel="noopener" className="text-red hover:underline">Google Cloud Console</a>. Authorized redirect URI: <code className="bg-[var(--cream-alt)] px-1">{window.location.origin}/api/v1/sso/callback/google</code></p>
          <p><strong>GitHub:</strong> Create an OAuth App at <a href="https://github.com/settings/developers" target="_blank" rel="noopener" className="text-red hover:underline">GitHub Developer Settings</a>. Authorization callback: <code className="bg-[var(--cream-alt)] px-1">{window.location.origin}/api/v1/sso/callback/github</code></p>
          <p><strong>Azure AD:</strong> Register an app at <a href="https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps/ApplicationsListBlade" target="_blank" rel="noopener" className="text-red hover:underline">Azure Portal</a>. Redirect URI: <code className="bg-[var(--cream-alt)] px-1">{window.location.origin}/api/v1/sso/callback/azuread</code></p>
          <p><strong>Okta:</strong> Create an OIDC app in your Okta admin console. Sign-in redirect: <code className="bg-[var(--cream-alt)] px-1">{window.location.origin}/api/v1/sso/callback/okta</code></p>
          <p><strong>SAML:</strong> Requires <code className="bg-[var(--cream-alt)] px-1">RUNRIGHT_SAML_CERT</code> and <code className="bg-[var(--cream-alt)] px-1">RUNRIGHT_SAML_KEY</code> environment variables. ACS URL: <code className="bg-[var(--cream-alt)] px-1">{window.location.origin}/api/v1/sso/callback/saml</code></p>
        </div>
      </div>
    </div>
  )
}
