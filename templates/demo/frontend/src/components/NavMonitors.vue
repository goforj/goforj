<script setup lang="ts">
import { computed, h, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { CirclePause, HeartPulse, Pause, Plus, Server, ShieldAlert, X } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import HeartbeatStrip from '@/components/HeartbeatStrip.vue'
import { normalizeHeartbeatPills } from '@/lib/heartbeat-pills'
import { subscribeMonitoringSettingsUpdated } from '@/lib/monitoring-settings-events'
import { fetchHeartbeatsForMonitorIDs, fetchSidebarMonitors } from '@/lib/monitoring-requests'
import { applyMonitorStatusSnapshot, subscribeMonitoringStatusEvents, type MonitorStatusEvent } from '@/lib/monitoring-live'
import { monitorSupportsFavicon, monitorTypeIcon } from '@/lib/monitor-icons'
import { displayTargetFromFields } from '@/lib/monitor-target'
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
  target?: string
  type?: string
  monitor_type?: string
  target_url?: string
  target_host?: string
  target_port?: number
  target_record_type?: string
  target_keyword?: string
  target_expected?: string
  target_container?: string
  target_docker_host?: string
  target_push_token?: string
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
const faviconLoadedByID = ref<Record<string, boolean>>({})
const query = ref('')
const state = ref<'all' | 'up' | 'down' | 'paused'>('all')
const globalMaintenanceActive = ref(false)
const listViewportRef = ref<HTMLElement | null>(null)
const listScrollTop = ref(0)
const listViewportHeight = ref(0)

const SIDEBAR_PILL_COUNT = 12
const SIDEBAR_ROW_HEIGHT = 32
const SIDEBAR_OVERSCAN = 6

let listResizeObserver: ResizeObserver | null = null
let visibleHeartbeatTimer: number | null = null
let visibleHeartbeatDebounceTimer: number | null = null
let refreshOnResumeTimer: number | null = null
let refreshOnResumeBound = false
let unsubscribeMonitoringLive: (() => void) | null = null
let unsubscribeMonitoringSettings: (() => void) | null = null

const collapsed = computed(() => sidebarState.value === 'collapsed')
const selectedMonitorID = computed(() => String(route.params.id || ''))

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
  return displayTargetFromFields(monitor.type || monitor.monitor_type || '', {
    target: monitor.target,
    target_url: monitor.target_url,
    target_host: monitor.target_host,
    target_port: monitor.target_port,
    target_record_type: monitor.target_record_type,
    target_keyword: monitor.target_keyword,
    target_expected: monitor.target_expected,
    target_container: monitor.target_container,
    target_docker_host: monitor.target_docker_host,
    target_push_token: monitor.target_push_token,
  })
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

const visibleMonitorIDs = computed(() =>
  virtualMonitors.value
    .map(({ monitor }) => String(monitor.id || '').trim())
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
  scheduleVisibleHeartbeatRefresh()
}

async function loadMonitors() {
  try {
    const monitorPayload = await fetchSidebarMonitors()
    monitors.value = Array.isArray(monitorPayload.monitors) ? (monitorPayload.monitors as Monitor[]) : []
    applyMonitorStatusSnapshot(monitors.value)
  } finally {
    monitorsLoaded.value = true
  }
}

async function loadVisibleHeartbeats(ids?: string[]) {
  const requested = Array.from(new Set((ids ?? visibleMonitorIDs.value).map((id) => String(id || '').trim()).filter(Boolean)))
  if (!requested.length) return
  if (document.visibilityState === 'hidden') return
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
  }
}

function scheduleVisibleHeartbeatRefresh(ids?: string[]) {
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
  if (visibleMonitorIDs.value.includes(event.monitor_id)) {
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
    void loadMonitors()
    scheduleVisibleHeartbeatRefresh()
  }, 100)
}

watch([query, state], () => {
  const el = listViewportRef.value
  if (el) {
    el.scrollTop = 0
  }
  updateListViewportMetrics()
  scheduleVisibleHeartbeatRefresh()
})

