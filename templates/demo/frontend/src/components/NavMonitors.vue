<script setup lang="ts">
import { computed, h, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ChevronDown, ChevronRight, CirclePause, HeartPulse, Pause, Plus, Server, ShieldAlert, X } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import HeartbeatStrip from '@/components/HeartbeatStrip.vue'
import { normalizeHeartbeatPills } from '@/lib/heartbeat-pills'
import { subscribeMonitoringSettingsUpdated } from '@/lib/monitoring-settings-events'
import { fetchHeartbeatsForMonitorIDs, fetchSidebarMonitors } from '@/lib/monitoring-requests'
import { applyMonitorStatusSnapshot, subscribeMonitoringStatusEvents, type MonitorStatusEvent } from '@/lib/monitoring-live'
import { monitorSupportsFavicon, monitorTypeIcon } from '@/lib/monitor-icons'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'

type Monitor = {
  id?: string
  name?: string
  type?: string
  monitor_type?: string
  target_display?: string
  enabled?: boolean
  last_status?: string
  maintenance_active?: boolean
  maintenance_starts_at?: string
  maintenance_ends_at?: string
}

type HeartbeatPoint = { status?: string; checked_at?: string; latency_ms?: number }

const route = useRoute()
const { state: sidebarState } = useSidebar()
const { t } = useI18n()

const monitors = ref<Monitor[]>([])
const heartbeats = ref<Record<string, string[]>>({})
const heartbeatPoints = ref<Record<string, HeartbeatPoint[]>>({})
const monitorsLoaded = ref(false)
const heartbeatReady = ref(false)
const faviconFailedByID = ref<Record<string, boolean>>({})
const faviconFailedAtByID = ref<Record<string, number>>({})
const faviconLoadedByID = ref<Record<string, boolean>>({})
const faviconRevealedByID = ref<Record<string, boolean>>({})
const query = ref('')
const state = ref<'all' | 'up' | 'down' | 'paused'>('all')
const controlsExpanded = ref(false)
const globalMaintenanceActive = ref(false)
const listViewportRef = ref<HTMLElement | null>(null)
const listScrollTop = ref(0)
const listViewportHeight = ref(0)

const SIDEBAR_PILL_COUNT = 12
const SIDEBAR_ROW_HEIGHT = 32
const SIDEBAR_OVERSCAN = 6
const SIDEBAR_PAGE_SIZE = 200
const SIDEBAR_FAVICON_RETRY_COOLDOWN_MS = 5 * 60 * 1000
const SIDEBAR_PILL_STRIP_WIDTH_REM = 5.75
const monitorToolsExpandedStorageKey = 'uptime-gopher:sidebar:monitor-tools-expanded'

let listResizeObserver: ResizeObserver | null = null
let visibleHeartbeatTimer: number | null = null
let visibleHeartbeatDebounceTimer: number | null = null
let scrollSettledTimer: number | null = null
let refreshOnResumeTimer: number | null = null
let refreshOnResumeBound = false
let unsubscribeMonitoringLive: (() => void) | null = null
let unsubscribeMonitoringSettings: (() => void) | null = null
let visibleHeartbeatRequestInFlight = false
let queuedVisibleHeartbeatIDs: string[] | null = null
let controlsExpandedLoaded = false
let sidebarHasMore = true
let sidebarNextOffset = 0
let sidebarLoadInFlight = false

const collapsed = computed(() => sidebarState.value === 'collapsed')
const selectedMonitorID = computed(() => String(route.params.id || ''))
const hasActiveControls = computed(() => query.value.trim() !== '' || state.value !== 'all')
const sidebarSectionReady = computed(() => monitorsLoaded.value)

function monitorWindowActive(startsAt?: string, endsAt?: string): boolean {
  if (!startsAt || !endsAt) return false
  const startMs = Date.parse(startsAt)
  const endMs = Date.parse(endsAt)
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs)) return false
  const now = Date.now()
  return startMs <= now && now < endMs
}

function monitorMaintenanceActive(monitor: Monitor): boolean {
  return globalMaintenanceActive.value || monitorWindowActive(monitor.maintenance_starts_at, monitor.maintenance_ends_at)
}

function effectiveMonitorStatus(monitor: Monitor): string {
  if (monitor.enabled === false) return 'paused'
  if (monitorMaintenanceActive(monitor)) return 'maintenance'
  return (monitor.last_status || 'unknown').toLowerCase()
}

