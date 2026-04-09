<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { CirclePause, HeartPulse, Pause, Plus, Server, ShieldAlert, X } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import HeartbeatStrip from '@/components/HeartbeatStrip.vue'
import { subscribeMonitoringSettingsUpdated } from '@/lib/monitoring-settings-events'
import { fetchHeartbeats, fetchMonitors } from '@/lib/monitoring-requests'
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

const route = useRoute()
const { state: sidebarState } = useSidebar()
const { t } = useI18n()
const monitors = ref<Monitor[]>([])
const heartbeats = ref<Record<string, string[]>>({})
const heartbeatPoints = ref<Record<string, Array<{ status?: string; checked_at?: string; latency_ms?: number }>>>({})
const monitorsLoaded = ref(false)
const heartbeatReady = ref(false)
const SIDEBAR_PILL_COUNT = 12
const faviconFailedByID = ref<Record<string, boolean>>({})
const query = ref('')
const state = ref<'all' | 'up' | 'down' | 'paused'>('all')
const globalMaintenanceActive = ref(false)
let unsubscribeMonitoringLive: (() => void) | null = null
let unsubscribeMonitoringSettings: (() => void) | null = null
const collapsed = computed(() => sidebarState.value === 'collapsed')

function monitorMaintenanceActive(monitor: Monitor): boolean {
  return globalMaintenanceActive.value || monitorWindowActive(monitor.maintenance_starts_at, monitor.maintenance_ends_at)
}

function monitorWindowActive(startsAt?: string, endsAt?: string): boolean {
  if (!startsAt || !endsAt) return false
  const startMs = Date.parse(startsAt)
  const endMs = Date.parse(endsAt)
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs)) return false
  const now = Date.now()
  return startMs <= now && now < endMs
}

function effectiveMonitorStatus(monitor: Monitor): string {
  if (monitor.enabled === false) return 'paused'
  if (monitorMaintenanceActive(monitor)) return 'maintenance'
  return (monitor.last_status || 'unknown').toLowerCase()
}

const selectedMonitorID = computed(() => String(route.params.id || ''))

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

async function loadMonitors() {
  try {
    const monitorPayload = await fetchMonitors()
    monitors.value = Array.isArray(monitorPayload.monitors) ? (monitorPayload.monitors as Monitor[]) : []
    applyMonitorStatusSnapshot(monitors.value)
  } finally {
    monitorsLoaded.value = true
  }
}

async function loadHeartbeats() {
  try {
    const heartbeatPayload = await fetchHeartbeats(30)
    heartbeats.value =
      heartbeatPayload.heartbeats && typeof heartbeatPayload.heartbeats === 'object'
        ? (heartbeatPayload.heartbeats as Record<string, string[]>)
        : {}
    heartbeatPoints.value =
      heartbeatPayload.heartbeat_points && typeof heartbeatPayload.heartbeat_points === 'object'
        ? (heartbeatPayload.heartbeat_points as Record<string, Array<{ status?: string; checked_at?: string; latency_ms?: number }>>)
        : {}
  } finally {
    heartbeatReady.value = true
  }
}

async function load(options: { deferHeartbeats?: boolean } = {}) {
  const deferHeartbeats = options.deferHeartbeats === true

  // Run monitors and heartbeat fetches independently to avoid coupling paint timing.
  void loadMonitors()
  if (deferHeartbeats) {
    window.setTimeout(() => {
      void loadHeartbeats()
    }, 0)
    return
  }
  void loadHeartbeats()
}

function applyMonitorStatusEvent(event: MonitorStatusEvent) {
  if (!event.monitor_id) return
  monitors.value = monitors.value.map((monitor) =>
    String(monitor.id || '') === event.monitor_id
      ? { ...monitor, last_status: event.status || monitor.last_status }
      : monitor,
  )
  void loadHeartbeats()
}

onMounted(() => {
  const onDetailRoute = typeof route.params.id === 'string' && route.params.id.length > 0
  const delayMs = onDetailRoute ? 1 : 0
  window.setTimeout(() => {
    void load({ deferHeartbeats: true })
  }, delayMs)
  if (!unsubscribeMonitoringLive) {
    unsubscribeMonitoringLive = subscribeMonitoringStatusEvents(applyMonitorStatusEvent)
  }
  if (!unsubscribeMonitoringSettings) {
    unsubscribeMonitoringSettings = subscribeMonitoringSettingsUpdated((maintenance) => {
      globalMaintenanceActive.value = Boolean(maintenance?.active)
      void load()
    })
  }
})

