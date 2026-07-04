import { useState, useEffect, type ReactNode } from 'react'
import { fetchUserSettings, upsertUserSettings, fetchSSOConfigs, upsertSSOConfig, deleteSSOConfig, testSSOConfig, type UserSettings } from '../api'
import { CURRENCY_OPTIONS, type CurrencyCode, useCurrencyPreference } from '../currency'
import type { SSOConfig, SSOProviderType } from '../types'

// Types
interface APIKey {
  id: string
  name: string
  key_prefix: string
  scopes: string[]
  last_used_at?: string
  expires_at?: string
  created_at: string
}

interface Team {
  id: string
  name: string
  slug: string
  member_count: number
  plan: string
}

interface AuditLog {
  id: string
  actor_email: string
  action: string
  resource_type: string
  resource_name?: string
  status: string
  created_at: string
}

interface TeamMember {
  id: string
  user_email: string
  role: string
  joined_at?: string
  invited_at?: string
}

// Tab IDs
type TabId = 'general' | 'sso' | 'api-keys' | 'team' | 'audit'

// Settings Tab Content Components
const PROVIDER_OPTIONS: { value: SSOProviderType; label: string; description: string }[] = [
  { value: 'google', label: 'Google', description: 'Google Workspace / Gmail' },
  { value: 'github', label: 'GitHub', description: 'GitHub OAuth' },
  { value: 'azuread', label: 'Azure AD', description: 'Microsoft Entra ID' },
  { value: 'okta', label: 'Okta', description: 'Okta OIDC' },
  { value: 'oidc', label: 'OIDC', description: 'Generic OpenID Connect' },
  { value: 'saml', label: 'SAML', description: 'Enterprise SAML 2.0' },
]

export default function SettingsPage() {
  const [activeTab, setActiveTab] = useState<TabId>('general')

  const tabs: { id: TabId; label: string; icon: ReactNode }[] = [
    { id: 'general', label: 'General', icon: <SettingsIcon /> },
    { id: 'sso', label: 'SSO', icon: <LockIcon /> },
    { id: 'api-keys', label: 'API Keys', icon: <KeyIcon /> },
    { id: 'team', label: 'Team', icon: <UsersIcon /> },
    { id: 'audit', label: 'Audit Log', icon: <ClipboardIcon /> },
  ]

  return (
    <div className="fadein max-w-5xl">
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-6 tracking-tight">
        Settings
      </h1>

      {/* Tab Navigation */}
      <div className="flex gap-1 border-b border-[var(--border)] mb-6 overflow-x-auto">
        {tabs.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`flex items-center gap-2 px-4 py-3 text-sm font-medium transition-colors whitespace-nowrap border-b-2 -mb-px ${
              activeTab === tab.id
                ? 'border-[var(--red)] text-[var(--text)]'
                : 'border-transparent text-[var(--text-light)] hover:text-[var(--text-mid)]'
            }`}
          >
            <span className="w-4 h-4">{tab.icon}</span>
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tab Content */}
      <div className="animate-fadeIn">
        {activeTab === 'general' && <GeneralTab />}
        {activeTab === 'sso' && <SSOTab />}
        {activeTab === 'api-keys' && <APIKeysTab />}
        {activeTab === 'team' && <TeamTab />}
        {activeTab === 'audit' && <AuditTab />}
      </div>
    </div>
  )
}