function monitorDisplayTarget(monitor: Monitor): string {
  return String(monitor.target_display || '').trim()
}

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  return monitors.value.filter((m) => {
    if (q) {
      const haystack = `${m.name || ''} ${monitorDisplayTarget(m) || ''}`.toLowerCase()
      if (!haystack.includes(q)) return false
    }
    const s = effectiveMonitorStatus(m)
    if (state.value === 'up' && (m.enabled === false || s !== 'up')) return false
    if (state.value === 'down' && (m.enabled === false || s === 'up' || s === 'maintenance')) return false
    if (state.value === 'paused' && m.enabled !== false && s !== 'maintenance') return false
    return true
  })
})

const virtualStartIndex = computed(() =>
  Math.max(0, Math.floor(listScrollTop.value / SIDEBAR_ROW_HEIGHT) - SIDEBAR_OVERSCAN),
)

const virtualEndIndex = computed(() =>
  Math.min(
    filtered.value.length,
    Math.ceil((listScrollTop.value + listViewportHeight.value) / SIDEBAR_ROW_HEIGHT) + SIDEBAR_OVERSCAN,
  ),
)

const viewportStartIndex = computed(() =>
  Math.max(0, Math.floor(listScrollTop.value / SIDEBAR_ROW_HEIGHT)),
)

const viewportEndIndex = computed(() =>
  Math.min(
    filtered.value.length,
    Math.ceil((listScrollTop.value + listViewportHeight.value) / SIDEBAR_ROW_HEIGHT),
  ),
)

const virtualMonitors = computed(() =>
  filtered.value.slice(virtualStartIndex.value, virtualEndIndex.value).map((monitor, index) => ({
    monitor,
    absoluteIndex: virtualStartIndex.value + index,
  })),
)

const viewportMonitorIDs = computed(() =>
  filtered.value
    .slice(viewportStartIndex.value, viewportEndIndex.value)
    .map((monitor) => String(monitor.id || '').trim())
    .filter(Boolean),
)

const listTopSpacerHeight = computed(() => virtualStartIndex.value * SIDEBAR_ROW_HEIGHT)
const listBottomSpacerHeight = computed(() =>
  Math.max(0, (filtered.value.length - virtualEndIndex.value) * SIDEBAR_ROW_HEIGHT),
)

function updateListViewportMetrics() {
  const el = listViewportRef.value
  if (!el) return
  listViewportHeight.value = el.clientHeight
  listScrollTop.value = el.scrollTop
}

function bindListViewport() {
  listResizeObserver?.disconnect()
  listResizeObserver = null
  const el = listViewportRef.value
  if (!el) return
  updateListViewportMetrics()
  listResizeObserver = new ResizeObserver(() => updateListViewportMetrics())
  listResizeObserver.observe(el)
}

function onListScroll() {
  updateListViewportMetrics()
  void ensureSidebarPageForViewport()
  if (scrollSettledTimer !== null) {
    window.clearTimeout(scrollSettledTimer)
  }
  scrollSettledTimer = window.setTimeout(() => {
    scrollSettledTimer = null
    scheduleVisibleHeartbeatRefresh(viewportMonitorIDs.value)
  }, 140)
}

function mergeSidebarMonitors(existing: Monitor[], incoming: Monitor[]): Monitor[] {
  const byID = new Map<string, Monitor>()
  for (const monitor of existing) {
    const key = String(monitor.id || '').trim()
    if (!key) continue
    byID.set(key, monitor)
  }
  for (const monitor of incoming) {
    const key = String(monitor.id || '').trim()
    if (!key) continue
    byID.set(key, monitor)
  }
  return Array.from(byID.values())
}

async function loadMonitors(reset: boolean = false) {
  if (sidebarLoadInFlight) return
  if (!reset && !sidebarHasMore) return
  sidebarLoadInFlight = true
  try {
    const offset = reset ? 0 : sidebarNextOffset
    const monitorPayload = await fetchSidebarMonitors(offset, SIDEBAR_PAGE_SIZE, {
      q: query.value,
      state: state.value,
    })
    const rows = Array.isArray(monitorPayload.monitors) ? (monitorPayload.monitors as Monitor[]) : []
    monitors.value = reset ? rows : mergeSidebarMonitors(monitors.value, rows)
    sidebarHasMore = Boolean(monitorPayload.has_more)
    const nextOffset = Number(monitorPayload.next_offset)
    sidebarNextOffset = Number.isFinite(nextOffset) && nextOffset >= 0 ? nextOffset : monitors.value.length
    applyMonitorStatusSnapshot(monitors.value)
  } finally {
    monitorsLoaded.value = true
    sidebarLoadInFlight = false
  }
}

