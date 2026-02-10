type AnyJSON = Record<string, unknown>

const inFlight = new Map<string, Promise<AnyJSON>>()

async function dedupedJSONFetch(key: string, input: RequestInfo | URL, init?: RequestInit): Promise<AnyJSON> {
  const existing = inFlight.get(key)
  if (existing) {
    return existing
  }
  const request = (async () => {
    const res = await fetch(input, init)
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

export async function fetchMonitors() {
  return dedupedJSONFetch('monitoring:monitors', '/api/v1/monitoring/monitors')
}

export async function fetchHeartbeats(limit: number) {
  return dedupedJSONFetch(`monitoring:heartbeats:${limit}`, `/api/v1/monitoring/heartbeats?limit=${limit}`)
}

export async function fetchMonitorDashboard(id: string, range: '15m' | '1h' | '24h' | '7d' | '30d') {
  return dedupedJSONFetch(
    `monitoring:monitor-dashboard:${id}:${range}`,
    `/api/v1/monitoring/monitors/${id}/dashboard?range=${range}&_ts=${Date.now()}`,
    { cache: 'no-store' },
  )
}

export async function fetchMonitoringSettings() {
  const res = await fetch('/api/v1/monitoring/settings', { cache: 'no-store' })
  if (!res.ok) return {}
  return (await res.json()) as AnyJSON
}

export async function updateMonitoringSettings(payload: { favicon_cache_ttl_seconds: number }) {
  const res = await fetch('/api/v1/monitoring/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(typeof data?.error === 'string' ? data.error : 'Failed to update settings')
  }
  return (await res.json()) as AnyJSON
}

export async function clearMonitoringFaviconCache() {
  const res = await fetch('/api/v1/monitoring/settings/favicon-cache/clear', {
    method: 'POST',
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(typeof data?.error === 'string' ? data.error : 'Failed to clear favicon cache')
  }
  return (await res.json()) as AnyJSON
}
