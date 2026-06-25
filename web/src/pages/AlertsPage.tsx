import { useEffect, useState } from 'react'
import { fetchDeliveryLogs, fetchNotificationSettings, fetchOwnership, fetchPolicies, fetchRepoJobs, fetchRepos, deleteOwnership, sendTestNotification, upsertNotificationSettings, upsertOwnership } from '../api'
import { convertFromUSD, convertToUSD, formatFromUSD, useCurrencyPreference } from '../currency'
import type { CurrencyCode } from '../currency'
import type { DeliveryLog, JobSummaryRow, NotificationSettings, OwnershipEntry, PolicyRule, RepoSummary } from '../types'

type RuleType = 'event' | 'threshold'
type EventType = 'policy_violation' | 'high_waste' | 'daily_summary'

type AlertRule = {
  id: string
  name: string
  type: RuleType
  // event rules
  event?: EventType
  // threshold rules
  scope: 'global' | 'repository' | 'job'
  repository: string
  jobId: string
  metric: 'max_cost_per_hour' | 'waste_percent' | 'monthly_savings_drop_percent'
  threshold: number
  destinationIds: string[]
  enabled: boolean
}

type ThresholdRuleDraft = {
  name: string
  scope: 'global' | 'repository' | 'job'
  repository: string
  jobId: string
  metric: AlertRule['metric']
  threshold: string
  destinationIds: string[]
}

type EventRuleDraft = {
  name: string
  event: EventType
  policyKey: string
  scope: 'global' | 'repository' | 'job'
  repository: string
  jobId: string
  destinationIds: string[]
}

type SlackDestination = {
  id: string
  name: string
  webhook_url: string
  has_secret?: boolean
  channel: string
  mention: string
}

const DEFAULT_SETTINGS: NotificationSettings = {
  enabled: true,
  events: {
    policy_violation: true,
    high_waste: false,
    daily_summary: true,
  },
  slack: {
    enabled: true,
    webhook_url: '',
    channel: '',
    mention: '',
    destinations: [],
  },
  teams: {
    enabled: false,
    destinations: [],
  },
  webhooks: {
    enabled: false,
    destinations: [],
  },
  email: {
    enabled: false,
    recipients: [],
    subject_prefix: '[RunRight]',
  },
    rules: [],
}

function normalizeSettings(input?: Partial<NotificationSettings> | null): NotificationSettings {
  if (!input) return DEFAULT_SETTINGS
  const providedDestinations = Array.isArray(input.slack?.destinations)
    ? (input.slack?.destinations as SlackDestination[])
    : []
  const migratedLegacyDestination: SlackDestination[] = input.slack?.webhook_url
    ? [{
        id: 'default',
        name: 'Default',
        webhook_url: input.slack.webhook_url,
        channel: input.slack?.channel ?? '',
        mention: input.slack?.mention ?? '',
      }]
    : []
  const destinations = (providedDestinations.length > 0 ? providedDestinations : migratedLegacyDestination)
    .filter((d): d is SlackDestination => Boolean(d?.id && d?.name && (d?.webhook_url || d?.has_secret)))
  return {
    enabled: input.enabled ?? DEFAULT_SETTINGS.enabled,
    events: {
      policy_violation: input.events?.policy_violation ?? DEFAULT_SETTINGS.events.policy_violation,
      high_waste: input.events?.high_waste ?? DEFAULT_SETTINGS.events.high_waste,
      daily_summary: input.events?.daily_summary ?? DEFAULT_SETTINGS.events.daily_summary,
    },
    slack: {
      enabled: input.slack?.enabled ?? DEFAULT_SETTINGS.slack.enabled,
      webhook_url: input.slack?.webhook_url ?? DEFAULT_SETTINGS.slack.webhook_url,
      channel: input.slack?.channel ?? DEFAULT_SETTINGS.slack.channel,
      mention: input.slack?.mention ?? DEFAULT_SETTINGS.slack.mention,
      destinations,
    },
    teams: {
      enabled: input.teams?.enabled ?? false,
      destinations: Array.isArray(input.teams?.destinations) ? input.teams!.destinations : [],
    },
    webhooks: {
      enabled: input.webhooks?.enabled ?? false,
      destinations: Array.isArray(input.webhooks?.destinations) ? input.webhooks!.destinations : [],
    },
    email: {
      enabled: input.email?.enabled ?? DEFAULT_SETTINGS.email.enabled,
      recipients: input.email?.recipients ?? DEFAULT_SETTINGS.email.recipients,
      subject_prefix: input.email?.subject_prefix ?? DEFAULT_SETTINGS.email.subject_prefix,
    },
    rules: Array.isArray(input.rules) ? (input.rules as AlertRule[]) : [],
  }
}

function isProbablyWebhook(url: string): boolean {
  return /^https?:\/\/.+/.test(url)
}

function maskWebhook(url: string): string {
  if (!url) return ''
  const trimmed = url.trim()
  if (trimmed.length <= 16) return '••••••••'
  return `${trimmed.slice(0, 26)}...${trimmed.slice(-6)}`
}