function shouldLoadNextSidebarPage() {
  if (!sidebarHasMore || sidebarLoadInFlight) return false
  const el = listViewportRef.value
  if (!el) return true
  const remaining = el.scrollHeight - (el.scrollTop + el.clientHeight)
  return remaining <= SIDEBAR_ROW_HEIGHT * 20
}

async function ensureSidebarPageForViewport() {
  if (!shouldLoadNextSidebarPage()) return
  await loadMonitors(false)
  await nextTick()
  updateListViewportMetrics()
  if (shouldLoadNextSidebarPage()) {
    await ensureSidebarPageForViewport()
  }
}

function normalizeRequestedMonitorIDs(ids?: string[]) {
  return Array.from(new Set((ids ?? viewportMonitorIDs.value).map((id) => String(id || '').trim()).filter(Boolean)))
}

async function loadVisibleHeartbeats(ids?: string[]) {
  const requested = visibleMonitorIDsMissingHeartbeats(ids)
  if (!requested.length) return
  if (document.visibilityState === 'hidden') return
  if (visibleHeartbeatRequestInFlight) {
    queuedVisibleHeartbeatIDs = requested
    return
  }
  visibleHeartbeatRequestInFlight = true
  try {
    const heartbeatPayload = await fetchHeartbeatsForMonitorIDs(requested, SIDEBAR_PILL_COUNT)
    const nextHeartbeats =
      heartbeatPayload.heartbeats && typeof heartbeatPayload.heartbeats === 'object'
        ? (heartbeatPayload.heartbeats as Record<string, string[]>)
        : {}
    const nextPoints =
      heartbeatPayload.heartbeat_points && typeof heartbeatPayload.heartbeat_points === 'object'
        ? (heartbeatPayload.heartbeat_points as Record<string, HeartbeatPoint[]>)
        : {}
    heartbeats.value = {
      ...heartbeats.value,
      ...nextHeartbeats,
    }
    heartbeatPoints.value = {
      ...heartbeatPoints.value,
      ...nextPoints,
    }
    heartbeatReady.value = true
  } catch {
    heartbeatReady.value = true
  } finally {
    visibleHeartbeatRequestInFlight = false
    const queued = queuedVisibleHeartbeatIDs
    queuedVisibleHeartbeatIDs = null
    if (queued && queued.join(',') !== requested.join(',')) {
      void loadVisibleHeartbeats(queued)
    }
  }
}

function visibleMonitorIDsMissingHeartbeats(ids?: string[]) {
  const requested = normalizeRequestedMonitorIDs(ids)
  return requested.filter((id) => !heartbeats.value[id] || !heartbeatPoints.value[id])
}

function scheduleVisibleHeartbeatRefresh(ids?: string[]) {
  const missing = visibleMonitorIDsMissingHeartbeats(ids)
  if (missing.length) {
    if (visibleHeartbeatDebounceTimer !== null) {
      window.clearTimeout(visibleHeartbeatDebounceTimer)
    }
    visibleHeartbeatDebounceTimer = window.setTimeout(() => {
      visibleHeartbeatDebounceTimer = null
      void loadVisibleHeartbeats(missing)
    }, 120)
    return
  }
  if (visibleHeartbeatDebounceTimer !== null) {
    window.clearTimeout(visibleHeartbeatDebounceTimer)
  }
  visibleHeartbeatDebounceTimer = window.setTimeout(() => {
    visibleHeartbeatDebounceTimer = null
    void loadVisibleHeartbeats(ids)
  }, 80)
}

function applyMonitorStatusEvent(event: MonitorStatusEvent) {
  if (!event.monitor_id) return
  monitors.value = monitors.value.map((monitor) =>
    String(monitor.id || '') === event.monitor_id
      ? { ...monitor, last_status: event.status || monitor.last_status }
      : monitor,
  )
  if (viewportMonitorIDs.value.includes(event.monitor_id)) {
    scheduleVisibleHeartbeatRefresh([event.monitor_id])
  }
}

function refreshOnResume() {
  if (document.visibilityState === 'hidden') return
  if (refreshOnResumeTimer !== null) {
    window.clearTimeout(refreshOnResumeTimer)
  }
  refreshOnResumeTimer = window.setTimeout(() => {
    refreshOnResumeTimer = null
    void loadMonitors(true)
    void ensureSidebarPageForViewport()
    scheduleVisibleHeartbeatRefresh(viewportMonitorIDs.value)
  }, 100)
}

