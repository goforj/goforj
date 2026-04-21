import { describe, expect, it } from 'vitest'
import { buildMonitoringChartData } from '@/lib/monitoring-chart'

function iso(ts: number): string {
  return new Date(ts).toISOString()
}

function finitePoints(points: ReturnType<typeof buildMonitoringChartData>) {
  return points.filter((point) => Number.isFinite(point.ms))
}

describe('buildMonitoringChartData', () => {
  it('keeps a 1h series continuous when a few retry bursts create shorter deltas', () => {
    const endTs = Date.UTC(2026, 3, 18, 15, 28, 0)
    const checks: Array<{ checked_at: string; duration_ms: number; status: string }> = []

    for (let minute = 0; minute < 60; minute++) {
      const baseTs = endTs - ((59 - minute) * 60 * 1000)
      checks.push({
        checked_at: new Date(baseTs).toISOString(),
        duration_ms: 240 + (minute % 7) * 15,
        status: 'up',
      })

      if (minute % 12 === 0) {
        checks.push({
          checked_at: new Date(baseTs + 5_000).toISOString(),
          duration_ms: 260,
          status: 'up',
        })
      }
    }

    const points = buildMonitoringChartData(checks, '1h', endTs)
    const holeCount = points.filter((point) => Number.isNaN(point.ms)).length
    const renderableCount = points.filter((point) => Number.isFinite(point.ms)).length

    expect(renderableCount).toBe(66)
    expect(holeCount).toBe(0)
  })

  it('still inserts holes for genuinely large gaps', () => {
    const endTs = Date.UTC(2026, 3, 18, 15, 28, 0)
    const checks = [
      { checked_at: new Date(endTs - 40 * 60 * 1000).toISOString(), duration_ms: 220, status: 'up' },
      { checked_at: new Date(endTs - 39 * 60 * 1000).toISOString(), duration_ms: 230, status: 'up' },
      { checked_at: new Date(endTs - 5 * 60 * 1000).toISOString(), duration_ms: 240, status: 'up' },
      { checked_at: new Date(endTs - 4 * 60 * 1000).toISOString(), duration_ms: 250, status: 'up' },
    ]

    const points = buildMonitoringChartData(checks, '1h', endTs)
    const holeCount = points.filter((point) => Number.isNaN(point.ms)).length

    expect(holeCount).toBeGreaterThan(0)
  })

  it('sorts unsorted rows into ascending chart order', () => {
    const endTs = Date.UTC(2026, 3, 18, 15, 28, 0)
    const checks = [
      { checked_at: iso(endTs - 1 * 60 * 1000), duration_ms: 230, status: 'up' },
      { checked_at: iso(endTs - 3 * 60 * 1000), duration_ms: 210, status: 'up' },
      { checked_at: iso(endTs - 2 * 60 * 1000), duration_ms: 220, status: 'up' },
    ]

    const points = finitePoints(buildMonitoringChartData(checks, '15m', endTs))
    const actual = points.map((point) => point.ts)

    expect(actual).toEqual([
      endTs - 3 * 60 * 1000,
      endTs - 2 * 60 * 1000,
      endTs - 1 * 60 * 1000,
      endTs,
    ])
  })

  it('deduplicates identical timestamps using the last value seen', () => {
    const endTs = Date.UTC(2026, 3, 18, 15, 28, 0)
    const dupTs = endTs - 2 * 60 * 1000
    const checks = [
      { checked_at: iso(endTs - 4 * 60 * 1000), duration_ms: 200, status: 'up' },
      { checked_at: iso(dupTs), duration_ms: 210, status: 'up' },
      { checked_at: iso(dupTs), duration_ms: 999, status: 'up' },
      { checked_at: iso(endTs - 1 * 60 * 1000), duration_ms: 220, status: 'up' },
    ]

    const points = finitePoints(buildMonitoringChartData(checks, '15m', endTs))
    const duplicate = points.find((point) => point.ts === dupTs)

    expect(duplicate?.rawMs).toBe(999)
  })

  it('normalizes paused and maintenance statuses to zero latency', () => {
    const endTs = Date.UTC(2026, 3, 18, 15, 28, 0)
    const checks = [
      { checked_at: iso(endTs - 3 * 60 * 1000), duration_ms: 180, status: 'up' },
      { checked_at: iso(endTs - 2 * 60 * 1000), duration_ms: 5000, status: 'paused' },
      { checked_at: iso(endTs - 1 * 60 * 1000), duration_ms: 5000, status: 'maintenance' },
    ]

    const points = finitePoints(buildMonitoringChartData(checks, '15m', endTs))
    const pausedPoint = points.find((point) => point.ts === endTs - 2 * 60 * 1000)
    const maintenancePoint = points.find((point) => point.ts === endTs - 1 * 60 * 1000)

    expect(pausedPoint?.rawMs).toBe(0)
    expect(maintenancePoint?.rawMs).toBe(0)
  })

  it('carries the latest value to the window edge when the trailing gap is small', () => {
    const endTs = Date.UTC(2026, 3, 18, 15, 28, 0)
    const checks = [
      { checked_at: iso(endTs - 3 * 60 * 1000), duration_ms: 200, status: 'up' },
      { checked_at: iso(endTs - 1 * 60 * 1000), duration_ms: 210, status: 'up' },
    ]

    const points = finitePoints(buildMonitoringChartData(checks, '15m', endTs))
    const lastPoint = points[points.length - 1]

    expect(lastPoint?.ts).toBe(endTs)
    expect(lastPoint?.rawMs).toBe(210)
  })

  it('adds leading holes instead of carrying when the first point is far from the window start', () => {
    const endTs = Date.UTC(2026, 3, 18, 15, 28, 0)
    const firstTs = endTs - 3 * 60 * 1000
    const checks = [
      { checked_at: iso(firstTs), duration_ms: 200, status: 'up' },
      { checked_at: iso(endTs - 2 * 60 * 1000), duration_ms: 210, status: 'up' },
      { checked_at: iso(endTs - 1 * 60 * 1000), duration_ms: 220, status: 'up' },
    ]

    const points = buildMonitoringChartData(checks, '15m', endTs)
    const firstFinite = points.find((point) => Number.isFinite(point.ms))

    expect(points[0]?.ts).toBe(endTs - 15 * 60 * 1000)
    expect(Number.isNaN(points[0]?.ms)).toBe(true)
    expect(Number.isNaN(points[1]?.ms)).toBe(true)
    expect(firstFinite?.ts).toBe(firstTs)
  })

  it('filters out rows older than the selected range', () => {
    const endTs = Date.UTC(2026, 3, 18, 15, 28, 0)
    const oldTs = endTs - 90 * 60 * 1000
    const recentTs = endTs - 10 * 60 * 1000
    const checks = [
      { checked_at: iso(oldTs), duration_ms: 999, status: 'up' },
      { checked_at: iso(recentTs), duration_ms: 220, status: 'up' },
    ]

    const points = finitePoints(buildMonitoringChartData(checks, '1h', endTs))

    expect(points.some((point) => point.ts === oldTs)).toBe(false)
    expect(points.some((point) => point.ts === recentTs)).toBe(true)
  })
})
