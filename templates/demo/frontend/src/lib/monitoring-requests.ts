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