watch([query, state], () => {
  if (hasActiveControls.value) {
    controlsExpanded.value = true
  }
  const el = listViewportRef.value
  if (el) {
    el.scrollTop = 0
  }
  updateListViewportMetrics()
  sidebarHasMore = true
  sidebarNextOffset = 0
  monitors.value = []
  monitorsLoaded.value = false
  void (async () => {
    await loadMonitors(true)
    await nextTick()
    updateListViewportMetrics()
    await ensureSidebarPageForViewport()
  })()
  scheduleVisibleHeartbeatRefresh(viewportMonitorIDs.value)
})

watch(controlsExpanded, (expanded) => {
  if (!controlsExpandedLoaded || typeof window === 'undefined') return
  window.localStorage.setItem(monitorToolsExpandedStorageKey, expanded ? 'true' : 'false')
})

watch(
  () => viewportMonitorIDs.value.join(','),
  () => {
    void ensureSidebarPageForViewport()
    scheduleVisibleHeartbeatRefresh(viewportMonitorIDs.value)
  },
)

watch(collapsed, async () => {
  await nextTick()
  bindListViewport()
  void ensureSidebarPageForViewport()
  scheduleVisibleHeartbeatRefresh(viewportMonitorIDs.value)
})

onMounted(async () => {
  if (typeof window !== 'undefined') {
    const storedControlsExpanded = window.localStorage.getItem(monitorToolsExpandedStorageKey)
    if (storedControlsExpanded === 'true' || storedControlsExpanded === 'false') {
      controlsExpanded.value = storedControlsExpanded === 'true'
    }
    controlsExpandedLoaded = true
  }

  await loadMonitors(true)
  await nextTick()
  bindListViewport()
  await ensureSidebarPageForViewport()
  scheduleVisibleHeartbeatRefresh(viewportMonitorIDs.value)

  visibleHeartbeatTimer = window.setInterval(() => {
    scheduleVisibleHeartbeatRefresh(viewportMonitorIDs.value)
  }, 15000)

  if (!unsubscribeMonitoringLive) {
    unsubscribeMonitoringLive = subscribeMonitoringStatusEvents(applyMonitorStatusEvent)
  }
  if (!unsubscribeMonitoringSettings) {
    unsubscribeMonitoringSettings = subscribeMonitoringSettingsUpdated((maintenance) => {
      globalMaintenanceActive.value = Boolean(maintenance?.active)
      void loadMonitors(true)
      void ensureSidebarPageForViewport()
      scheduleVisibleHeartbeatRefresh(viewportMonitorIDs.value)
    })
  }
  if (!refreshOnResumeBound) {
    window.addEventListener('focus', refreshOnResume)
    document.addEventListener('visibilitychange', refreshOnResume)
    window.addEventListener('pageshow', refreshOnResume)
    refreshOnResumeBound = true
  }
})

onUnmounted(() => {
  if (visibleHeartbeatTimer !== null) {
    window.clearInterval(visibleHeartbeatTimer)
    visibleHeartbeatTimer = null
  }
  if (visibleHeartbeatDebounceTimer !== null) {
    window.clearTimeout(visibleHeartbeatDebounceTimer)
    visibleHeartbeatDebounceTimer = null
  }
  if (scrollSettledTimer !== null) {
    window.clearTimeout(scrollSettledTimer)
    scrollSettledTimer = null
  }
  if (refreshOnResumeTimer !== null) {
    window.clearTimeout(refreshOnResumeTimer)
    refreshOnResumeTimer = null
  }
  if (refreshOnResumeBound) {
    window.removeEventListener('focus', refreshOnResume)
    document.removeEventListener('visibilitychange', refreshOnResume)
    window.removeEventListener('pageshow', refreshOnResume)
    refreshOnResumeBound = false
  }
  listResizeObserver?.disconnect()
  listResizeObserver = null
  if (unsubscribeMonitoringLive) {
    unsubscribeMonitoringLive()
    unsubscribeMonitoringLive = null
  }
  if (unsubscribeMonitoringSettings) {
    unsubscribeMonitoringSettings()
    unsubscribeMonitoringSettings = null
  }
})

function filterButtonClass(filter: 'all' | 'up' | 'down' | 'paused') {
  if (state.value !== filter) return 'text-muted-foreground'
  if (filter === 'up') return 'border-emerald-500/40 text-emerald-400 bg-emerald-500/10'
  if (filter === 'down') return 'border-rose-500/40 text-rose-400 bg-rose-500/10'
  if (filter === 'paused') return 'border-amber-500/40 text-amber-400 bg-amber-500/10'
  return 'border-border text-foreground bg-muted/30'
}

