export type MonitoringChartRange = '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'

export type MonitoringCheck = {
  checked_at?: string
  duration_ms?: number
  status?: string
}

export type MonitoringChartPoint = {
  ts: number
  ms: number
  rawMs: number
}

const HOLE_VALUE = Number.NaN

export function parseMonitoringTime(value?: string): number {
  if (!value) return Date.now()
  const ts = Date.parse(value)
  if (Number.isNaN(ts)) return Date.now()
  return ts
}

export function monitoringRangeDurationMs(value: MonitoringChartRange): number {
  if (value === '15m') return 15 * 60 * 1000
  if (value === '1h') return 60 * 60 * 1000
  if (value === '3h') return 3 * 60 * 60 * 1000
  if (value === '6h') return 6 * 60 * 60 * 1000
  if (value === '12h') return 12 * 60 * 60 * 1000
  if (value === '7d') return 7 * 24 * 60 * 60 * 1000
  if (value === '30d') return 30 * 24 * 60 * 60 * 1000
  return 24 * 60 * 60 * 1000
}

export function buildMonitoringChartData(
  checks: MonitoringCheck[],
  range: MonitoringChartRange,
  nowTs = Date.now(),
): MonitoringChartPoint[] {
  const horizonMs = monitoringRangeDurationMs(range)
  const mapped: Array<{ ts: number; ms: number }> = []
  let looksAscending = true
  let looksDescending = true
  let prevTs: number | null = null

  for (const row of checks) {
    const ts = parseMonitoringTime(row.checked_at)
    const status = String(row.status || '').toLowerCase()
    const ms = status === 'paused' || status === 'maintenance' ? 0 : Math.max(0, Number(row.duration_ms || 0))
    mapped.push({ ts, ms })
    if (prevTs !== null) {
      if (ts < prevTs) looksAscending = false
      if (ts > prevTs) looksDescending = false
    }
    prevTs = ts
  }

  if (!looksAscending) {
    if (looksDescending) {
      mapped.reverse()
    } else {
      mapped.sort((a, b) => a.ts - b.ts)
    }
  }

  if (!mapped.length) {
    return []
  }

  const endTs = Math.max(nowTs, mapped[mapped.length - 1].ts)
  const startTs = endTs - horizonMs
  const windowRows = mapped.filter((row) => row.ts >= startTs && row.ts <= endTs)
  if (!windowRows.length) {
    return []
  }

  const merged = new Map<number, number>()
  for (const row of windowRows) {
    merged.set(row.ts, row.ms)
  }
  const normalized = Array.from(merged.entries())
    .map(([ts, ms]) => ({ ts, ms }))
    .sort((a, b) => a.ts - b.ts)

  if (!normalized.length) {
    return []
  }

  const deltas: number[] = []
  for (let i = 1; i < normalized.length; i++) {
    const delta = normalized[i].ts - normalized[i - 1].ts
    if (delta > 0) deltas.push(delta)
  }
  const sortedDeltas = [...deltas].sort((a, b) => a - b)
  const medianDelta = sortedDeltas.length
    ? sortedDeltas[Math.floor(sortedDeltas.length / 2)]
    : 60_000

  // Base gap detection on the median observed cadence so a handful of short
  // retry bursts do not fragment wider time ranges into isolated single points.
  const pollingIntervalMs = Math.max(1_000, medianDelta)
  const nullGapThresholdMs = Math.max(15_000, pollingIntervalMs * 5)
  const carryThresholdMs = Math.max(30_000, Math.min(10 * 60 * 1000, medianDelta * 3))

  const filled = [...normalized]
  if (filled[0].ts > startTs) {
    const leftGapMs = filled[0].ts - startTs
    if (leftGapMs <= carryThresholdMs) {
      filled.unshift({ ts: startTs, ms: filled[0].ms })
    } else {
      const leftStopTs = Math.max(startTs, filled[0].ts - 1)
      filled.unshift({ ts: leftStopTs, ms: HOLE_VALUE })
      filled.unshift({ ts: startTs, ms: HOLE_VALUE })
    }
  }
  if (filled[filled.length - 1].ts < endTs) {
    const rightGapMs = endTs - filled[filled.length - 1].ts
    if (rightGapMs <= carryThresholdMs) {
      filled.push({ ts: endTs, ms: filled[filled.length - 1].ms })
    } else {
      const rightStartTs = Math.min(endTs, filled[filled.length - 1].ts + 1)
      filled.push({ ts: rightStartTs, ms: HOLE_VALUE })
      filled.push({ ts: endTs, ms: HOLE_VALUE })
    }
  }

  const segmented: MonitoringChartPoint[] = []
  for (let i = 0; i < filled.length; i++) {
    const point = filled[i]
    if (segmented.length > 0) {
      const prev = filled[i - 1]
      const delta = point.ts - prev.ts
      if (delta > nullGapThresholdMs) {
        const leftBreakTs = Math.min(point.ts - 1, prev.ts + 1)
        const rightBreakTs = Math.max(prev.ts + 1, point.ts - 1)
        segmented.push({ ts: leftBreakTs, ms: HOLE_VALUE, rawMs: HOLE_VALUE })
        segmented.push({ ts: rightBreakTs, ms: HOLE_VALUE, rawMs: HOLE_VALUE })
      }
    }
    segmented.push({ ts: point.ts, ms: point.ms, rawMs: point.ms })
  }

  return segmented
}
