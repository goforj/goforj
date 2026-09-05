import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MonitorDetailPanel from '@/components/MonitorDetailPanel.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

type Scenario = {
  name: string
  monitorEnabled: boolean
  checks: Array<{
    id: string
    checked_at: string
    status: string
    status_code: number
    duration_ms: number
    error_message: string
  }>
}

const baseMonitor = {
  id: 'monitor_cloudflare',
  name: 'Cloudflare',
  type: 'http',
  target: 'https://www.cloudflare.com',
  interval_seconds: 60,
  timeout_ms: 5000,
  enabled: true,
  updated_at: '2026-02-07T12:00:00Z',
}

const baseIncidents = [
  {
    id: 'inc_1',
    opened_at: '2026-02-07T11:50:00Z',
    resolved_at: null,
    summary: 'Probe timeout spike',
  },
]

const baseStats = {
  sample_count: 40,
  uptime_pct: 99.2,
  p50_ms: 190,
  p95_ms: 340,
}

function mountScenario(s: Scenario) {
  return mount(MonitorDetailPanel, {
    props: {
      monitor: { ...baseMonitor, enabled: s.monitorEnabled },
      heartbeatStatuses: ['unknown', 'up', 'up', 'down', 'paused', 'up'],
      heartbeatPoints: [
        { status: 'unknown' },
        { status: 'up', checked_at: '2026-02-07T11:55:00Z', latency_ms: 180 },
        { status: 'up', checked_at: '2026-02-07T11:56:00Z', latency_ms: 210 },
        { status: 'down', checked_at: '2026-02-07T11:57:00Z', latency_ms: 0 },
        { status: 'paused', checked_at: '2026-02-07T11:58:00Z', latency_ms: 0 },
        { status: 'up', checked_at: '2026-02-07T11:59:00Z', latency_ms: 175 },
      ],
      checks: s.checks,
      checkRange: '1h',
      incidents: baseIncidents,
      stats: baseStats,
    },
    global: {
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        ChartAreaInteractive: { template: '<div data-test="chart-stub" />' },
        HeartbeatStrip: { template: '<div data-test="heartbeat-stub" />' },
      },
    },
  })
}

describe('MonitorDetailPanel snapshot baselines', () => {
  const scenarios: Scenario[] = [
    {
      name: 'up',
      monitorEnabled: true,
      checks: [
        {
          id: 'check_up',
          checked_at: '2026-02-07T11:59:00Z',
          status: 'up',
          status_code: 200,
          duration_ms: 165,
          error_message: '',
        },
      ],
    },
    {
      name: 'down',
      monitorEnabled: true,
      checks: [
        {
          id: 'check_down',
          checked_at: '2026-02-07T11:59:00Z',
          status: 'down',
          status_code: 503,
          duration_ms: 0,
          error_message: 'timeout',
        },
      ],
    },
    {
      name: 'paused',
      monitorEnabled: false,
      checks: [
        {
          id: 'check_paused',
          checked_at: '2026-02-07T11:59:00Z',
          status: 'paused',
          status_code: 0,
          duration_ms: 0,
          error_message: '',
        },
      ],
    },
    {
      name: 'gap',
      monitorEnabled: true,
      checks: [],
    },
  ]

  for (const scenario of scenarios) {
    it(`matches ${scenario.name} baseline`, () => {
      const wrapper = mountScenario(scenario)
      expect(wrapper.html()).toMatchSnapshot()
    })
  }
})
