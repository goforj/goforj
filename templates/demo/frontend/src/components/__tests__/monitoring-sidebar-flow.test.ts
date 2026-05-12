import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import NavMonitors from '@/components/NavMonitors.vue'
import MonitoringView from '@/views/MonitoringView.vue'

const mocks = vi.hoisted(() => ({
  fetchSidebarMonitors: vi.fn(),
  fetchHeartbeatsForMonitorIDs: vi.fn(),
  fetchMonitorDashboard: vi.fn(),
  subscribeMonitoringStatusEvents: vi.fn(() => () => {}),
  subscribeMonitoringSettingsUpdated: vi.fn(() => () => {}),
  applyMonitorStatusSnapshot: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' },
  }),
}))

vi.mock('@/lib/monitoring-requests', () => ({
  fetchSidebarMonitors: mocks.fetchSidebarMonitors,
  fetchHeartbeatsForMonitorIDs: mocks.fetchHeartbeatsForMonitorIDs,
  fetchMonitorDashboard: mocks.fetchMonitorDashboard,
}))

vi.mock('@/lib/monitoring-live', () => ({
  subscribeMonitoringStatusEvents: mocks.subscribeMonitoringStatusEvents,
  applyMonitorStatusSnapshot: mocks.applyMonitorStatusSnapshot,
}))

vi.mock('@/lib/monitoring-settings-events', () => ({
  subscribeMonitoringSettingsUpdated: mocks.subscribeMonitoringSettingsUpdated,
}))

vi.mock('@/lib/monitor-icons', () => ({
  monitorSupportsFavicon: () => true,
  monitorTypeIcon: () => ({ template: '<span data-test="monitor-icon" />' }),
}))

vi.mock('@/components/ui/sidebar', () => ({
  useSidebar: () => ({ state: { value: 'expanded' } }),
  SidebarGroup: { template: '<div><slot /></div>' },
  SidebarGroupContent: { template: '<div><slot /></div>' },
  SidebarGroupLabel: { template: '<div><slot /></div>' },
  SidebarMenu: { template: '<ul><slot /></ul>' },
  SidebarMenuButton: { template: '<div><slot /></div>' },
  SidebarMenuItem: { template: '<li><slot /></li>' },
}))

function makeMonitors(count: number, start: number = 1) {
  return Array.from({ length: count }, (_, index) => {
    const id = String(start + index)
    return {
      id,
      name: `Monitor ${id}`,
      type: 'http',
      enabled: true,
      last_status: 'up',
      target_display: `https://host-${id}.example.com`,
    }
  })
}

function createTestRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/monitors/:id?',
        component: { template: '<div />' },
      },
    ],
  })
}

beforeEach(() => {
  mocks.fetchSidebarMonitors.mockReset()
  mocks.fetchHeartbeatsForMonitorIDs.mockReset()
  mocks.fetchMonitorDashboard.mockReset()
  mocks.subscribeMonitoringStatusEvents.mockClear()
  mocks.subscribeMonitoringSettingsUpdated.mockClear()
  mocks.applyMonitorStatusSnapshot.mockReset()

  global.ResizeObserver = class {
    observe() {}
    disconnect() {}
    unobserve() {}
  } as unknown as typeof ResizeObserver
})

describe('monitoring sidebar flow regressions', () => {
  it('loads another sidebar page when the viewport still has room', async () => {
    const router = createTestRouter()
    await router.push('/monitors')
    await router.isReady()

    mocks.fetchSidebarMonitors
      .mockResolvedValueOnce({
        monitors: makeMonitors(200, 1),
        has_more: true,
        next_offset: 200,
      })
      .mockResolvedValueOnce({
        monitors: makeMonitors(20, 201),
        has_more: false,
        next_offset: 220,
      })
    mocks.fetchHeartbeatsForMonitorIDs.mockResolvedValue({
      ok: true,
      heartbeats: {},
      heartbeat_points: {},
    })

    const wrapper = mount(NavMonitors, {
      global: {
        plugins: [router],
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :href="typeof to === \'string\' ? to : \'#\'"><slot /></a>',
          },
          Badge: { template: '<span><slot /></span>' },
          Button: { template: '<button><slot /></button>' },
          Input: {
            props: ['modelValue'],
            emits: ['update:modelValue'],
            template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
          },
          Skeleton: { template: '<div><slot /></div>' },
          HeartbeatStrip: { template: '<div data-test="heartbeat-strip" />' },
          ChevronDown: { template: '<span />' },
          ChevronRight: { template: '<span />' },
          CirclePause: { template: '<span />' },
          HeartPulse: { template: '<span />' },
          Pause: { template: '<span />' },
          Plus: { template: '<span />' },
          Server: { template: '<span />' },
          ShieldAlert: { template: '<span />' },
          X: { template: '<span />' },
        },
      },
    })

    await flushPromises()
    await flushPromises()

    expect(mocks.fetchSidebarMonitors).toHaveBeenCalledTimes(2)
    expect(mocks.fetchSidebarMonitors).toHaveBeenNthCalledWith(1, 0, 200, { q: '', state: 'all' })
    expect(mocks.fetchSidebarMonitors).toHaveBeenNthCalledWith(2, 200, 200, { q: '', state: 'all' })

    wrapper.unmount()
  })

  it('reloads heartbeats when the selected monitor route changes', async () => {
    const router = createTestRouter()
    await router.push('/monitors/1')
    await router.isReady()

    mocks.fetchMonitorDashboard.mockImplementation(async (monitorID: string) => ({
      monitor: {
        id: monitorID,
        name: `Monitor ${monitorID}`,
        type: 'http',
        enabled: true,
        interval_seconds: 60,
      },
      checks: [],
      stats: null,
      incidents: [],
    }))
    mocks.fetchHeartbeatsForMonitorIDs.mockImplementation(async (ids: string[]) => ({
      ok: true,
      heartbeats: { [ids[0]]: ['up', 'up'] },
      heartbeat_points: { [ids[0]]: [{ status: 'up' }, { status: 'up' }] },
    }))
    mocks.fetchSidebarMonitors.mockResolvedValue({
      monitors: [],
      has_more: false,
      next_offset: 0,
    })

    const wrapper = mount(MonitoringView, {
      global: {
        plugins: [router],
        stubs: {
          MonitorDetailPanel: { template: '<div data-test="monitor-detail" />' },
        },
      },
    })

    await flushPromises()
    await new Promise((resolve) => window.setTimeout(resolve, 10))
    await flushPromises()

    mocks.fetchHeartbeatsForMonitorIDs.mockClear()

    await router.push('/monitors/2')
    await flushPromises()

    expect(mocks.fetchHeartbeatsForMonitorIDs).toHaveBeenCalledWith(['2'], 30)
    expect(mocks.fetchMonitorDashboard).toHaveBeenCalledWith('2', '1h')

    wrapper.unmount()
  })
})