// === General Settings Tab ===
function GeneralTab() {
  const { currency, setCurrency } = useCurrencyPreference()
  const [settings, setSettings] = useState<UserSettings>({
    otel_endpoint: '',
    allowed_machine_ids: [],
    allowed_series: [],
    allowed_families: [],
  })
  const [preferredCurrency, setPreferredCurrency] = useState<CurrencyCode>(currency)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    void (async () => {
      try {
        const data = await fetchUserSettings()
        setSettings(data)
      } catch {
        setError('Unable to load settings.')
      } finally {
        setLoading(false)
      }
    })()
  }, [])

  useEffect(() => {
    setPreferredCurrency(currency)
  }, [currency])

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    try {
      await upsertUserSettings(settings)
      setCurrency(preferredCurrency)
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch {
      setError('Unable to save settings.')
    }
  }

  if (loading) return <LoadingState />

  return (
    <div className="space-y-6">
      <Card title="Preferences">
        <form onSubmit={e => void save(e)} className="space-y-5">
          <FormGroup label="Display Currency" hint="Affects all dashboard money values">
            <select
              className="settings-select"
              value={preferredCurrency}
              onChange={e => setPreferredCurrency(e.target.value as CurrencyCode)}
            >
              {CURRENCY_OPTIONS.map(opt => (
                <option key={opt.code} value={opt.code}>{opt.label}</option>
              ))}
            </select>
          </FormGroup>

          <FormGroup label="OpenTelemetry Endpoint" hint="For OTLP metric export">
            <input
              type="text"
              className="settings-input"
              placeholder="http://localhost:4317"
              value={settings.otel_endpoint}
              onChange={e => setSettings({ ...settings, otel_endpoint: e.target.value })}
            />
          </FormGroup>

          {error && <ErrorMessage message={error} />}
          
          <div className="flex items-center gap-4">
            <button type="submit" className="settings-btn-primary">Save Changes</button>
            {saved && <span className="text-sm text-green-600">Saved!</span>}
          </div>
        </form>
      </Card>
    </div>
  )
}

