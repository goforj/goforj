import { describe, expect, it } from 'vitest'
import { normalizeHeartbeatPills, shouldUseHeartbeatVisualBackfill } from '@/lib/heartbeat-pills'

describe('normalizeHeartbeatPills', () => {
  it('pads missing history on the left with unknown placeholders', () => {
    const result = normalizeHeartbeatPills(
      ['up', 'down'],
      [
        { status: 'up', checked_at: '2026-04-18T20:00:00Z', latency_ms: 120 },
        { status: 'down', checked_at: '2026-04-18T20:01:00Z', latency_ms: 0 },
      ],
      5,
    )

    expect(result.statuses).toEqual(['unknown', 'unknown', 'unknown', 'up', 'down'])
    expect(result.points.slice(0, 3)).toEqual([null, null, null])
  })

  it('drops the still-open newest unknown bucket when it has no point payload', () => {
    const result = normalizeHeartbeatPills(
      ['up', 'down', 'unknown'],
      [
        { status: 'up', checked_at: '2026-04-18T20:00:00Z', latency_ms: 120 },
        { status: 'down', checked_at: '2026-04-18T20:01:00Z', latency_ms: 0 },
      ],
      3,
    )

    expect(result.statuses).toEqual(['unknown', 'up', 'down'])
    expect(result.points[2]?.checkedAt).toBe('2026-04-18T20:01:00Z')
  })

  it('keeps the newest targetCount pills when the server returns a longer run', () => {
    const result = normalizeHeartbeatPills(
      ['unknown', 'up', 'down', 'up', 'paused'],
      [
        { status: 'unknown' },
        { status: 'up', checked_at: '2026-04-18T19:57:00Z', latency_ms: 111 },
        { status: 'down', checked_at: '2026-04-18T19:58:00Z', latency_ms: 0 },
        { status: 'up', checked_at: '2026-04-18T19:59:00Z', latency_ms: 112 },
        { status: 'paused', checked_at: '2026-04-18T20:00:00Z', latency_ms: 0 },
      ],
      3,
    )

    expect(result.statuses).toEqual(['down', 'up', 'paused'])
    expect(result.points.map((point) => point?.status)).toEqual(['down', 'up', 'paused'])
  })

  it('uses point status and normalized latency when status-only data is missing', () => {
    const result = normalizeHeartbeatPills(
      [],
      [
        { status: 'UP', checked_at: '2026-04-18T20:00:00Z', latency_ms: 125 },
        { status: 'maintenance', checked_at: '2026-04-18T20:01:00Z', latency_ms: null },
      ],
      2,
    )

    expect(result.statuses).toEqual(['up', 'maintenance'])
    expect(result.points[0]?.latencyMs).toBe(125)
    expect(result.points[1]?.latencyMs).toBe(0)
  })

  it('allows visual backfill when the latest real heartbeat is still fresh', () => {
    const now = new Date('2026-04-18T20:03:00Z').getTime()
    const points = [
      null,
      null,
      { status: 'up', checkedAt: '2026-04-18T20:00:00Z', latencyMs: 120 },
      { status: 'up', checkedAt: '2026-04-18T20:01:00Z', latencyMs: 118 },
      { status: 'up', checkedAt: '2026-04-18T20:02:00Z', latencyMs: 115 },
    ]

    expect(shouldUseHeartbeatVisualBackfill(points, now)).toBe(true)
  })

  it('disables visual backfill when the latest real heartbeat is stale', () => {
    const now = new Date('2026-04-18T20:10:00Z').getTime()
    const points = [
      null,
      null,
      { status: 'up', checkedAt: '2026-04-18T20:00:00Z', latencyMs: 120 },
      { status: 'up', checkedAt: '2026-04-18T20:01:00Z', latencyMs: 118 },
      { status: 'up', checkedAt: '2026-04-18T20:02:00Z', latencyMs: 115 },
    ]

    expect(shouldUseHeartbeatVisualBackfill(points, now)).toBe(false)
  })
})