export default function AlertsPage() {
  const { currency } = useCurrencyPreference()
  const [settings, setSettings] = useState<NotificationSettings>(DEFAULT_SETTINGS)
  const [rules, setRules] = useState<AlertRule[]>([])
  const [ruleType, setRuleType] = useState<RuleType>('threshold')
  const [thresholdDraftCurrency, setThresholdDraftCurrency] = useState(currency)

  function costInputDigits(): number {
    return currency === 'JPY' ? 0 : 2
  }

  function defaultCostThresholdInput(): string {
    return convertFromUSD(0.5, currency).toFixed(costInputDigits())
  }

  function displayCostFromUSD(usdAmount: number): string {
    return convertFromUSD(usdAmount, currency).toFixed(costInputDigits())
  }

  function parseDraftCostToUSD(value: string): number {
    return convertToUSD(Number(value), currency)
  }

  function convertDisplayedCostBetweenCurrencies(value: string, fromCurrency: CurrencyCode, toCurrency: CurrencyCode): string {
    const parsed = Number(value)
    if (!Number.isFinite(parsed) || parsed < 0) {
      return convertFromUSD(0.5, toCurrency).toFixed(toCurrency === 'JPY' ? 0 : 2)
    }
    const usd = convertToUSD(parsed, fromCurrency)
    return convertFromUSD(usd, toCurrency).toFixed(toCurrency === 'JPY' ? 0 : 2)
  }

  const [thresholdDraft, setThresholdDraft] = useState<ThresholdRuleDraft>({
    name: '',
    scope: 'global',
    repository: '',
    jobId: '',
    metric: 'max_cost_per_hour',
    threshold: defaultCostThresholdInput(),
    destinationIds: [],
  })
  const [eventDraft, setEventDraft] = useState<EventRuleDraft>({
    name: '',
    event: 'policy_violation',
    policyKey: '',
    scope: 'global',
    repository: '',
    jobId: '',
    destinationIds: [],
  })
  const [editingRuleId, setEditingRuleId] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'destinations' | 'teams' | 'webhooks' | 'rules' | 'deliveries' | 'ownership'>('rules')
  const [ownershipEntries, setOwnershipEntries] = useState<OwnershipEntry[]>([])
  const [newOwnershipRepo, setNewOwnershipRepo] = useState('')
  const [newOwnershipTeam, setNewOwnershipTeam] = useState('')
  const [newOwnershipDestIds, setNewOwnershipDestIds] = useState<string[]>([])
  const [repos, setRepos] = useState<RepoSummary[]>([])
  const [repoJobs, setRepoJobs] = useState<JobSummaryRow[]>([])
  const [policies, setPolicies] = useState<PolicyRule[]>([])
  const [newDestinationName, setNewDestinationName] = useState('')
  const [newDestinationWebhook, setNewDestinationWebhook] = useState('')
  const [newDestinationChannel, setNewDestinationChannel] = useState('')
  const [newDestinationMention, setNewDestinationMention] = useState('')
  const [newTeamsName, setNewTeamsName] = useState('')
  const [newTeamsWebhook, setNewTeamsWebhook] = useState('')
  const [newWebhookName, setNewWebhookName] = useState('')
  const [newWebhookUrl, setNewWebhookUrl] = useState('')
  const [deliveryLogs, setDeliveryLogs] = useState<DeliveryLog[]>([])
  const [deliveryLogsLoading, setDeliveryLogsLoading] = useState(false)
  const [showEventRepoSuggestions, setShowEventRepoSuggestions] = useState(false)
  const [showEventJobSuggestions, setShowEventJobSuggestions] = useState(false)
  const [showThresholdRepoSuggestions, setShowThresholdRepoSuggestions] = useState(false)
  const [showThresholdJobSuggestions, setShowThresholdJobSuggestions] = useState(false)
  const [showPolicySuggestions, setShowPolicySuggestions] = useState(false)
  const [policySearch, setPolicySearch] = useState('')
  const [showDestinationPicker, setShowDestinationPicker] = useState(false)
  const [destinationSearch, setDestinationSearch] = useState('')
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')
  const [note, setNote] = useState('')

  useEffect(() => {
    // Security: remove any legacy local copy that may contain webhook secrets.
    localStorage.removeItem('rr-notifications')

    void (async () => {
      try {
        const remote = normalizeSettings(await fetchNotificationSettings())
        setSettings(remote)
        setRules(Array.isArray(remote.rules) ? remote.rules : [])
      } catch {
        setError('Unable to load notification settings from backend.')
      }
    })()
  }, [])

  useEffect(() => {
    const firstId = settings.slack.destinations?.[0]?.id
    if (!firstId) return
    setThresholdDraft((prev) => (prev.destinationIds.length > 0 ? prev : { ...prev, destinationIds: [firstId] }))
    setEventDraft((prev) => (prev.destinationIds.length > 0 ? prev : { ...prev, destinationIds: [firstId] }))
  }, [settings.slack.destinations])

  useEffect(() => {
    void fetchRepos().then(setRepos).catch(() => {})
    void fetchPolicies().then(setPolicies).catch(() => {})
  }, [])

  useEffect(() => {
    const activeScope = ruleType === 'threshold' ? thresholdDraft.scope : eventDraft.scope
    const activeRepo = ruleType === 'threshold' ? thresholdDraft.repository : eventDraft.repository
    if (activeRepo && activeScope === 'job') {
      void fetchRepoJobs(activeRepo).then(setRepoJobs).catch(() => setRepoJobs([]))
    } else {
      setRepoJobs([])
    }
  }, [ruleType, thresholdDraft.scope, thresholdDraft.repository, eventDraft.scope, eventDraft.repository])

  useEffect(() => {
    if (thresholdDraftCurrency === currency) return
    setThresholdDraft((prev) => {
      if (prev.metric !== 'max_cost_per_hour') return prev
      return {
        ...prev,
        threshold: convertDisplayedCostBetweenCurrencies(prev.threshold, thresholdDraftCurrency, currency),
      }
    })
    setThresholdDraftCurrency(currency)
  }, [currency, thresholdDraftCurrency])

  const repoSuggestions = (query: string) =>
    Array.from(new Set(repos.map((r) => r.repository).filter(Boolean)))
      .filter((repo) => query.trim() === '' || repo.toLowerCase().includes(query.toLowerCase()))
      .slice(0, 8)

  const jobSuggestions = (query: string) =>
    Array.from(new Set(repoJobs.map((j) => j.job_id).filter(Boolean)))
      .filter((job) => query.trim() === '' || job.toLowerCase().includes(query.toLowerCase()))
      .slice(0, 8)

  const policyOptions = policies
    .filter((p) => p.enabled)
    .map((p) => ({
      key: `${p.repository}::${p.job_id}`,
      label: p.repository && p.job_id
        ? `${p.repository} / ${p.job_id}`
        : p.repository
          ? `${p.repository} (repo default)`
          : 'Global policy',
      repository: p.repository,
      jobId: p.job_id,
    }))

  const filteredPolicyOptions = (query: string) => {
    const q = query.trim().toLowerCase()
    if (!q) return policyOptions.slice(0, 20)
    return policyOptions
      .filter((p) => p.label.toLowerCase().includes(q) || p.repository.toLowerCase().includes(q) || p.jobId.toLowerCase().includes(q))
      .slice(0, 20)
  }

  useEffect(() => {
    if (!eventDraft.policyKey) {
      setPolicySearch('')
      return
    }
    const picked = policyOptions.find((p) => p.key === eventDraft.policyKey)
    setPolicySearch(picked?.label ?? '')
  }, [eventDraft.policyKey, policyOptions])

  // reserved for future settings toggles

  async function persistSettings(updated: NotificationSettings) {
    const payload = {
      ...updated,
      rules: Array.isArray(updated.rules) ? updated.rules : rules,
      enabled: updated.slack.destinations.length > 0 || (updated.teams?.destinations?.length ?? 0) > 0 || (updated.webhooks?.destinations?.length ?? 0) > 0,
    }
    payload.slack.enabled = payload.slack.destinations.length > 0
    if (payload.teams) payload.teams.enabled = (payload.teams.destinations?.length ?? 0) > 0
    if (payload.webhooks) payload.webhooks.enabled = (payload.webhooks.destinations?.length ?? 0) > 0
    const invalid = payload.slack.destinations.find((d) => !d.has_secret && !isProbablyWebhook(d.webhook_url || ''))
    if (invalid) {
      setError(`Invalid webhook URL for destination: ${invalid.name}.`)
      return
    }
    if (payload.slack.destinations.length > 0) {
      payload.slack.webhook_url = payload.slack.destinations[0].webhook_url
      payload.slack.channel = payload.slack.destinations[0].channel
      payload.slack.mention = payload.slack.destinations[0].mention
    }
    try {
      await upsertNotificationSettings(payload)
      setSettings(payload)
      setNote('Saved.')
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch {
      setError('Unable to save. Check backend and try again.')
    }
  }

  async function persistRules(nextRules: AlertRule[]) {
    const payload: NotificationSettings = {
      ...settings,
      rules: nextRules,
    }
    await persistSettings(payload)
  }

  async function onSendTest() {
    setBusy(true)
    setError('')
    setNote('')
    try {
      await sendTestNotification()
      setNote('Test notification sent.')
    } catch {
      setNote('Test delivery failed. Confirm you saved settings and your backend notifications API is reachable.')
    } finally {
      setBusy(false)
    }
  }

  async function addRule(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    const name = (ruleType === 'threshold' ? thresholdDraft.name : eventDraft.name).trim()

    if (!name) { setError('Rule name is required.'); return }

    let next: AlertRule
    if (ruleType === 'threshold') {
      const thresholdValue = thresholdDraft.metric === 'max_cost_per_hour'
        ? parseDraftCostToUSD(thresholdDraft.threshold)
        : Number(thresholdDraft.threshold)
      if (thresholdDraft.destinationIds.length === 0) { setError('Select at least one destination.'); return }
      if (!Number.isFinite(thresholdValue) || thresholdValue <= 0) { setError('Threshold must be greater than zero.'); return }
      if (thresholdDraft.scope !== 'global' && !thresholdDraft.repository.trim()) { setError('Repository is required for this scope.'); return }
      if (thresholdDraft.scope === 'job' && !thresholdDraft.jobId.trim()) { setError('Job ID is required for job scope.'); return }
      next = {
        id: editingRuleId ?? crypto.randomUUID(),
        name, type: 'threshold',
        scope: thresholdDraft.scope, repository: thresholdDraft.repository.trim(), jobId: thresholdDraft.jobId.trim(),
        metric: thresholdDraft.metric, threshold: thresholdValue,
        destinationIds: thresholdDraft.destinationIds, enabled: true,
      }
    } else {
      const isGlobalEvent = eventDraft.event === 'daily_summary'
      if (eventDraft.destinationIds.length === 0) { setError('Select at least one destination.'); return }
      if (eventDraft.event === 'policy_violation') {
        if (!eventDraft.policyKey) { setError('Select which policy this alert should track.'); return }
        const picked = policyOptions.find((p) => p.key === eventDraft.policyKey)
        if (!picked) { setError('Selected policy could not be found.'); return }
        next = {
          id: editingRuleId ?? crypto.randomUUID(),
          name, type: 'event', event: eventDraft.event,
          scope: picked.jobId ? 'job' : picked.repository ? 'repository' : 'global',
          repository: picked.repository,
          jobId: picked.jobId,
          metric: 'max_cost_per_hour', threshold: 0,
          destinationIds: eventDraft.destinationIds, enabled: true,
        }
      } else {
      if (!isGlobalEvent && eventDraft.scope !== 'global' && !eventDraft.repository.trim()) { setError('Repository is required for this scope.'); return }
      if (!isGlobalEvent && eventDraft.scope === 'job' && !eventDraft.jobId.trim()) { setError('Job ID is required for job scope.'); return }
      next = {
        id: editingRuleId ?? crypto.randomUUID(),
        name, type: 'event', event: eventDraft.event,
        scope: isGlobalEvent ? 'global' : eventDraft.scope,
        repository: isGlobalEvent ? '' : eventDraft.repository.trim(),
        jobId: isGlobalEvent ? '' : eventDraft.jobId.trim(),
        metric: 'max_cost_per_hour', threshold: 0,
        destinationIds: eventDraft.destinationIds, enabled: true,
      }
      }
    }

    if (editingRuleId) {
      const nextRules = rules.map((r) => r.id === editingRuleId ? { ...next, enabled: r.enabled } : r)
      setRules(nextRules)
      await persistRules(nextRules)
      setEditingRuleId(null)
      setNote('Rule updated.')
    } else {
      const nextRules = [next, ...rules]
      setRules(nextRules)
      await persistRules(nextRules)
      setNote('Alert rule added.')
    }

    const firstId = settings.slack.destinations?.[0]?.id
    setThresholdDraft({
      name: '',
      scope: 'global',
      repository: '',
      jobId: '',
      metric: 'max_cost_per_hour',
      threshold: defaultCostThresholdInput(),
      destinationIds: firstId ? [firstId] : [],
    })
    setEventDraft({
      name: '',
      event: 'policy_violation',
      policyKey: '',
      scope: 'global',
      repository: '',
      jobId: '',
      destinationIds: firstId ? [firstId] : [],
    })
    setPolicySearch('')
  }

  function startEditRule(rule: AlertRule) {
    setEditingRuleId(rule.id)
    setRuleType(rule.type)
    if (rule.type === 'threshold') {
      setThresholdDraft({
        name: rule.name,
        scope: rule.scope,
        repository: rule.repository,
        jobId: rule.jobId,
        metric: rule.metric,
        threshold: rule.metric === 'max_cost_per_hour' ? displayCostFromUSD(rule.threshold) : String(rule.threshold),
        destinationIds: rule.destinationIds,
      })
    } else {
      setEventDraft({
        name: rule.name,
        event: rule.event ?? 'policy_violation',
        policyKey: `${rule.repository}::${rule.jobId}`,
        scope: rule.scope,
        repository: rule.repository,
        jobId: rule.jobId,
        destinationIds: rule.destinationIds,
      })
      const label = rule.repository && rule.jobId
        ? `${rule.repository} / ${rule.jobId}`
        : rule.repository
          ? `${rule.repository} (repo default)`
          : 'Global policy'
      setPolicySearch(label)
    }
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  function cancelEdit() {
    setEditingRuleId(null)
    const firstId = settings.slack.destinations?.[0]?.id
    setThresholdDraft((prev) => ({ ...prev, name: '', repository: '', jobId: '', scope: 'global', metric: 'max_cost_per_hour', threshold: defaultCostThresholdInput(), destinationIds: firstId ? [firstId] : [] }))
    setEventDraft((prev) => ({ ...prev, name: '', event: 'policy_violation', policyKey: '', repository: '', jobId: '', scope: 'global', destinationIds: firstId ? [firstId] : [] }))
    setPolicySearch('')
    setRuleType('threshold')
  }

  async function addDestination(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    const name = newDestinationName.trim()
    const webhook = newDestinationWebhook.trim()
    if (!name) { setError('Destination name is required.'); return }
    if (!isProbablyWebhook(webhook)) { setError('A valid webhook URL is required.'); return }

    const next: SlackDestination = {
      id: crypto.randomUUID(), name, webhook_url: webhook,
      channel: newDestinationChannel.trim(), mention: newDestinationMention.trim(),
    }
    const updated: NotificationSettings = {
      ...settings,
      slack: { ...settings.slack, destinations: [...settings.slack.destinations, next] },
    }
    setThresholdDraft((prev) => (prev.destinationIds.length === 0 ? { ...prev, destinationIds: [next.id] } : prev))
    setEventDraft((prev) => (prev.destinationIds.length === 0 ? { ...prev, destinationIds: [next.id] } : prev))
    setNewDestinationName('')
    setNewDestinationWebhook('')
    setNewDestinationChannel('')
    setNewDestinationMention('')
    setBusy(true)
    setError('')
    setNote('')
    await persistSettings(updated)
    setBusy(false)
  }

  async function removeDestination(id: string) {
    const updated: NotificationSettings = {
      ...settings,
      slack: { ...settings.slack, destinations: settings.slack.destinations.filter((d) => d.id !== id) },
    }
    setThresholdDraft((prev) => ({ ...prev, destinationIds: prev.destinationIds.filter((did) => did !== id) }))
    setEventDraft((prev) => ({ ...prev, destinationIds: prev.destinationIds.filter((did) => did !== id) }))
    setRules((prev) => prev.map((r) => ({ ...r, destinationIds: r.destinationIds.filter((did) => did !== id) })))
    setBusy(true)
    setError('')
    setNote('')
    await persistSettings(updated)
    setBusy(false)
  }

  function toggleRuleDestination(id: string, checked: boolean) {
    if (ruleType === 'threshold') {
      setThresholdDraft((prev) => {
        const current = prev.destinationIds
        if (checked) {
          if (current.includes(id)) return prev
          return { ...prev, destinationIds: [...current, id] }
        }
        return { ...prev, destinationIds: current.filter((destinationId) => destinationId !== id) }
      })
      return
    }
    setEventDraft((prev) => {
      const current = prev.destinationIds
      if (checked) {
        if (current.includes(id)) return prev
        return { ...prev, destinationIds: [...current, id] }
      }
      return { ...prev, destinationIds: current.filter((destinationId) => destinationId !== id) }
    })
  }

  function destinationNames(ids: string[]): string {
    const byId = new Map([
      ...settings.slack.destinations.map((d) => [d.id, d.name] as const),
      ...(settings.teams?.destinations ?? []).map((d) => [d.id, `${d.name} (Teams)`] as const),
      ...(settings.webhooks?.destinations ?? []).map((d) => [d.id, `${d.name} (Webhook)`] as const),
    ])
    const names = ids.map((id) => byId.get(id)).filter((name): name is string => Boolean(name))
    return names.length > 0 ? names.join(', ') : 'No destinations'
  }

  function removeRule(id: string) {
    const nextRules = rules.filter((r) => r.id !== id)
    setRules(nextRules)
    void persistRules(nextRules)
  }

  function toggleRuleEnabled(id: string, enabled: boolean) {
    const nextRules = rules.map((r) => (r.id === id ? { ...r, enabled } : r))
    setRules(nextRules)
    void persistRules(nextRules)
  }

  function metricLabel(metric: AlertRule['metric']): string {
    if (metric === 'max_cost_per_hour') return 'Max Cost/hr'
    if (metric === 'waste_percent') return 'Waste %'
    return 'Monthly Savings Drop %'
  }

  function eventLabel(event: EventType): string {
    if (event === 'policy_violation') return 'Policy Violation'
    if (event === 'high_waste') return 'High Waste Job'
    return 'Daily Summary'
  }

  function eventDescription(event: EventType): string {
    if (event === 'policy_violation') return 'Fires when a job run costs more per hour than a limit set on the Policies page.'
    if (event === 'high_waste') return 'Fires when a job repeatedly uses less than 20% of its machine.'
    return 'Fires once per day with a summary of cost, waste, and policy events.'
  }

  function thresholdHint(metric: AlertRule['metric']): string {
    if (metric === 'max_cost_per_hour') return `Triggers when detected cost per hour is greater than this ${currency}/hr amount.`
    if (metric === 'waste_percent') return 'Triggers when estimated waste percent is greater than this value (for example 80 = 80%).'
    return 'Triggers when monthly savings drops by more than this percent from baseline (for example 25 = 25%).'
  }

  function thresholdUnit(metric: AlertRule['metric']): string {
    if (metric === 'max_cost_per_hour') return `${currency}/hr`
    return '%'
  }

  function thresholdStep(metric: AlertRule['metric']): string {
    if (metric === 'max_cost_per_hour') return currency === 'JPY' ? '1' : '0.01'
    return '1'
  }

  function thresholdPlaceholder(metric: AlertRule['metric']): string {
    if (metric === 'max_cost_per_hour') return defaultCostThresholdInput()
    if (metric === 'waste_percent') return '80'
    return '25'
  }

  function convertThresholdForMetricSwitch(
    value: string,
    fromMetric: AlertRule['metric'],
    toMetric: AlertRule['metric'],
  ): string {
    if (fromMetric === toMetric) return value
    const parsed = Number(value)
    if (!Number.isFinite(parsed) || parsed < 0) {
      return thresholdPlaceholder(toMetric)
    }

    const fromIsPercent = fromMetric !== 'max_cost_per_hour'
    const toIsPercent = toMetric !== 'max_cost_per_hour'

    // Keep percent-to-percent unchanged; only convert when crossing cost/hr <-> %.
    if (fromIsPercent && toIsPercent) return String(Math.round(parsed))
    if (!fromIsPercent && !toIsPercent) return parsed.toFixed(costInputDigits())

    if (!fromIsPercent && toIsPercent) {
      // Cost/hr input (in selected currency) -> percent-style threshold using USD baseline.
      const parsedUSD = parseDraftCostToUSD(value)
      return String(Math.round(parsedUSD * 100))
    }

    // Percent-style threshold (50) -> cost/hr displayed in selected currency.
    return displayCostFromUSD(parsed / 100)
  }

  const activeDestinationIds = ruleType === 'threshold' ? thresholdDraft.destinationIds : eventDraft.destinationIds
  const selectedDestinations = settings.slack.destinations.filter((d) => activeDestinationIds.includes(d.id))
  const filteredDestinations = settings.slack.destinations.filter((d) => {
    const q = destinationSearch.trim().toLowerCase()
    if (!q) return true
    const haystack = `${d.name} ${d.channel} ${d.mention}`.toLowerCase()
    return haystack.includes(q)
  })

  const tabBtn = (tab: typeof activeTab, label: string) => (
    <button
      type="button"
      onClick={() => {
        setActiveTab(tab)
        if (tab === 'deliveries' && deliveryLogs.length === 0) {
          setDeliveryLogsLoading(true)
          void fetchDeliveryLogs(undefined, 100).then(setDeliveryLogs).finally(() => setDeliveryLogsLoading(false))
        }
      }}
      className={`px-5 py-2.5 font-deco text-[13px] tracking-[1.5px] uppercase border-b-2 transition-colors ${
        activeTab === tab
          ? 'border-[var(--red)] text-[var(--text)]'
          : 'border-transparent text-[var(--text-light)] hover:text-[var(--text-mid)]'
      }`}
    >{label}</button>
  )

  const icon = {
    event: (
      <svg viewBox="0 0 20 20" aria-hidden="true" className="h-3.5 w-3.5">
        <path fill="currentColor" d="M10 1a1 1 0 0 1 1 1v1.07a6 6 0 0 1 4.93 5.9V12l1.46 2.2A1 1 0 0 1 16.56 16H3.44a1 1 0 0 1-.83-1.8L4.07 12V8.97A6 6 0 0 1 9 3.07V2a1 1 0 0 1 1-1Zm-2.5 16h5a2.5 2.5 0 0 1-5 0Z"/>
      </svg>
    ),
    threshold: (
      <svg viewBox="0 0 20 20" aria-hidden="true" className="h-3.5 w-3.5">
        <path fill="currentColor" d="M2.5 14a1 1 0 0 0 0 2h15a1 1 0 1 0 0-2h-15ZM2.5 9a1 1 0 1 0 0 2h10a1 1 0 1 0 0-2h-10Zm0-5a1 1 0 1 0 0 2h6a1 1 0 1 0 0-2h-6Z"/>
      </svg>
    ),
    edit: (
      <svg viewBox="0 0 20 20" aria-hidden="true" className="h-3.5 w-3.5">
        <path d="M13.8 3.2 16.8 6.2 7.2 15.8 3.6 16.6 4.4 13z" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
      </svg>
    ),
    trash: (
      <svg viewBox="0 0 20 20" aria-hidden="true" className="h-3.5 w-3.5">
        <path d="M4.5 5.5h11" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
        <path d="M7.2 5.5V4.2c0-.5.4-.9.9-.9h3.8c.5 0 .9.4.9.9v1.3" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
        <path d="m6.2 7 1 9.2c.1.6.5 1 1.1 1h3.4c.6 0 1-.4 1.1-1l1-9.2" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
      </svg>
    ),
  }

  return (
    <div className="fadein max-w-[1280px]">
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-5 tracking-tight">Alerts</h1>

      {/* Tab bar */}
      <div className="flex gap-0 border-b border-[var(--border)] mb-6 flex-wrap">
        {tabBtn('rules', 'Rules')}
        {tabBtn('destinations', 'Slack')}
        {tabBtn('teams', 'Teams')}
        {tabBtn('webhooks', 'Webhooks')}
        {tabBtn('ownership', 'Ownership')}
        {tabBtn('deliveries', 'Delivery Logs')}
      </div>

      {/* ── Destinations tab ── */}
      {activeTab === 'destinations' && (
        <div className="rr-card max-w-2xl">
          <h2 className="font-serif text-[18px] font-bold text-[var(--text)] mb-1">Slack Destinations</h2>
          <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
            Each destination is a Slack webhook URL. Alert rules route notifications here. Add one destination per channel or team you want to notify.
          </p>

          <form onSubmit={(e) => void addDestination(e)}>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3 mb-3">
              <div className="form-group mb-0">
                <label>Name</label>
                <input type="text" placeholder="CI Alerts" value={newDestinationName} onChange={(e) => setNewDestinationName(e.target.value)} />
              </div>
              <div className="form-group mb-0">
                <label>Webhook URL</label>
                <input type="text" placeholder="https://hooks.slack.com/services/..." value={newDestinationWebhook} onChange={(e) => setNewDestinationWebhook(e.target.value)} />
              </div>
              <div className="form-group mb-0">
                <label>Channel <span className="text-[var(--text-light)] font-normal">(optional)</span></label>
                <input type="text" placeholder="#ci-alerts" value={newDestinationChannel} onChange={(e) => setNewDestinationChannel(e.target.value)} />
              </div>
              <div className="form-group mb-0">
                <label>Mention <span className="text-[var(--text-light)] font-normal">(optional)</span></label>
                <input type="text" placeholder="@oncall-team" value={newDestinationMention} onChange={(e) => setNewDestinationMention(e.target.value)} />
              </div>
            </div>
            <button className="btn-rr mt-3" type="submit" disabled={busy}>
              {busy ? 'Saving...' : 'Add Destination'}
            </button>
          </form>

          {error && <p className="text-red text-sm mt-4">{error}</p>}
          {note && <p className="text-sm text-[var(--text-light)] mt-3">{note}</p>}

          <div className="mt-6">
            <div className="font-deco text-[11px] tracking-[1.5px] text-[var(--text-mid)] uppercase mb-3">Saved Destinations</div>
            {settings.slack.destinations.length === 0 ? (
              <div className="empty text-sm">No destinations yet. Add one above.</div>
            ) : (
              <div className="space-y-2">
                {settings.slack.destinations.map((d) => (
                  <div key={d.id} className="bg-paper border border-[var(--border)] rounded px-4 py-3 flex items-start justify-between gap-4">
                    <div>
                      <div className="font-semibold text-sm text-[var(--text)]">{d.name}</div>
                      <div className="text-xs text-[var(--text-light)] mt-1">
                        {d.channel || '(default channel)'}{d.mention ? ` · ${d.mention}` : ''}
                      </div>
                      <div className="text-xs text-[var(--text-light)] mt-0.5 font-mono">{maskWebhook(d.webhook_url)}</div>
                    </div>
                    <button type="button" className="text-red underline underline-offset-2 text-xs shrink-0" disabled={busy} onClick={() => void removeDestination(d.id)}>Remove</button>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="flex items-center gap-3 mt-6 pt-4 border-t border-[var(--border)]">
            <button className="btn-rr" type="button" onClick={() => void onSendTest()} disabled={busy || settings.slack.destinations.length === 0}>Send Test</button>
            {saved && <span className="font-deco text-[15px] tracking-[2px] text-[#2E7D32]">Saved</span>}
            <span className="text-xs text-[var(--text-light)]">Webhook values are masked in the UI.</span>
          </div>
        </div>
      )}

      {/* ── Rules tab ── */}
      {activeTab === 'rules' && (
        <div className="rr-card">
          <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-1">Alert Rules</h2>
          <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
            <strong>Threshold</strong> — fires when a metric crosses a number you set.&nbsp;&nbsp;
            <strong>Event</strong> — fires when RunRight detects a policy breach, high waste, or sends a daily digest.
          </p>

          {settings.slack.destinations.length === 0 && (
            <div className="bg-[var(--cream-alt)] border border-[var(--border)] rounded px-4 py-3 text-sm text-[var(--text-mid)] mb-5">
              No Slack destinations configured.{' '}
              <button type="button" className="underline underline-offset-2" onClick={() => setActiveTab('destinations')}>Add one first.</button>
            </div>
          )}

          <form className="settings-form settings-form-wide" onSubmit={addRule}>
            <div className="flex gap-2 mb-5">
              {(['threshold', 'event'] as const).map((t) => (
                <button key={t} type="button"
                  className={`px-4 py-2 text-sm font-semibold border capitalize ${ruleType === t ? 'bg-[var(--red)] text-[var(--cream)] border-[var(--red)]' : 'bg-paper text-[var(--text-mid)] border-[var(--border)]'}`}
                  onClick={() => setRuleType(t)}
                >{t}</button>
              ))}
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
              <div className="form-group">
                <label>Rule Name</label>
                <input
                  type="text"
                  placeholder={ruleType === 'event' ? 'Policy Breach → #ops' : 'High Cost Build'}
                  value={ruleType === 'event' ? eventDraft.name : thresholdDraft.name}
                  onChange={(e) => {
                    const v = e.target.value
                    if (ruleType === 'event') setEventDraft((prev) => ({ ...prev, name: v }))
                    else setThresholdDraft((prev) => ({ ...prev, name: v }))
                  }}
                />
              </div>

              {ruleType === 'event' ? (
                <>
                  <div className="form-group md:col-span-2">
                    <label>Event Type</label>
                    <select className="rr-select w-full" value={eventDraft.event} onChange={(e) => {
                      setEventDraft((prev) => ({ ...prev, event: e.target.value as EventType, scope: 'global', policyKey: '' }))
                      setPolicySearch('')
                    }}>
                      <option value="policy_violation">Policy Violation — job exceeds cost/hr policy</option>
                      <option value="high_waste">High Waste Job — job uses less than 20% of its machine</option>
                      <option value="daily_summary">Daily Summary — one digest per day (no scope)</option>
                    </select>
                    <p className="text-xs text-[var(--text-light)] mt-1.5">{eventDescription(eventDraft.event)}</p>
                  </div>
                  {eventDraft.event === 'policy_violation' && (
                    <div className="form-group md:col-span-2">
                      <label>Policy Target</label>
                      <input
                        type="text"
                        placeholder="Search policy by repo/job"
                        value={policySearch}
                        onChange={(e) => {
                          setPolicySearch(e.target.value)
                          setEventDraft((prev) => ({ ...prev, policyKey: '' }))
                        }}
                        onFocus={() => setShowPolicySuggestions(true)}
                        onClick={() => setShowPolicySuggestions(true)}
                        onBlur={() => setTimeout(() => setShowPolicySuggestions(false), 120)}
                      />
                      {showPolicySuggestions && filteredPolicyOptions(policySearch).length > 0 && (
                        <div className="mt-2 border border-[var(--border)] rounded max-h-44 overflow-auto bg-paper">
                          {filteredPolicyOptions(policySearch).map((p) => (
                            <button
                              key={p.key}
                              type="button"
                              className="block w-full text-left px-2 py-1.5 text-xs hover:bg-[var(--cream-alt)]"
                              onMouseDown={() => {
                                setEventDraft((prev) => ({ ...prev, policyKey: p.key }))
                                setPolicySearch(p.label)
                                setShowPolicySuggestions(false)
                              }}
                            >
                              {p.label}
                            </button>
                          ))}
                        </div>
                      )}
                      <p className="text-xs text-[var(--text-light)] mt-1.5">This event alert follows the selected policy scope automatically.</p>
                    </div>
                  )}
                  {eventDraft.event !== 'daily_summary' && eventDraft.event !== 'policy_violation' && (
                    <>
                      <div className="form-group">
                        <label>Scope</label>
                        <select className="rr-select w-full" value={eventDraft.scope} onChange={(e) => setEventDraft((prev) => ({ ...prev, scope: e.target.value as AlertRule['scope'], repository: '', jobId: '' }))}>
                          <option value="global">Global (any job)</option>
                          <option value="repository">Repository</option>
                          <option value="job">Specific Job</option>
                        </select>
                      </div>
                      {eventDraft.scope !== 'global' && (
                        <div className="form-group">
                          <label>Repository</label>
                          <input
                            type="text"
                            placeholder="owner/repo"
                            value={eventDraft.repository}
                            onChange={(e) => setEventDraft((prev) => ({ ...prev, repository: e.target.value }))}
                            onFocus={() => setShowEventRepoSuggestions(true)}
                            onClick={() => setShowEventRepoSuggestions(true)}
                            onBlur={() => setTimeout(() => setShowEventRepoSuggestions(false), 120)}
                          />
                          {showEventRepoSuggestions && repoSuggestions(eventDraft.repository).length > 0 && (
                            <div className="mt-2 border border-[var(--border)] rounded max-h-28 overflow-auto bg-paper">
                              {repoSuggestions(eventDraft.repository).map((repo) => (
                                <button
                                  key={repo}
                                  type="button"
                                  className="block w-full text-left px-2 py-1.5 text-xs hover:bg-[var(--cream-alt)]"
                                  onMouseDown={() => {
                                    setEventDraft((prev) => ({ ...prev, repository: repo, jobId: '' }))
                                    setShowEventRepoSuggestions(false)
                                    setShowEventJobSuggestions(false)
                                  }}
                                >
                                  {repo}
                                </button>
                              ))}
                            </div>
                          )}
                        </div>
                      )}
                      {eventDraft.scope === 'job' && (
                        <div className="form-group">
                          <label>Job ID</label>
                          <input
                            type="text"
                            placeholder="build"
                            value={eventDraft.jobId}
                            onChange={(e) => setEventDraft((prev) => ({ ...prev, jobId: e.target.value }))}
                            onFocus={() => setShowEventJobSuggestions(true)}
                            onClick={() => setShowEventJobSuggestions(true)}
                            onBlur={() => setTimeout(() => setShowEventJobSuggestions(false), 120)}
                          />
                          {showEventJobSuggestions && jobSuggestions(eventDraft.jobId).length > 0 && (
                            <div className="mt-2 border border-[var(--border)] rounded max-h-28 overflow-auto bg-paper">
                              {jobSuggestions(eventDraft.jobId).map((jobId) => (
                                <button
                                  key={jobId}
                                  type="button"
                                  className="block w-full text-left px-2 py-1.5 text-xs hover:bg-[var(--cream-alt)]"
                                  onMouseDown={() => {
                                    setEventDraft((prev) => ({ ...prev, jobId }))
                                    setShowEventJobSuggestions(false)
                                  }}
                                >
                                  {jobId}
                                </button>
                              ))}
                            </div>
                          )}
                        </div>
                      )}
                    </>
                  )}
                </>
              ) : (
                <>
                  <div className="form-group">
                    <label>Scope</label>
                    <select className="rr-select w-full" value={thresholdDraft.scope} onChange={(e) => setThresholdDraft((prev) => ({ ...prev, scope: e.target.value as AlertRule['scope'], repository: '', jobId: '' }))}>
                      <option value="global">Global</option>
                      <option value="repository">Repository</option>
                      <option value="job">Job</option>
                    </select>
                  </div>
                  <div className="form-group">
                    <label>Metric</label>
                      <select className="rr-select w-full" value={thresholdDraft.metric} onChange={(e) => {
                        const nextMetric = e.target.value as AlertRule['metric']
                        setThresholdDraft((prev) => ({
                          ...prev,
                          metric: nextMetric,
                          threshold: convertThresholdForMetricSwitch(prev.threshold, prev.metric, nextMetric),
                        }))
                      }}>
                      <option value="max_cost_per_hour">Max Cost/hr</option>
                      <option value="waste_percent">Waste %</option>
                      <option value="monthly_savings_drop_percent">Monthly Savings Drop %</option>
                    </select>
                  </div>
                  {thresholdDraft.scope !== 'global' && (
                    <div className="form-group">
                      <label>Repository</label>
                      <input
                        type="text"
                        placeholder="owner/repo"
                        value={thresholdDraft.repository}
                        onChange={(e) => setThresholdDraft((prev) => ({ ...prev, repository: e.target.value }))}
                        onFocus={() => setShowThresholdRepoSuggestions(true)}
                        onClick={() => setShowThresholdRepoSuggestions(true)}
                        onBlur={() => setTimeout(() => setShowThresholdRepoSuggestions(false), 120)}
                      />
                      {showThresholdRepoSuggestions && repoSuggestions(thresholdDraft.repository).length > 0 && (
                        <div className="mt-2 border border-[var(--border)] rounded max-h-28 overflow-auto bg-paper">
                          {repoSuggestions(thresholdDraft.repository).map((repo) => (
                            <button
                              key={repo}
                              type="button"
                              className="block w-full text-left px-2 py-1.5 text-xs hover:bg-[var(--cream-alt)]"
                              onMouseDown={() => {
                                setThresholdDraft((prev) => ({ ...prev, repository: repo, jobId: '' }))
                                setShowThresholdRepoSuggestions(false)
                                setShowThresholdJobSuggestions(false)
                              }}
                            >
                              {repo}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                  {thresholdDraft.scope === 'job' && (
                    <div className="form-group">
                      <label>Job ID</label>
                      <input
                        type="text"
                        placeholder="build"
                        value={thresholdDraft.jobId}
                        onChange={(e) => setThresholdDraft((prev) => ({ ...prev, jobId: e.target.value }))}
                        onFocus={() => setShowThresholdJobSuggestions(true)}
                        onClick={() => setShowThresholdJobSuggestions(true)}
                        onBlur={() => setTimeout(() => setShowThresholdJobSuggestions(false), 120)}
                      />
                      {showThresholdJobSuggestions && jobSuggestions(thresholdDraft.jobId).length > 0 && (
                        <div className="mt-2 border border-[var(--border)] rounded max-h-28 overflow-auto bg-paper">
                          {jobSuggestions(thresholdDraft.jobId).map((jobId) => (
                            <button
                              key={jobId}
                              type="button"
                              className="block w-full text-left px-2 py-1.5 text-xs hover:bg-[var(--cream-alt)]"
                              onMouseDown={() => {
                                setThresholdDraft((prev) => ({ ...prev, jobId }))
                                setShowThresholdJobSuggestions(false)
                              }}
                            >
                              {jobId}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                  <div className="form-group">
                    <label>Threshold ({thresholdUnit(thresholdDraft.metric)})</label>
                    <input type="number" min="0" step={thresholdStep(thresholdDraft.metric)} placeholder={thresholdPlaceholder(thresholdDraft.metric)} value={thresholdDraft.threshold} onChange={(e) => setThresholdDraft((prev) => ({ ...prev, threshold: e.target.value }))} />
                    <p className="text-xs text-[var(--text-light)] mt-1.5">{thresholdHint(thresholdDraft.metric)}</p>
                  </div>
                </>
              )}
            </div>

            <div className="mb-4 mt-2">
              <div className="text-sm text-[var(--text-mid)] mb-2">Route To Destinations</div>
              {settings.slack.destinations.length === 0 ? (
                <span className="text-xs text-[var(--text-light)]">No destinations — <button type="button" className="underline" onClick={() => setActiveTab('destinations')}>add one first</button>.</span>
              ) : (
                <div className="space-y-2">
                  <button
                    type="button"
                    className="w-full text-left bg-paper border border-[var(--border)] rounded px-3 py-2 text-sm text-[var(--text)] hover:border-[var(--border-dark)]"
                    onClick={() => setShowDestinationPicker((prev) => !prev)}
                  >
                    {selectedDestinations.length > 0
                      ? `Selected: ${selectedDestinations.map((d) => d.name).join(', ')}`
                      : 'Select destination channels...'}
                  </button>

                  {showDestinationPicker && (
                    <div className="border border-[var(--border)] rounded bg-paper p-2">
                      <input
                        type="text"
                        className="rr-input"
                        placeholder="Search destinations"
                        value={destinationSearch}
                        onChange={(e) => setDestinationSearch(e.target.value)}
                      />
                      <div className="mt-2 max-h-48 overflow-auto border border-[var(--border)] rounded">
                        {filteredDestinations.length === 0 ? (
                          <div className="px-3 py-2 text-xs text-[var(--text-light)]">No matches</div>
                        ) : (
                          filteredDestinations.map((d) => (
                            <label key={d.id} className="flex items-start gap-3 px-3 py-2 text-sm text-[var(--text-mid)] hover:bg-[var(--cream-alt)] cursor-pointer">
                              <input
                                type="checkbox"
                                className="mt-0.5"
                                checked={activeDestinationIds.includes(d.id)}
                                onChange={(e) => toggleRuleDestination(d.id, e.target.checked)}
                              />
                              <span>
                                <span className="block font-semibold text-[var(--text)]">{d.name}</span>
                                <span className="block text-xs text-[var(--text-light)]">
                                  {d.channel || '(default channel)'}{d.mention ? ` · ${d.mention}` : ''}
                                </span>
                              </span>
                            </label>
                          ))
                        )}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>

            {error && <p className="text-red text-sm mb-3">{error}</p>}
            {note && <p className="text-sm text-[var(--text-light)] mb-3">{note}</p>}

            <div className="flex items-center gap-3 flex-wrap">
              <button className="btn-rr" type="submit">{editingRuleId ? 'Update Rule' : 'Add Rule'}</button>
              {editingRuleId && <button type="button" className="btn-rr" style={{ background: 'var(--text-light)' }} onClick={cancelEdit}>Cancel</button>}
            </div>
          </form>

          <div className="mt-6">
            {rules.length === 0 ? (
              <div className="empty text-base">No alert rules yet.</div>
            ) : (
              <div className="space-y-3">
                {rules.map((rule) => (
                  <div key={rule.id} className={`bg-paper border rounded px-4 py-3 ${editingRuleId === rule.id ? 'border-[var(--gold)]' : 'border-[var(--border)]'}`}>
                    <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
                      <div>
                        <div className="flex items-center gap-2">
                          <div className="font-sans font-semibold text-sm text-[var(--text)]">{rule.name}</div>
                          <span className={`inline-flex items-center gap-1 text-[10px] font-bold px-1.5 py-0.5 rounded uppercase tracking-wide ${rule.type === 'event' ? 'bg-[var(--navy)] text-[var(--cream)]' : 'bg-[var(--gold)] text-[var(--text)]'}`}>
                            {rule.type === 'event' ? icon.event : icon.threshold}
                            {rule.type ?? 'threshold'}
                          </span>
                        </div>
                        <div className="text-xs text-[var(--text-light)] mt-1">
                          {rule.type === 'event'
                            ? (() => {
                                const scopePart = rule.scope !== 'global' ? ` · ${rule.scope === 'job' ? `${rule.repository} / ${rule.jobId}` : rule.repository}` : ''
                                return `${eventLabel(rule.event ?? 'policy_violation')}${scopePart}`
                              })()
                            : `${rule.scope.toUpperCase()} · ${metricLabel(rule.metric)} > ${rule.metric === 'max_cost_per_hour' ? `${formatFromUSD(rule.threshold, currency, { minimumFractionDigits: currency === 'JPY' ? 0 : 2, maximumFractionDigits: currency === 'JPY' ? 0 : 2 })}/hr` : `${rule.threshold} ${thresholdUnit(rule.metric)}`}${rule.repository ? ` · ${rule.repository}` : ''}${rule.jobId ? ` / ${rule.jobId}` : ''}`
                          }
                        </div>
                        <div className="text-xs text-[var(--text-light)] mt-1">→ {destinationNames(rule.destinationIds)}</div>
                      </div>
                      <div className="flex items-center gap-3 shrink-0">
                        <label className="rr-switch-row rr-switch-label">
                          <input
                            className="rr-switch"
                            type="checkbox"
                            checked={rule.enabled}
                            onChange={(e) => toggleRuleEnabled(rule.id, e.target.checked)}
                            title={rule.enabled ? 'Disable alert rule' : 'Enable alert rule'}
                            aria-label={rule.enabled ? 'Disable alert rule' : 'Enable alert rule'}
                          />
                          <span className={`rr-switch-state ${rule.enabled ? 'on' : 'off'}`}>{rule.enabled ? 'Enabled' : 'Disabled'}</span>
                        </label>
                        <button
                          type="button"
                          className="inline-flex h-7 w-7 items-center justify-center rounded border border-[var(--border)] text-[var(--text-light)] hover:border-[var(--border-dark)] hover:text-[var(--text-mid)] hover:bg-[var(--cream-alt)]"
                          onClick={() => startEditRule(rule)}
                          title="Edit rule"
                          aria-label="Edit rule"
                        >
                          {icon.edit}
                        </button>
                        <button
                          type="button"
                          className="inline-flex h-7 w-7 items-center justify-center rounded border border-[var(--border)] text-[var(--text-light)] hover:border-[var(--red-dark)] hover:text-[var(--red-dark)] hover:bg-[rgba(194,59,34,.08)]"
                          onClick={() => removeRule(rule.id)}
                          title="Delete rule"
                          aria-label="Delete rule"
                        >
                          {icon.trash}
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* ── Teams tab ── */}
      {activeTab === 'teams' && (
        <div className="rr-card">
          <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-1">Microsoft Teams Destinations</h2>
          <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">Add Teams Incoming Webhook URLs. Rules can route to any destination across Slack, Teams, and custom webhooks.</p>

          {(settings.teams?.destinations ?? []).length > 0 && (
            <div className="divide-y divide-[var(--border)] border border-[var(--border)] rounded mb-6">
              {settings.teams!.destinations.map((d) => (
                <div key={d.id} className="flex items-center justify-between px-4 py-3 gap-3">
                  <div className="min-w-0">
                    <div className="font-semibold text-sm text-[var(--text)] truncate">{d.name}</div>
                    <div className="text-xs text-[var(--text-light)] mt-0.5">{d.has_secret ? 'Webhook saved ✓' : d.webhook_url || '—'}</div>
                  </div>
                  <button type="button"
                    className="flex-shrink-0 text-xs font-deco tracking-widest text-[var(--red)] border border-[var(--red)] px-3 py-1 rounded hover:bg-[rgba(194,59,34,.06)]"
                    onClick={() => {
                      const updated: NotificationSettings = {
                        ...settings,
                        teams: { ...settings.teams!, destinations: (settings.teams?.destinations ?? []).filter((x) => x.id !== d.id) },
                      }
                      setBusy(true); void persistSettings(updated).finally(() => setBusy(false))
                    }}
                  >Remove</button>
                </div>
              ))}
            </div>
          )}

          <form className="settings-form" onSubmit={async (e) => {
            e.preventDefault()
            const name = newTeamsName.trim(); const webhook = newTeamsWebhook.trim()
            if (!name) { setError('Name is required.'); return }
            if (!isProbablyWebhook(webhook)) { setError('A valid Teams webhook URL is required.'); return }
            const next = { id: crypto.randomUUID(), name, webhook_url: webhook }
            const updated: NotificationSettings = {
              ...settings,
              teams: { enabled: true, destinations: [...(settings.teams?.destinations ?? []), next] },
            }
            setNewTeamsName(''); setNewTeamsWebhook('')
            setBusy(true); setError(''); setNote('')
            await persistSettings(updated)
            setBusy(false)
          }}>
            <div className="form-group">
              <label>Destination Name</label>
              <input type="text" placeholder="e.g. CI Alerts" value={newTeamsName} onChange={(e) => setNewTeamsName(e.target.value)} />
            </div>
            <div className="form-group">
              <label>Teams Incoming Webhook URL</label>
              <input type="url" placeholder="https://outlook.office.com/webhook/..." value={newTeamsWebhook} onChange={(e) => setNewTeamsWebhook(e.target.value)} />
            </div>
            <button type="submit" className="btn-rr" disabled={busy}>Add Teams Destination</button>
          </form>
        </div>
      )}

      {/* ── Webhooks tab ── */}
      {activeTab === 'webhooks' && (
        <div className="rr-card">
          <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-1">Generic Webhook Destinations</h2>
          <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">Send structured JSON payloads to any HTTP endpoint — PagerDuty, custom pipelines, Zapier, or your own webhook receiver.</p>

          {(settings.webhooks?.destinations ?? []).length > 0 && (
            <div className="divide-y divide-[var(--border)] border border-[var(--border)] rounded mb-6">
              {settings.webhooks!.destinations.map((d) => (
                <div key={d.id} className="flex items-center justify-between px-4 py-3 gap-3">
                  <div className="min-w-0">
                    <div className="font-semibold text-sm text-[var(--text)] truncate">{d.name}</div>
                    <div className="text-xs text-[var(--text-light)] mt-0.5">{d.has_secret ? 'URL saved ✓' : d.url || '—'}</div>
                  </div>
                  <button type="button"
                    className="flex-shrink-0 text-xs font-deco tracking-widest text-[var(--red)] border border-[var(--red)] px-3 py-1 rounded hover:bg-[rgba(194,59,34,.06)]"
                    onClick={() => {
                      const updated: NotificationSettings = {
                        ...settings,
                        webhooks: { ...settings.webhooks!, destinations: (settings.webhooks?.destinations ?? []).filter((x) => x.id !== d.id) },
                      }
                      setBusy(true); void persistSettings(updated).finally(() => setBusy(false))
                    }}
                  >Remove</button>
                </div>
              ))}
            </div>
          )}

          <form className="settings-form" onSubmit={async (e) => {
            e.preventDefault()
            const name = newWebhookName.trim(); const url = newWebhookUrl.trim()
            if (!name) { setError('Name is required.'); return }
            if (!isProbablyWebhook(url)) { setError('A valid webhook URL is required.'); return }
            const next = { id: crypto.randomUUID(), name, url }
            const updated: NotificationSettings = {
              ...settings,
              webhooks: { enabled: true, destinations: [...(settings.webhooks?.destinations ?? []), next] },
            }
            setNewWebhookName(''); setNewWebhookUrl('')
            setBusy(true); setError(''); setNote('')
            await persistSettings(updated)
            setBusy(false)
          }}>
            <div className="form-group">
              <label>Destination Name</label>
              <input type="text" placeholder="e.g. PagerDuty" value={newWebhookName} onChange={(e) => setNewWebhookName(e.target.value)} />
            </div>
            <div className="form-group">
              <label>Webhook URL</label>
              <input type="url" placeholder="https://..." value={newWebhookUrl} onChange={(e) => setNewWebhookUrl(e.target.value)} />
            </div>
            <button type="submit" className="btn-rr" disabled={busy}>Add Webhook Destination</button>
          </form>
        </div>
      )}

      {/* ── Delivery Logs tab ── */}
      {activeTab === 'deliveries' && (
        <div className="rr-card">
          <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-1">Delivery Logs</h2>
          <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">Recent notification delivery attempts. Reload to refresh.</p>
          <button type="button" className="btn-rr-sm mb-5" onClick={() => {
            setDeliveryLogsLoading(true)
            void fetchDeliveryLogs(undefined, 100).then(setDeliveryLogs).finally(() => setDeliveryLogsLoading(false))
          }}>Refresh</button>
          {deliveryLogsLoading && <p className="text-sm text-[var(--text-light)]">Loading…</p>}
          {!deliveryLogsLoading && deliveryLogs.length === 0 && (
            <p className="text-sm text-[var(--text-light)]">No delivery logs yet. Deliveries appear here once alert rules fire.</p>
          )}
          {deliveryLogs.length > 0 && (
            <div className="overflow-x-auto">
              <table className="w-full text-xs border-collapse">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left">
                    {['Status', 'Channel', 'Destination', 'Rule', 'Job', 'Repository', 'Sent'].map((h) => (
                      <th key={h} className="py-2 pr-4 font-deco tracking-widest text-[var(--text-light)] whitespace-nowrap">{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {deliveryLogs.map((log) => (
                    <tr key={log.id} className="border-b border-[var(--border)] hover:bg-[var(--cream-alt)]">
                      <td className="py-2 pr-4">
                        <span className={`inline-block px-2 py-0.5 rounded text-[10px] font-deco tracking-wider ${log.status === 'delivered' ? 'bg-[rgba(46,125,50,.10)] text-[#2E7D32]' : 'bg-[rgba(194,59,34,.10)] text-[var(--red)]'}`}>
                          {log.status}
                        </span>
                      </td>
                      <td className="py-2 pr-4 text-[var(--text-mid)]">{log.channel}</td>
                      <td className="py-2 pr-4 text-[var(--text-mid)] max-w-[120px] truncate">{log.destination_id}</td>
                      <td className="py-2 pr-4 text-[var(--text-mid)] max-w-[120px] truncate">{log.rule_id || '—'}</td>
                      <td className="py-2 pr-4 text-[var(--text-mid)] max-w-[100px] truncate">{log.job_id || '—'}</td>
                      <td className="py-2 pr-4 text-[var(--text-mid)] max-w-[120px] truncate">{log.repository || '—'}</td>
                      <td className="py-2 pr-4 text-[var(--text-light)] whitespace-nowrap">{new Date(log.sent_at).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* ── Ownership tab ── */}
      {activeTab === 'ownership' && (() => {
        // Load on first open.
        if (ownershipEntries.length === 0) {
          void fetchOwnership().then(setOwnershipEntries).catch(() => {})
        }
        // All destination IDs across all channels for the picker.
        const allDests: Array<{ id: string; name: string; kind: string }> = [
          ...(settings.slack.destinations ?? []).map((d) => ({ id: d.id, name: d.name, kind: 'slack' })),
          ...(settings.teams?.destinations ?? []).map((d) => ({ id: d.id, name: d.name, kind: 'teams' })),
          ...(settings.webhooks?.destinations ?? []).map((d) => ({ id: d.id, name: d.name, kind: 'webhook' })),
        ]
        return (
          <div className="rr-card">
            <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-1">Repository Ownership</h2>
            <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
              Map repositories to team names and notification destinations. When an alert fires for a matched repository,
              RunRight automatically routes to these destinations in addition to any rule-configured ones.
            </p>

            {ownershipEntries.length > 0 && (
              <div className="divide-y divide-[var(--border)] border border-[var(--border)] rounded mb-6">
                {ownershipEntries.map((entry) => (
                  <div key={`${entry.repository}:${entry.team_name}`} className="flex items-start justify-between px-4 py-3 gap-3">
                    <div className="min-w-0">
                      <div className="font-semibold text-sm text-[var(--text)] truncate">{entry.repository}</div>
                      <div className="text-xs text-[var(--text-mid)] mt-0.5">Team: {entry.team_name}</div>
                      <div className="text-xs text-[var(--text-light)] mt-0.5">
                        {entry.destination_ids.length === 0
                          ? 'No destinations'
                          : entry.destination_ids.map((id) => allDests.find((d) => d.id === id)?.name ?? id).join(', ')}
                      </div>
                    </div>
                    <button type="button"
                      className="flex-shrink-0 text-xs font-deco tracking-widest text-[var(--red)] border border-[var(--red)] px-3 py-1 rounded hover:bg-[rgba(194,59,34,.06)]"
                      onClick={() => {
                        setBusy(true)
                        void deleteOwnership(entry.repository, entry.team_name)
                          .then(() => fetchOwnership().then(setOwnershipEntries))
                          .catch(() => setError('Failed to remove ownership entry.'))
                          .finally(() => setBusy(false))
                      }}
                    >Remove</button>
                  </div>
                ))}
              </div>
            )}

            <form className="settings-form" onSubmit={(e) => {
              e.preventDefault()
              const repo = newOwnershipRepo.trim(); const team = newOwnershipTeam.trim()
              if (!repo || !team) { setError('Repository and team name are required.'); return }
              setBusy(true); setError(''); setNote('')
              void upsertOwnership({ repository: repo, team_name: team, destination_ids: newOwnershipDestIds })
                .then(() => fetchOwnership().then(setOwnershipEntries))
                .then(() => { setNewOwnershipRepo(''); setNewOwnershipTeam(''); setNewOwnershipDestIds([]) })
                .catch(() => setError('Failed to save ownership entry.'))
                .finally(() => setBusy(false))
            }}>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="form-group">
                  <label>Repository</label>
                  <input type="text" placeholder="owner/repo" value={newOwnershipRepo} onChange={(e) => setNewOwnershipRepo(e.target.value)} />
                </div>
                <div className="form-group">
                  <label>Team Name</label>
                  <input type="text" placeholder="e.g. platform-team" value={newOwnershipTeam} onChange={(e) => setNewOwnershipTeam(e.target.value)} />
                </div>
              </div>
              {allDests.length > 0 && (
                <div className="form-group">
                  <label>Route alerts to</label>
                  <div className="flex flex-wrap gap-2 mt-1">
                    {allDests.map((d) => (
                      <label key={d.id} className="flex items-center gap-1.5 text-sm cursor-pointer">
                        <input type="checkbox"
                          checked={newOwnershipDestIds.includes(d.id)}
                          onChange={(e) => setNewOwnershipDestIds((prev) =>
                            e.target.checked ? [...prev, d.id] : prev.filter((id) => id !== d.id)
                          )}
                        />
                        <span>{d.name} <span className="text-[var(--text-light)] text-xs">({d.kind})</span></span>
                      </label>
                    ))}
                  </div>
                </div>
              )}
              {allDests.length === 0 && (
                <p className="text-sm text-[var(--text-light)] mb-4">
                  No destinations configured yet.{' '}
                  <button type="button" className="underline underline-offset-2" onClick={() => setActiveTab('destinations')}>Add a Slack destination first.</button>
                </p>
              )}
              <button type="submit" className="btn-rr" disabled={busy}>Save Ownership Rule</button>
            </form>
          </div>
        )
      })()}
    </div>
  )
}

