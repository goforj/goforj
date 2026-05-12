import { apiFetch } from '@/lib/auth'
import type { NotificationProvider } from '@/lib/notification-providers'
import { i18n } from '@/i18n'
type AnyJSON = Record<string, unknown>

export type NotificationChannel = {
  id: number
  name: string
  provider: NotificationProvider
  is_enabled: boolean
  config: Record<string, string>
  has_secrets: boolean
  secrets_present: string[]
}

export type UpsertNotificationChannelPayload = {
  name: string
  provider: NotificationProvider
  is_enabled: boolean
  config: Record<string, string>
  secrets?: Record<string, string>
}

const inFlight = new Map<string, Promise<AnyJSON>>()
const DEFAULT_TIMEOUT_MS = 10_000

async function dedupedJSONFetch(key: string, input: RequestInfo | URL, init?: RequestInit): Promise<AnyJSON> {
  const existing = inFlight.get(key)
  if (existing) {
    return existing
  }
  const request = (async () => {
    const res = await apiFetch(input, init)
    if (!res.ok) {
      return {}
    }
    return (await res.json()) as AnyJSON
  })()
  inFlight.set(key, request)
  try {
    return await request
  } finally {
    inFlight.delete(key)
  }
}

async function jsonFetchWithTimeout(
  input: RequestInfo | URL,
  init?: RequestInit,
  timeoutMs: number = DEFAULT_TIMEOUT_MS,
): Promise<AnyJSON> {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), timeoutMs)
  try {
    const res = await apiFetch(input, { ...init, signal: controller.signal })
    if (!res.ok) {
      return {}
    }
    return (await res.json()) as AnyJSON
  } catch {
    return {}
  } finally {
    window.clearTimeout(timeout)
  }
}

export async function fetchMonitors() {
  return jsonFetchWithTimeout('/api/v1/monitoring/monitors')
}

export async function fetchSidebarMonitors() {
  return jsonFetchWithTimeout('/api/v1/monitoring/monitors/sidebar')
}

export async function fetchHeartbeats(limit: number) {
  return jsonFetchWithTimeout(`/api/v1/monitoring/heartbeats?limit=${limit}`)
}

export async function fetchHeartbeatsForMonitorIDs(ids: string[], limit: number) {
  const unique = Array.from(new Set(ids.map((id) => String(id || '').trim()).filter(Boolean)))
  if (!unique.length) {
    return { ok: true, limit, heartbeats: {}, heartbeat_points: {} }
  }
  const params = new URLSearchParams({
    limit: String(limit),
    ids: unique.join(','),
  })
  return jsonFetchWithTimeout(`/api/v1/monitoring/heartbeats?${params.toString()}`)
}

export async function fetchMonitorDashboard(id: string, range: '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d') {
  // Do not dedupe dashboard calls; if a prior request hangs (background tab/network flap),
  // we still want the next timer/focus refresh to issue a fresh request.
  return jsonFetchWithTimeout(
    `/api/v1/monitoring/monitors/${id}/dashboard?range=${range}&_ts=${Date.now()}`,
    { cache: 'no-store' },
  )
}

export async function fetchMonitoringSettings() {
  const res = await apiFetch('/api/v1/monitoring/settings', { cache: 'no-store' })
  if (!res.ok) return {}
  return (await res.json()) as AnyJSON
}

export type MonitoringSettingsUpdatePayload = {
  favicon_cache_ttl_seconds?: number
  monitoring_retention_raw_days?: number
  monitoring_retention_downsample_hourly_after_days?: number
  monitoring_retention_downsample_daily_after_days?: number
  monitoring_retention_hourly_rollup_days?: number
  monitoring_retention_daily_rollup_days?: number
  monitoring_retention_alert_dispatch_days?: number
  monitoring_retention_resolved_incident_days?: number
  monitoring_poll_batch_size?: number
  monitoring_maintenance_starts_at?: string | null
  monitoring_maintenance_ends_at?: string | null
}

export async function updateMonitoringSettings(payload: MonitoringSettingsUpdatePayload) {
  const res = await apiFetch('/api/v1/monitoring/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(typeof data?.error === 'string' ? data.error : i18n.global.t('settings.failedSave'))
  }
  return (await res.json()) as AnyJSON
}

export async function clearMonitoringFaviconCache() {
  const res = await apiFetch('/api/v1/monitoring/settings/favicon-cache/clear', {
    method: 'POST',
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(typeof data?.error === 'string' ? data.error : i18n.global.t('settings.failedClearCache'))
  }
  return (await res.json()) as AnyJSON
}

export async function fetchNotificationChannels() {
  const res = await apiFetch('/api/v1/monitoring/settings/notification-channels', { cache: 'no-store' })
  if (!res.ok) {
    throw new Error(i18n.global.t('settings.channels.loadFailed'))
  }
  return (await res.json()) as AnyJSON
}

export async function createNotificationChannel(payload: UpsertNotificationChannelPayload) {
  const res = await apiFetch('/api/v1/monitoring/settings/notification-channels', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(typeof data?.error === 'string' ? data.error : i18n.global.t('settings.channels.createFailed'))
  }
  return (await res.json()) as AnyJSON
}

export async function updateNotificationChannel(id: number, payload: UpsertNotificationChannelPayload) {
  const res = await apiFetch(`/api/v1/monitoring/settings/notification-channels/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(typeof data?.error === 'string' ? data.error : i18n.global.t('settings.channels.saveFailed'))
  }
  return (await res.json()) as AnyJSON
}

export async function deleteNotificationChannel(id: number) {
  const res = await apiFetch(`/api/v1/monitoring/settings/notification-channels/${id}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(typeof data?.error === 'string' ? data.error : i18n.global.t('settings.channels.deleteFailed'))
  }
  return (await res.json()) as AnyJSON
}