function sidebarFaviconSrc(monitor: Monitor): string {
  const id = String(monitor.id || '')
  const monitorType = monitor.type || monitor.monitor_type || ''
  const failedAt = faviconFailedAtByID.value[id] || 0
  const coolingDown = failedAt > 0 && Date.now() - failedAt < SIDEBAR_FAVICON_RETRY_COOLDOWN_MS
  if (
    !id ||
    !monitorSupportsFavicon(monitorType) ||
    faviconFailedByID.value[id] ||
    coolingDown ||
    (!viewportMonitorIDs.value.includes(id) && selectedMonitorID.value !== id)
  ) {
    return ''
  }
  return `/api/v1/monitoring/monitors/${id}/favicon`
}

function markFaviconFailed(monitor: Monitor) {
  const id = String(monitor.id || '')
  if (!id) return
  faviconFailedByID.value = { ...faviconFailedByID.value, [id]: true }
  faviconFailedAtByID.value = { ...faviconFailedAtByID.value, [id]: Date.now() }
  faviconRevealedByID.value = { ...faviconRevealedByID.value, [id]: false }
}

function markFaviconLoaded(monitor: Monitor) {
  const id = String(monitor.id || '')
  if (!id) return
  faviconLoadedByID.value = { ...faviconLoadedByID.value, [id]: true }
  if (faviconFailedByID.value[id] || faviconFailedAtByID.value[id]) {
    const nextFailed = { ...faviconFailedByID.value }
    const nextFailedAt = { ...faviconFailedAtByID.value }
    delete nextFailed[id]
    delete nextFailedAt[id]
    faviconFailedByID.value = nextFailed
    faviconFailedAtByID.value = nextFailedAt
  }
  if (faviconRevealedByID.value[id]) return
  window.requestAnimationFrame(() => {
    window.requestAnimationFrame(() => {
      faviconRevealedByID.value = { ...faviconRevealedByID.value, [id]: true }
    })
  })
}

function iconForMonitor(monitor: Monitor) {
  return monitorTypeIcon(monitor.type || monitor.monitor_type)
}

function faviconVisible(monitor: Monitor): boolean {
  const id = String(monitor.id || '')
  return !!id && !!faviconLoadedByID.value[id] && !!faviconRevealedByID.value[id] && !!sidebarFaviconSrc(monitor)
}

function faviconLoading(monitor: Monitor): boolean {
  const id = String(monitor.id || '')
  return !!id && !!sidebarFaviconSrc(monitor) && !faviconLoadedByID.value[id] && !faviconFailedByID.value[id]
}

function monitorStatusLabel(monitor: Monitor): string {
  const status = effectiveMonitorStatus(monitor)
  if (status === 'paused') return t('monitoring.paused')
  if (status === 'up') return t('status.up')
  if (status === 'maintenance') return t('monitoring.maintenance')
  if (status === 'pending') return t('status.pending')
  if (status === 'down') return t('status.down')
  return t('monitorDetail.checking')
}

function sidebarStatuses(monitorID: string): string[] {
  return sidebarHeartbeat(monitorID).statuses
}

function sidebarPoints(monitorID: string): Array<{ status?: string; checkedAt?: string; latencyMs?: number } | null> {
  return sidebarHeartbeat(monitorID).points
}

function sidebarHeartbeatLoaded(monitorID: string): boolean {
  const id = String(monitorID || '').trim()
  if (!id) return false
  return Array.isArray(heartbeats.value[id]) && Array.isArray(heartbeatPoints.value[id])
}

function sidebarHeartbeat(monitorID: string): {
  statuses: string[]
  points: Array<{ status?: string; checkedAt?: string; latencyMs?: number } | null>
} {
  return normalizeHeartbeatPills(heartbeats.value[monitorID], heartbeatPoints.value[monitorID], SIDEBAR_PILL_COUNT)
}

function monitorStatusToneClass(monitor: Monitor): string {
  const status = effectiveMonitorStatus(monitor)
  if (status === 'paused' || status === 'maintenance') return 'border-amber-500/40 text-amber-400 bg-amber-500/10'
  if (status === 'up') return 'border-emerald-500/40 text-emerald-400 bg-emerald-500/10'
  if (status === 'pending') return 'border-yellow-500/40 text-yellow-300 bg-yellow-500/10'
  if (status === 'down') return 'border-rose-500/40 text-rose-400 bg-rose-500/10'
  return 'border-border/70 text-muted-foreground bg-muted/30'
}