// === SSO Settings Tab ===
function SSOTab() {
  const [configs, setConfigs] = useState<SSOConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<Partial<SSOConfig> | null>(null)
  const [saving, setSaving] = useState(false)

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
      scopes: 'email,profile',
      default_role: 'viewer',
    })
  }

  async function handleSave() {
    if (!editing) return
    setSaving(true)
    try {
      await upsertSSOConfig(editing)
      await loadConfigs()
      setEditing(null)
    } catch {
      setError('Failed to save')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this SSO provider?')) return
    await deleteSSOConfig(id)
    await loadConfigs()
  }

  if (loading) return <LoadingState />

  return (
    <div className="space-y-6">
      {editing ? (
        <Card title={editing.id ? 'Edit Provider' : 'New SSO Provider'}>
          <div className="space-y-5">
            <FormGroup label="Provider Type">
              <select
                className="settings-select"
                value={editing.provider_type}
                onChange={e => setEditing({ ...editing, provider_type: e.target.value as SSOProviderType })}
                disabled={!!editing.id}
              >
                {PROVIDER_OPTIONS.map(opt => (
                  <option key={opt.value} value={opt.value}>{opt.label} - {opt.description}</option>
                ))}
              </select>
            </FormGroup>

            <FormGroup label="Display Name">
              <input
                type="text"
                className="settings-input"
                placeholder="Company SSO"
                value={editing.name ?? ''}
                onChange={e => setEditing({ ...editing, name: e.target.value })}
              />
            </FormGroup>

            <div className="flex items-center gap-3">
              <input
                type="checkbox"
                id="sso-enabled"
                checked={editing.enabled ?? false}
                onChange={e => setEditing({ ...editing, enabled: e.target.checked })}
                className="w-4 h-4 rounded border-[var(--border)]"
              />
              <label htmlFor="sso-enabled" className="text-sm">Enable this provider</label>
            </div>

            {editing.provider_type !== 'saml' && (
              <>
                <div className="grid grid-cols-2 gap-4">
                  <FormGroup label="Client ID">
                    <input
                      type="text"
                      className="settings-input"
                      value={editing.client_id ?? ''}
                      onChange={e => setEditing({ ...editing, client_id: e.target.value })}
                    />
                  </FormGroup>
                  <FormGroup label="Client Secret">
                    <input
                      type="password"
                      className="settings-input"
                      placeholder={editing.id ? '••••••••' : ''}
                      value={editing.client_secret ?? ''}
                      onChange={e => setEditing({ ...editing, client_secret: e.target.value })}
                    />
                  </FormGroup>
                </div>

                {['okta', 'oidc'].includes(editing.provider_type ?? '') && (
                  <FormGroup label="Issuer URL">
                    <input
                      type="url"
                      className="settings-input"
                      placeholder="https://your-org.okta.com"
                      value={editing.issuer_url ?? ''}
                      onChange={e => setEditing({ ...editing, issuer_url: e.target.value })}
                    />
                  </FormGroup>
                )}
              </>
            )}

            {editing.provider_type === 'saml' && (
              <>
                <FormGroup label="IDP Metadata URL">
                  <input
                    type="url"
                    className="settings-input"
                    placeholder="https://idp.example.com/metadata"
                    value={editing.idp_metadata_url ?? ''}
                    onChange={e => setEditing({ ...editing, idp_metadata_url: e.target.value })}
                  />
                </FormGroup>
              </>
            )}

            <FormGroup label="Allowed Email Domains" hint="Comma-separated, leave blank for all">
              <input
                type="text"
                className="settings-input"
                placeholder="company.com, acme.org"
                value={editing.allowed_domains ?? ''}
                onChange={e => setEditing({ ...editing, allowed_domains: e.target.value })}
              />
            </FormGroup>

            {error && <ErrorMessage message={error} />}

            <div className="flex gap-3 pt-2">
              <button onClick={handleSave} disabled={saving} className="settings-btn-primary">
                {saving ? 'Saving...' : 'Save Provider'}
              </button>
              <button onClick={() => setEditing(null)} className="settings-btn-secondary">
                Cancel
              </button>
            </div>
          </div>
        </Card>
      ) : (
        <Card 
          title="Identity Providers" 
          action={<button onClick={startNew} className="settings-btn-primary">Add Provider</button>}
        >
          {configs.length === 0 ? (
            <EmptyState 
              icon={<LockIcon />}
              message="No SSO providers configured"
              action={<button onClick={startNew} className="settings-btn-secondary">Add your first provider</button>}
            />
          ) : (
            <div className="divide-y divide-[var(--border)]">
              {configs.map(config => (
                <div key={config.id} className="py-4 flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div className={`w-2 h-2 rounded-full ${config.enabled ? 'bg-green-500' : 'bg-gray-300'}`} />
                    <div>
                      <div className="font-medium text-[var(--text)]">{config.name}</div>
                      <div className="text-sm text-[var(--text-light)]">
                        {PROVIDER_OPTIONS.find(p => p.value === config.provider_type)?.label}
                        {config.allowed_domains && ` · ${config.allowed_domains}`}
                      </div>
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <button onClick={() => setEditing(config)} className="text-sm text-[var(--text-mid)] hover:text-[var(--text)]">Edit</button>
                    <button onClick={() => handleDelete(config.id!)} className="text-sm text-red-500 hover:text-red-700">Delete</button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
      )}
    </div>
  )
}

// === API Keys Tab ===
function APIKeysTab() {
  const [keys, setKeys] = useState<APIKey[]>([])
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [newKey, setNewKey] = useState<{ name: string; expiresIn: string }>({ name: '', expiresIn: '90d' })
  const [createdKey, setCreatedKey] = useState<string | null>(null)

  useEffect(() => {
    loadKeys()
  }, [])

  async function loadKeys() {
    try {
      const res = await fetch('/api/v1/api-keys', { credentials: 'include' })
      const data = await res.json()
      setKeys(data.api_keys || [])
    } catch {
      // Ignore
    } finally {
      setLoading(false)
    }
  }

  async function createKey() {
    if (!newKey.name) return
    setCreating(true)
    try {
      const res = await fetch('/api/v1/api-keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          name: newKey.name,
          expires_in: newKey.expiresIn,
          scopes: ['read', 'write'],
        }),
      })
      const data = await res.json()
      setCreatedKey(data.key)
      setNewKey({ name: '', expiresIn: '90d' })
      await loadKeys()
    } finally {
      setCreating(false)
    }
  }

  async function revokeKey(id: string) {
    if (!confirm('Revoke this API key? This cannot be undone.')) return
    await fetch(`/api/v1/api-keys/${id}`, { method: 'DELETE', credentials: 'include' })
    await loadKeys()
  }

  if (loading) return <LoadingState />

  return (
    <div className="space-y-6">
      {createdKey && (
        <Card title="New API Key Created" className="bg-green-50 border-green-200">
          <div className="space-y-3">
            <p className="text-sm text-green-800">Copy this key now. It won't be shown again.</p>
            <div className="bg-white border border-green-300 rounded px-4 py-3 font-mono text-sm break-all">
              {createdKey}
            </div>
            <button onClick={() => setCreatedKey(null)} className="settings-btn-secondary">Done</button>
          </div>
        </Card>
      )}

      <Card title="Create API Key">
        <div className="flex gap-4 items-end">
          <FormGroup label="Name" className="flex-1">
            <input
              type="text"
              className="settings-input"
              placeholder="My CI Key"
              value={newKey.name}
              onChange={e => setNewKey({ ...newKey, name: e.target.value })}
            />
          </FormGroup>
          <FormGroup label="Expires">
            <select
              className="settings-select"
              value={newKey.expiresIn}
              onChange={e => setNewKey({ ...newKey, expiresIn: e.target.value })}
            >
              <option value="30d">30 days</option>
              <option value="90d">90 days</option>
              <option value="1y">1 year</option>
              <option value="never">Never</option>
            </select>
          </FormGroup>
          <button onClick={createKey} disabled={creating || !newKey.name} className="settings-btn-primary">
            {creating ? 'Creating...' : 'Create Key'}
          </button>
        </div>
      </Card>

      <Card title="Your API Keys">
        {keys.length === 0 ? (
          <EmptyState icon={<KeyIcon />} message="No API keys yet" />
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {keys.map(key => (
              <div key={key.id} className="py-4 flex items-center justify-between">
                <div>
                  <div className="font-medium text-[var(--text)]">{key.name}</div>
                  <div className="text-sm text-[var(--text-light)]">
                    {key.key_prefix}... · Created {new Date(key.created_at).toLocaleDateString()}
                    {key.last_used_at && ` · Last used ${new Date(key.last_used_at).toLocaleDateString()}`}
                  </div>
                </div>
                <button onClick={() => revokeKey(key.id)} className="text-sm text-red-500 hover:text-red-700">
                  Revoke
                </button>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}

// === Team Tab ===
function TeamTab() {
  const [teams, setTeams] = useState<Team[]>([])
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [newTeam, setNewTeam] = useState({ name: '' })
  const [selectedTeam, setSelectedTeam] = useState<Team | null>(null)
  const [members, setMembers] = useState<TeamMember[]>([])
  const [loadingMembers, setLoadingMembers] = useState(false)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState('member')
  const [inviting, setInviting] = useState(false)

  useEffect(() => {
    loadTeams()
  }, [])

  async function loadTeams() {
    try {
      const res = await fetch('/api/v1/teams', { credentials: 'include' })
      const data = await res.json()
      setTeams(data.teams || [])
    } catch {
      // Ignore
    } finally {
      setLoading(false)
    }
  }

  async function createTeam() {
    if (!newTeam.name) return
    setCreating(true)
    try {
      await fetch('/api/v1/teams', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ name: newTeam.name }),
      })
      setNewTeam({ name: '' })
      await loadTeams()
    } finally {
      setCreating(false)
    }
  }

  async function selectTeam(team: Team) {
    setSelectedTeam(team)
    setLoadingMembers(true)
    try {
      const res = await fetch(`/api/v1/teams/${team.id}/members`, { credentials: 'include' })
      const data = await res.json()
      setMembers(data.members || [])
    } catch {
      setMembers([])
    } finally {
      setLoadingMembers(false)
    }
  }

  async function inviteMember() {
    if (!selectedTeam || !inviteEmail) return
    setInviting(true)
    try {
      await fetch(`/api/v1/teams/${selectedTeam.id}/members/invite`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ email: inviteEmail, role: inviteRole }),
      })
      setInviteEmail('')
      // Reload members
      const res = await fetch(`/api/v1/teams/${selectedTeam.id}/members`, { credentials: 'include' })
      const data = await res.json()
      setMembers(data.members || [])
      await loadTeams() // Update member counts
    } finally {
      setInviting(false)
    }
  }

  async function removeMember(memberId: string) {
    if (!selectedTeam || !confirm('Remove this member from the team?')) return
    try {
      await fetch(`/api/v1/teams/${selectedTeam.id}/members/${memberId}`, {
        method: 'DELETE',
        credentials: 'include',
      })
      setMembers(members.filter(m => m.id !== memberId))
      await loadTeams() // Update member counts
    } catch {
      // Ignore
    }
  }

  if (loading) return <LoadingState />

  // Team detail view
  if (selectedTeam) {
    return (
      <div className="space-y-6">
        <button
          onClick={() => setSelectedTeam(null)}
          className="flex items-center gap-2 text-sm text-[var(--text-mid)] hover:text-[var(--text)]"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
          </svg>
          Back to Teams
        </button>

        <Card title={selectedTeam.name} action={
          <span className="text-sm text-[var(--text-light)]">{selectedTeam.plan} plan</span>
        }>
          <div className="space-y-6">
            {/* Invite Member */}
            <div className="pb-4 border-b border-[var(--border)]">
              <div className="text-sm font-medium text-[var(--text)] mb-3">Invite Member</div>
              <div className="flex gap-3">
                <input
                  type="email"
                  className="settings-input flex-1"
                  placeholder="email@example.com"
                  value={inviteEmail}
                  onChange={e => setInviteEmail(e.target.value)}
                />
                <select
                  className="settings-select w-32"
                  value={inviteRole}
                  onChange={e => setInviteRole(e.target.value)}
                >
                  <option value="member">Member</option>
                  <option value="admin">Admin</option>
                  <option value="owner">Owner</option>
                </select>
                <button
                  onClick={inviteMember}
                  disabled={inviting || !inviteEmail}
                  className="settings-btn-primary"
                >
                  {inviting ? 'Inviting...' : 'Invite'}
                </button>
              </div>
            </div>

            {/* Members List */}
            <div>
              <div className="text-sm font-medium text-[var(--text)] mb-3">
                Members ({members.length})
              </div>
              {loadingMembers ? (
                <div className="text-sm text-[var(--text-light)] py-4">Loading members...</div>
              ) : members.length === 0 ? (
                <div className="text-sm text-[var(--text-light)] py-4">No members yet. Invite someone to get started.</div>
              ) : (
                <div className="divide-y divide-[var(--border)]">
                  {members.map(member => (
                    <div key={member.id} className="py-3 flex items-center justify-between">
                      <div>
                        <div className="font-medium text-[var(--text)]">{member.user_email}</div>
                        <div className="text-xs text-[var(--text-light)]">
                          {member.role} · {member.joined_at ? `joined ${new Date(member.joined_at).toLocaleDateString()}` : 'pending'}
                        </div>
                      </div>
                      <button
                        onClick={() => removeMember(member.id)}
                        className="text-xs text-red-600 hover:text-red-700"
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </Card>
      </div>
    )
  }

  // Teams list view
  return (
    <div className="space-y-6">
      <Card title="Create Team">
        <div className="flex gap-4 items-end">
          <FormGroup label="Team Name" className="flex-1">
            <input
              type="text"
              className="settings-input"
              placeholder="Engineering"
              value={newTeam.name}
              onChange={e => setNewTeam({ name: e.target.value })}
            />
          </FormGroup>
          <button onClick={createTeam} disabled={creating || !newTeam.name} className="settings-btn-primary">
            {creating ? 'Creating...' : 'Create Team'}
          </button>
        </div>
      </Card>

      <Card title="Your Teams">
        {teams.length === 0 ? (
          <EmptyState icon={<UsersIcon />} message="No teams yet" />
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {teams.map(team => (
              <div key={team.id} className="py-4 flex items-center justify-between">
                <div>
                  <div className="font-medium text-[var(--text)]">{team.name}</div>
                  <div className="text-sm text-[var(--text-light)]">
                    {team.member_count} member{team.member_count !== 1 ? 's' : ''} · {team.plan} plan
                  </div>
                </div>
                <button
                  onClick={() => selectTeam(team)}
                  className="text-sm text-[var(--text-mid)] hover:text-[var(--text)]"
                >
                  Manage
                </button>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}

// === Audit Log Tab ===
function AuditTab() {
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    loadLogs()
  }, [])

  async function loadLogs() {
    try {
      const res = await fetch('/api/v1/audit-logs?limit=50', { credentials: 'include' })
      const data = await res.json()
      setLogs(data.logs || [])
    } catch {
      // Ignore
    } finally {
      setLoading(false)
    }
  }

  if (loading) return <LoadingState />

  return (
    <Card title="Recent Activity" action={
      <button className="text-sm text-[var(--text-mid)] hover:text-[var(--text)]">Export CSV</button>
    }>
      {logs.length === 0 ? (
        <EmptyState icon={<ClipboardIcon />} message="No activity logged yet" />
      ) : (
        <div className="divide-y divide-[var(--border)]">
          {logs.map(log => (
            <div key={log.id} className="py-3 flex items-start gap-4">
              <div className={`w-2 h-2 rounded-full mt-2 ${log.status === 'success' ? 'bg-green-500' : 'bg-red-500'}`} />
              <div className="flex-1 min-w-0">
                <div className="text-sm text-[var(--text)]">
                  <span className="font-medium">{log.actor_email}</span>
                  {' '}{formatAction(log.action)}{' '}
                  {log.resource_name && <span className="font-medium">{log.resource_name}</span>}
                </div>
                <div className="text-xs text-[var(--text-light)]">
                  {new Date(log.created_at).toLocaleString()}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}

// === Shared Components ===

function Card({ title, action, children, className = '' }: { title: string; action?: React.ReactNode; children: React.ReactNode; className?: string }) {
  return (
    <div className={`bg-white border border-[var(--border)] rounded-lg shadow-sm ${className}`}>
      <div className="flex items-center justify-between px-6 py-4 border-b border-[var(--border)]">
        <h2 className="font-semibold text-[var(--text)]">{title}</h2>
        {action}
      </div>
      <div className="px-6 py-5">{children}</div>
    </div>
  )
}

function FormGroup({ label, hint, children, className = '' }: { label: string; hint?: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={className}>
      <label className="block text-sm font-medium text-[var(--text-mid)] mb-1.5">{label}</label>
      {children}
      {hint && <p className="text-xs text-[var(--text-light)] mt-1.5">{hint}</p>}
    </div>
  )
}

function LoadingState() {
  return <div className="text-sm text-[var(--text-light)] py-8 text-center">Loading...</div>
}

function ErrorMessage({ message }: { message: string }) {
  return <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded px-4 py-3">{message}</div>
}

function EmptyState({ icon, message, action }: { icon: React.ReactNode; message: string; action?: React.ReactNode }) {
  return (
    <div className="text-center py-8">
      <div className="w-12 h-12 mx-auto mb-4 text-[var(--border-dark)]">{icon}</div>
      <p className="text-sm text-[var(--text-light)] mb-4">{message}</p>
      {action}
    </div>
  )
}

function formatAction(action: string): string {
  const parts = action.split('.')
  const verb = parts[1] || parts[0]
  const verbMap: Record<string, string> = {
    create: 'created',
    update: 'updated',
    delete: 'deleted',
    revoke: 'revoked',
    invite: 'invited',
    remove: 'removed',
    export: 'exported',
    run: 'ran',
  }
  return verbMap[verb] || verb
}

// Icons
function SettingsIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  )
}

function LockIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </svg>
  )
}

function KeyIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4" />
    </svg>
  )
}

function UsersIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
      <path d="M16 3.13a4 4 0 0 1 0 7.75" />
    </svg>
  )
}

function ClipboardIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
      <rect x="8" y="2" width="8" height="4" rx="1" ry="1" />
    </svg>
  )
}
