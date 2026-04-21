export type HeartbeatAPIStatus = string | null | undefined

export type HeartbeatAPIStatusPoint = {
  status?: string | null
  checked_at?: string | null
  latency_ms?: number | null
}

export type HeartbeatPillPoint = {
  status?: string
  checkedAt?: string
  latencyMs?: number
}

export type HeartbeatPillData = {
  statuses: string[]
  points: Array<HeartbeatPillPoint | null>
}

export function normalizeHeartbeatPills(
  statusesInput: HeartbeatAPIStatus[] | undefined,
  pointsInput: HeartbeatAPIStatusPoint[] | undefined,
  targetCount: number,
): HeartbeatPillData {
  const statuses = Array.isArray(statusesInput)
    ? statusesInput.map((status) => (status || 'unknown').toLowerCase())
    : []
  const points = Array.isArray(pointsInput)
    ? pointsInput.map((point) => ({
      status: (point?.status || 'unknown').toLowerCase(),
      checkedAt: point?.checked_at || undefined,
      latencyMs: point?.latency_ms == null ? 0 : Number(point.latency_ms),
    }))
    : []

  const count = Math.max(statuses.length, points.length)
  if (!count) {
    return {
      statuses: Array(targetCount).fill('unknown'),
      points: Array(targetCount).fill(null),
    }
  }

  let merged = Array.from({ length: count }, (_, idx) => ({
    status: (statuses[idx] || points[idx]?.status || 'unknown').toLowerCase(),
    point: points[idx] || null,
  }))

  const tail = merged[merged.length - 1]
  if (tail && tail.status === 'unknown' && !tail.point?.checkedAt) {
    merged = merged.slice(0, -1)
  }

  const trimmed = merged.slice(-targetCount)
  const padded = [
    ...Array(Math.max(0, targetCount - trimmed.length)).fill({ status: 'unknown', point: null }),
    ...trimmed,
  ]

  return {
    statuses: padded.map((row) => row.status),
    points: padded.map((row) => row.point),
  }
}