let refreshTimer: number | null = null
onMounted(() => {
  refreshTimer = window.setInterval(() => {
    void load()
  }, 15000)
})
onUnmounted(() => {
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
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
  if (!id || !monitorSupportsFavicon(monitorType) || faviconFailedByID.value[id]) {
    return ''
  }
  return `/api/v1/monitoring/monitors/${id}/favicon`
}

function markFaviconFailed(monitor: Monitor) {
  const id = String(monitor.id || '')
  if (!id) return
  faviconFailedByID.value = { ...faviconFailedByID.value, [id]: true }
}

function iconForMonitor(monitor: Monitor) {
  return monitorTypeIcon(monitor.type || monitor.monitor_type)
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
  const statuses = (heartbeats.value[monitorID] || []).map((s) => (s || 'unknown').toLowerCase())
  const points = (heartbeatPoints.value[monitorID] || []).map((point) => ({
    status: (point?.status || 'unknown').toLowerCase(),
    checkedAt: point?.checked_at,
    latencyMs: point?.latency_ms,
  }))

  const count = Math.max(statuses.length, points.length)
  let merged = Array.from({ length: count }, (_, idx) => ({
    status: (statuses[idx] || points[idx]?.status || 'unknown').toLowerCase(),
    point: points[idx] || null,
  }))

  const tail = merged[merged.length - 1]
  if (tail && tail.status === 'unknown' && !tail.point?.checkedAt) {
    merged = merged.slice(0, -1)
  }

  const trimmed = merged.slice(-SIDEBAR_PILL_COUNT)
  const padded = [
    ...Array(Math.max(0, SIDEBAR_PILL_COUNT - trimmed.length)).fill({ status: 'unknown', point: null }),
    ...trimmed,
  ]

  return {
    statuses: padded.map((row) => row.status),
    points: padded.map((row) => row.point),
  }
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
        ? h('img', {
            src: favicon,
            alt: '',
            class: 'size-4 shrink-0 rounded-sm',
          })
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
  <SidebarGroup>
    <SidebarGroupLabel>{{ t('nav.monitors') }}</SidebarGroupLabel>
    <SidebarGroupContent v-if="!collapsed" class="space-y-2">
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
        <Button
          size="sm"
          variant="outline"
          class="h-6 min-w-0 gap-1 px-1.5 text-[11px]"
          :class="filterButtonClass('all')"
          @click="state = 'all'"
        >
          <Server class="size-3" />
          {{ t('common.all') }}
        </Button>
        <Button
          size="sm"
          variant="outline"
          class="h-6 min-w-0 gap-1 px-1.5 text-[11px]"
          :class="filterButtonClass('up')"
          @click="state = 'up'"
        >
          <HeartPulse class="size-3" />
          {{ t('status.up') }}
        </Button>
        <Button
          size="sm"
          variant="outline"
          class="h-6 min-w-0 gap-1 px-1.5 text-[11px]"
          :class="filterButtonClass('down')"
          @click="state = 'down'"
        >
          <ShieldAlert class="size-3" />
          {{ t('status.down') }}
        </Button>
        <Button
          size="sm"
          variant="outline"
          class="h-6 min-w-0 gap-1 px-1.5 text-[11px]"
          :class="filterButtonClass('paused')"
          @click="state = 'paused'"
        >
          <CirclePause class="size-3" />
          {{ t('monitoring.paused') }}
        </Button>
      </div>
      <SidebarMenu>
        <SidebarMenuItem v-if="monitorsLoaded && !filtered.length">
          <SidebarMenuButton disabled>
            <span>{{ t('monitoring.noMonitors') }}</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
        <SidebarMenuItem v-for="monitor in filtered" :key="monitor.id || monitor.name">
          <SidebarMenuButton as-child :is-active="selectedMonitorID === (monitor.id || '')">
            <RouterLink :to="`/monitors/${monitor.id || ''}`" class="flex w-full items-center gap-2">
              <div class="min-w-0 flex flex-1 items-center justify-between gap-2">
                <div class="flex min-w-0 items-center gap-2">
                  <img
                    v-if="sidebarFaviconSrc(monitor)"
                    :src="sidebarFaviconSrc(monitor)"
                    alt=""
                    class="size-4 shrink-0 rounded-sm"
                    @error="markFaviconFailed(monitor)"
                  />
                  <component
                    :is="iconForMonitor(monitor)"
                    v-else
                    class="size-4 shrink-0 text-muted-foreground"
                  />
                  <Badge
                    variant="outline"
                    class="h-4 min-w-8 justify-center rounded-full px-1 text-[10px]"
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
                    <Pause v-if="monitor.enabled === false" class="size-3" />
                    <template v-else>
                      {{ monitorStatusLabel(monitor) }}
                    </template>
                  </Badge>
                  <span class="truncate">{{ monitor.name || monitorDisplayTarget(monitor) || t('monitoring.monitorFallback') }}</span>
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
      </SidebarMenu>
    </SidebarGroupContent>
    <SidebarGroupContent v-else>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton as-child :tooltip="t('monitoring.newMonitor')">
            <RouterLink to="/monitors/new" class="relative flex items-center justify-center">
              <Plus class="size-4" />
            </RouterLink>
          </SidebarMenuButton>
        </SidebarMenuItem>
        <SidebarMenuItem v-for="monitor in filtered" :key="`collapsed-${monitor.id || monitor.name}`">
          <SidebarMenuButton
            as-child
            :is-active="selectedMonitorID === (monitor.id || '')"
            :tooltip="tooltipForMonitor(monitor)"
          >
            <RouterLink :to="`/monitors/${monitor.id || ''}`" class="relative flex items-center justify-center">
              <img
                v-if="sidebarFaviconSrc(monitor)"
                :src="sidebarFaviconSrc(monitor)"
                alt=""
                class="size-4 shrink-0 rounded-sm"
                @error="markFaviconFailed(monitor)"
              />
              <component
                :is="iconForMonitor(monitor)"
                v-else
                class="size-4 shrink-0 text-muted-foreground"
              />
              <div
                class="absolute right-1.5 bottom-1.5 size-2 rounded-full border ring-2 ring-sidebar"
                :class="monitorStatusDotClass(monitor)"
              />
            </RouterLink>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarGroupContent>
  </SidebarGroup>
</template>