watch(
  () => visibleMonitorIDs.value.join(','),
  () => {
    scheduleVisibleHeartbeatRefresh()
  },
)

watch(collapsed, async () => {
  await nextTick()
  bindListViewport()
  scheduleVisibleHeartbeatRefresh()
})

onMounted(async () => {
  await loadMonitors()
  await nextTick()
  bindListViewport()
  scheduleVisibleHeartbeatRefresh()

  visibleHeartbeatTimer = window.setInterval(() => {
    scheduleVisibleHeartbeatRefresh()
  }, 15000)

  if (!unsubscribeMonitoringLive) {
    unsubscribeMonitoringLive = subscribeMonitoringStatusEvents(applyMonitorStatusEvent)
  }
  if (!unsubscribeMonitoringSettings) {
    unsubscribeMonitoringSettings = subscribeMonitoringSettingsUpdated((maintenance) => {
      globalMaintenanceActive.value = Boolean(maintenance?.active)
      void loadMonitors()
      scheduleVisibleHeartbeatRefresh()
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
  if (
    !id ||
    !monitorSupportsFavicon(monitorType) ||
    faviconFailedByID.value[id] ||
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
}

function markFaviconLoaded(monitor: Monitor) {
  const id = String(monitor.id || '')
  if (!id) return
  faviconLoadedByID.value = { ...faviconLoadedByID.value, [id]: true }
}

function iconForMonitor(monitor: Monitor) {
  return monitorTypeIcon(monitor.type || monitor.monitor_type)
}

function faviconVisible(monitor: Monitor): boolean {
  const id = String(monitor.id || '')
  return !!id && !!faviconLoadedByID.value[id] && !!sidebarFaviconSrc(monitor)
}

function monitorStatusLabel(monitor: Monitor): string {
  const status = effectiveMonitorStatus(monitor)
  if (status === 'paused') return t('monitoring.paused')
  if (status === 'up') return t('status.up')
  if (status === 'maintenance') return t('monitoring.maintenance')
  if (status === 'pending') return t('status.pending')
  if (status === 'down') return t('status.down')
  return t('status.unknown')
}

function sidebarStatuses(monitorID: string): string[] {
  return sidebarHeartbeat(monitorID).statuses
}

function sidebarPoints(monitorID: string): Array<{ status?: string; checkedAt?: string; latencyMs?: number } | null> {
  return sidebarHeartbeat(monitorID).points
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
  return 'border-rose-500/40 text-rose-400 bg-rose-500/10'
}

function monitorStatusDotClass(monitor: Monitor): string {
  const status = effectiveMonitorStatus(monitor)
  if (status === 'paused' || status === 'maintenance') return 'border-amber-500/50 bg-amber-400 shadow-[0_0_0_1px_rgba(251,191,36,0.22)]'
  if (status === 'up') return 'border-emerald-500/50 bg-emerald-400 shadow-[0_0_0_1px_rgba(52,211,153,0.18)]'
  if (status === 'pending') return 'border-yellow-500/50 bg-yellow-300 shadow-[0_0_0_1px_rgba(253,224,71,0.2)]'
  return 'border-rose-500/50 bg-rose-400 shadow-[0_0_0_1px_rgba(251,113,133,0.22)]'
}

function tooltipForMonitor(monitor: Monitor) {
  return {
    name: 'NavMonitorTooltip',
    render() {
      const favicon = sidebarFaviconSrc(monitor)
      const iconNode = favicon
        ? h('div', { class: 'relative size-4 shrink-0' }, [
            h(iconForMonitor(monitor), {
              class: `absolute inset-0 size-4 text-muted-foreground transition-opacity ${faviconVisible(monitor) ? 'opacity-0' : 'opacity-100'}`,
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
          heartbeatReady.value
            ? h(HeartbeatStrip, {
                size: 'sm',
                hideOpenBucket: false,
                showTooltip: false,
                statuses: sidebarStatuses(String(monitor.id || '')),
                points: sidebarPoints(String(monitor.id || '')),
              })
            : h('div', { class: 'h-3 w-16 rounded-full bg-muted-foreground/25' }),
        ]),
      ])
    },
  }
}
</script>

<template>
  <SidebarGroup class="min-h-0 flex-1">
    <SidebarGroupLabel>{{ t('nav.monitors') }}</SidebarGroupLabel>
    <SidebarGroupContent v-if="!collapsed" class="flex min-h-0 flex-1 flex-col gap-2 overflow-hidden">
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
                      <component
                        :is="iconForMonitor(monitor)"
                        class="absolute inset-0 size-3.5 text-muted-foreground transition-opacity"
                        :class="faviconVisible(monitor) ? 'opacity-0' : 'opacity-100'"
                      />
                      <img
                        :src="sidebarFaviconSrc(monitor)"
                        alt=""
                        class="absolute inset-0 size-3.5 rounded-sm transition-opacity"
                        :class="faviconVisible(monitor) ? 'opacity-100' : 'opacity-0'"
                        @load="markFaviconLoaded(monitor)"
                        @error="markFaviconFailed(monitor)"
                      />
                    </div>
                    <component :is="iconForMonitor(monitor)" v-else class="size-3.5 shrink-0 text-muted-foreground" />
                    <Badge
                      variant="outline"
                      class="h-4 min-w-7 justify-center rounded-full px-1 text-[9px]"
                      :class="
                        monitor.enabled === false
                          ? 'border-amber-500/40 text-amber-400'
                          : effectiveMonitorStatus(monitor) === 'maintenance'
                          ? 'border-amber-500/40 text-amber-400'
                          : effectiveMonitorStatus(monitor) === 'up'
                          ? 'border-emerald-500/40 text-emerald-400'
                          : effectiveMonitorStatus(monitor) === 'pending'
                          ? 'border-yellow-500/40 text-yellow-300'
                          : 'border-rose-500/40 text-rose-400'
                      "
                    >
                      <Pause v-if="monitor.enabled === false" class="size-2.5" />
                      <template v-else>
                        {{ monitorStatusLabel(monitor) }}
                      </template>
                    </Badge>
                    <span class="truncate text-[13px]">{{ monitor.name || monitorDisplayTarget(monitor) || t('monitoring.monitorFallback') }}</span>
                  </div>
                  <HeartbeatStrip
                    v-if="heartbeatReady"
                    class="shrink-0"
                    size="sm"
                    :hide-open-bucket="false"
                    :show-tooltip="false"
                    :statuses="sidebarStatuses(monitor.id || '')"
                    :points="sidebarPoints(monitor.id || '')"
                  />
                  <div v-else class="shrink-0">
                    <Skeleton class="h-3 w-16 rounded-full bg-muted-foreground/25" />
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
              <RouterLink :to="`/monitors/${monitor.id || ''}`" class="relative flex items-center justify-center">
                <img
                  v-if="sidebarFaviconSrc(monitor)"
                  :src="sidebarFaviconSrc(monitor)"
                  alt=""
                  class="absolute inset-0 size-3.5 rounded-sm transition-opacity"
                  :class="faviconVisible(monitor) ? 'opacity-100' : 'opacity-0'"
                  @load="markFaviconLoaded(monitor)"
                  @error="markFaviconFailed(monitor)"
                />
                <component
                  :is="iconForMonitor(monitor)"
                  class="size-3.5 shrink-0 text-muted-foreground transition-opacity"
                  :class="sidebarFaviconSrc(monitor) && faviconVisible(monitor) ? 'opacity-0' : 'opacity-100'"
                />
                <div class="absolute right-1.5 bottom-1.5 size-2 rounded-full border ring-2 ring-sidebar" :class="monitorStatusDotClass(monitor)" />
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