function monitorStatusDotClass(monitor: Monitor): string {
  const status = effectiveMonitorStatus(monitor)
  if (status === 'paused' || status === 'maintenance') return 'border-amber-500/50 bg-amber-400 shadow-[0_0_0_1px_rgba(251,191,36,0.22)]'
  if (status === 'up') return 'border-emerald-500/50 bg-emerald-400 shadow-[0_0_0_1px_rgba(52,211,153,0.18)]'
  if (status === 'pending') return 'border-yellow-500/50 bg-yellow-300 shadow-[0_0_0_1px_rgba(253,224,71,0.2)]'
  if (status === 'down') return 'border-rose-500/50 bg-rose-400 shadow-[0_0_0_1px_rgba(251,113,133,0.22)]'
  return 'border-border/60 bg-muted-foreground/45 shadow-[0_0_0_1px_rgba(148,163,184,0.12)]'
}

function tooltipForMonitor(monitor: Monitor) {
  return {
    name: 'NavMonitorTooltip',
    render() {
      const favicon = sidebarFaviconSrc(monitor)
      const iconNode = favicon
        ? h('div', { class: 'relative size-4 shrink-0' }, [
            h(iconForMonitor(monitor), {
              class: `absolute inset-0 size-4 text-muted-foreground transition-opacity ${faviconVisible(monitor) ? 'opacity-0' : 'opacity-100'} ${faviconLoading(monitor) ? 'animate-pulse' : ''}`,
            }),
            h('img', {
              src: favicon,
              alt: '',
              class: `absolute inset-0 size-4 rounded-sm transition-opacity ${faviconVisible(monitor) ? 'opacity-100' : 'opacity-0'}`,
              onLoad: () => markFaviconLoaded(monitor),
              onError: () => markFaviconFailed(monitor),
            }),
          ])
        : h(iconForMonitor(monitor), { class: 'size-4 shrink-0 text-muted-foreground' })

      return h('div', { class: 'flex min-w-[220px] items-start gap-3' }, [
        h('div', { class: 'relative mt-0.5 shrink-0' }, [
          iconNode,
          h('span', {
            class: `absolute -right-1 -bottom-1 size-2 rounded-full border ring-2 ring-card ${monitorStatusDotClass(monitor)}`,
          }),
        ]),
        h('div', { class: 'min-w-0 flex-1 space-y-2' }, [
          h('div', { class: 'flex items-center gap-2' }, [
            h('span', { class: 'truncate font-medium text-foreground' }, monitor.name || monitorDisplayTarget(monitor) || t('monitoring.monitorFallback')),
            h('span', {
              class: `inline-flex min-w-0 items-center rounded-full border px-1.5 py-0.5 text-[10px] font-medium ${monitorStatusToneClass(monitor)}`,
            }, monitorStatusLabel(monitor)),
          ]),
          h('div', { class: 'truncate text-xs text-muted-foreground' }, monitorDisplayTarget(monitor)),
          sidebarHeartbeatLoaded(String(monitor.id || ''))
            ? h(HeartbeatStrip, {
                size: 'sm',
                hideOpenBucket: false,
                showTooltip: false,
                statuses: sidebarStatuses(String(monitor.id || '')),
                points: sidebarPoints(String(monitor.id || '')),
              })
            : h(
                'div',
                { class: 'flex items-center gap-1 animate-pulse' },
                Array.from({ length: SIDEBAR_PILL_COUNT }, (_, index) =>
                  h('span', {
                    key: `heartbeat-placeholder-${index}`,
                    class: 'inline-block h-3 w-1 rounded-full bg-muted-foreground/35 animate-pulse',
                  }),
                ),
              ),
        ]),
      ])
    },
  }
}
</script>

<template>
  <SidebarGroup class="min-h-0 flex-1">
    <SidebarGroupLabel>{{ t('nav.monitors') }}</SidebarGroupLabel>
    <SidebarGroupContent
      v-if="!collapsed"
      class="flex min-h-0 flex-1 flex-col gap-2 overflow-hidden transition-opacity duration-200 ease-out"
      :class="sidebarSectionReady ? 'opacity-100' : 'opacity-0'"
    >
      <button
        type="button"
        class="flex h-7 w-full items-center justify-between rounded-md border border-border/60 bg-muted/20 px-2 text-[11px] font-medium text-muted-foreground hover:bg-muted/40 hover:text-foreground"
        :aria-expanded="controlsExpanded"
        @click="controlsExpanded = !controlsExpanded"
      >
        <span class="inline-flex items-center gap-2">
          <span>Monitor Tools</span>
          <span
            v-if="hasActiveControls"
            class="inline-flex h-4 min-w-4 items-center justify-center rounded-full border border-emerald-500/30 bg-emerald-500/10 px-1 text-[9px] text-emerald-300"
          >
            active
          </span>
        </span>
        <ChevronDown v-if="controlsExpanded" class="size-3.5" />
        <ChevronRight v-else class="size-3.5" />
      </button>
      <div v-if="controlsExpanded" class="space-y-2">
        <Button as-child size="sm" class="w-full justify-start">
          <RouterLink to="/monitors/new">+ {{ t('monitoring.newMonitor') }}</RouterLink>
        </Button>
        <div class="relative">
          <Input v-model="query" :placeholder="t('monitoring.searchHosts')" class="h-8 pr-8 text-xs" />
          <button
            v-if="query"
            type="button"
            class="absolute top-1/2 right-1 inline-flex size-6 -translate-y-1/2 items-center justify-center rounded-sm text-muted-foreground hover:text-foreground"
            :aria-label="t('monitoring.clearMonitorSearch')"
            @click="query = ''"
          >
            <X class="size-3.5" />
          </button>
        </div>
        <div class="grid grid-cols-4 gap-1">
          <Button size="sm" variant="outline" class="h-6 min-w-0 gap-1 px-1.5 text-[11px]" :class="filterButtonClass('all')" @click="state = 'all'">
            <Server class="size-3" />
            {{ t('common.all') }}
          </Button>
          <Button size="sm" variant="outline" class="h-6 min-w-0 gap-1 px-1.5 text-[11px]" :class="filterButtonClass('up')" @click="state = 'up'">
            <HeartPulse class="size-3" />
            {{ t('status.up') }}
          </Button>
          <Button size="sm" variant="outline" class="h-6 min-w-0 gap-1 px-1.5 text-[11px]" :class="filterButtonClass('down')" @click="state = 'down'">
            <ShieldAlert class="size-3" />
            {{ t('status.down') }}
          </Button>
          <Button size="sm" variant="outline" class="h-6 min-w-0 gap-1 px-1.5 text-[11px]" :class="filterButtonClass('paused')" @click="state = 'paused'">
            <CirclePause class="size-3" />
            {{ t('monitoring.paused') }}
          </Button>
        </div>
      </div>

      <div ref="listViewportRef" class="min-h-0 flex-1 overflow-y-auto pr-1" @scroll="onListScroll">
        <SidebarMenu v-if="monitorsLoaded && filtered.length" class="gap-0.5">
          <li v-if="listTopSpacerHeight > 0" aria-hidden="true" class="pointer-events-none" :style="{ height: `${listTopSpacerHeight}px` }" />
          <SidebarMenuItem
            v-for="{ monitor, absoluteIndex } in virtualMonitors"
            :key="monitor.id || monitor.name"
            :style="{ height: `${SIDEBAR_ROW_HEIGHT}px` }"
          >
            <SidebarMenuButton as-child :is-active="selectedMonitorID === (monitor.id || '')" class="h-8 px-2" :data-index="absoluteIndex">
              <RouterLink :to="`/monitors/${monitor.id || ''}`" class="flex w-full items-center gap-1.5">
                <div class="min-w-0 flex flex-1 items-center justify-between gap-1.5">
                  <div class="flex min-w-0 items-center gap-1.5">
                    <div v-if="sidebarFaviconSrc(monitor)" class="relative size-3.5 shrink-0">
                      <div
                        class="absolute inset-0 rounded-sm bg-muted/60"
                        :class="faviconVisible(monitor) ? 'opacity-0' : 'opacity-100'"
                      />
                      <img
                        :src="sidebarFaviconSrc(monitor)"
                        alt=""
                        class="absolute inset-0 size-3.5 rounded-sm"
                        :class="faviconVisible(monitor) ? 'opacity-100' : 'opacity-0'"
                        @load="markFaviconLoaded(monitor)"
                        @error="markFaviconFailed(monitor)"
                      />
                    </div>
                    <component :is="iconForMonitor(monitor)" v-else class="size-3.5 shrink-0 text-muted-foreground" />
                    <Badge
                      variant="outline"
                      class="h-4 min-w-7 justify-center rounded-full px-1 text-[9px]"
                      :class="monitorStatusToneClass(monitor)"
                    >
                      <Pause v-if="monitor.enabled === false" class="size-2.5" />
                      <template v-else>
                        {{ monitorStatusLabel(monitor) }}
                      </template>
                    </Badge>
                    <span class="truncate text-[13px]">{{ monitor.name || monitorDisplayTarget(monitor) || t('monitoring.monitorFallback') }}</span>
                  </div>
                  <div class="relative h-3 shrink-0" :style="{ width: `${SIDEBAR_PILL_STRIP_WIDTH_REM}rem` }">
                    <div
                      v-if="!sidebarHeartbeatLoaded(monitor.id || '')"
                      class="absolute inset-0 flex items-center gap-1"
                    >
                      <span
                        v-for="pillIndex in SIDEBAR_PILL_COUNT"
                        :key="`sidebar-heartbeat-placeholder-${monitor.id || absoluteIndex}-${pillIndex}`"
                        class="inline-block h-3 w-1 rounded-full bg-muted-foreground/35"
                      />
                    </div>
                    <HeartbeatStrip
                      v-else
                      class="absolute inset-0 shrink-0"
                      size="sm"
                      :hide-open-bucket="false"
                      :show-tooltip="false"
                      :statuses="sidebarStatuses(monitor.id || '')"
                      :points="sidebarPoints(monitor.id || '')"
                    />
                  </div>
                </div>
              </RouterLink>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <li
            v-if="listBottomSpacerHeight > 0"
            aria-hidden="true"
            class="pointer-events-none"
            :style="{ height: `${listBottomSpacerHeight}px` }"
          />
        </SidebarMenu>
        <SidebarMenu v-else-if="monitorsLoaded" class="gap-0.5">
          <SidebarMenuItem>
            <SidebarMenuButton disabled>
              <span>{{ t('monitoring.noMonitors') }}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </div>
    </SidebarGroupContent>

    <SidebarGroupContent v-else class="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div ref="listViewportRef" class="min-h-0 flex-1 overflow-y-auto" @scroll="onListScroll">
        <SidebarMenu class="gap-0.5">
          <SidebarMenuItem>
            <SidebarMenuButton as-child :tooltip="t('monitoring.newMonitor')">
              <RouterLink to="/monitors/new" class="relative flex items-center justify-center">
                <Plus class="size-4" />
              </RouterLink>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <li v-if="listTopSpacerHeight > 0" aria-hidden="true" class="pointer-events-none" :style="{ height: `${listTopSpacerHeight}px` }" />
          <SidebarMenuItem
            v-for="{ monitor } in virtualMonitors"
            :key="`collapsed-${monitor.id || monitor.name}`"
            :style="{ height: `${SIDEBAR_ROW_HEIGHT}px` }"
          >
            <SidebarMenuButton as-child :is-active="selectedMonitorID === (monitor.id || '')" :tooltip="tooltipForMonitor(monitor)" class="h-8 px-2">
              <RouterLink :to="`/monitors/${monitor.id || ''}`" class="flex w-full items-center justify-center">
                <div class="relative flex size-4 items-center justify-center">
                  <img
                    v-if="sidebarFaviconSrc(monitor)"
                    :src="sidebarFaviconSrc(monitor)"
                    alt=""
                    class="absolute inset-0 m-auto size-3.5 rounded-sm"
                    :class="faviconVisible(monitor) ? 'opacity-100' : 'opacity-0'"
                    @load="markFaviconLoaded(monitor)"
                    @error="markFaviconFailed(monitor)"
                  />
                  <div
                    v-if="sidebarFaviconSrc(monitor)"
                    class="absolute inset-0 m-auto size-3.5 rounded-sm bg-muted/60"
                    :class="faviconVisible(monitor) ? 'opacity-0' : 'opacity-100'"
                  />
                  <component
                    v-else
                    :is="iconForMonitor(monitor)"
                    class="size-3.5 shrink-0 text-muted-foreground"
                  />
                  <div class="absolute -right-0.5 -bottom-0.5 size-2 rounded-full border ring-2 ring-sidebar" :class="monitorStatusDotClass(monitor)" />
                </div>
              </RouterLink>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <li
            v-if="listBottomSpacerHeight > 0"
            aria-hidden="true"
            class="pointer-events-none"
            :style="{ height: `${listBottomSpacerHeight}px` }"
          />
        </SidebarMenu>
      </div>
    </SidebarGroupContent>
  </SidebarGroup>
</template>
