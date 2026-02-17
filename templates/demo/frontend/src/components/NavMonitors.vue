<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { CirclePause, HeartPulse, Pause, Server, ShieldAlert, X } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import HeartbeatStrip from '@/components/HeartbeatStrip.vue'
import { fetchHeartbeats, fetchMonitors } from '@/lib/monitoring-requests'
import { monitorSupportsFavicon, monitorTypeIcon } from '@/lib/monitor-icons'
import { displayTargetFromFields } from '@/lib/monitor-target'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
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
}

const route = useRoute()
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

const selectedMonitorID = computed(() => String(route.params.id || ''))

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  return monitors.value.filter((m) => {
    if (q) {
      const haystack = `${m.name || ''} ${monitorDisplayTarget(m) || ''}`.toLowerCase()
      if (!haystack.includes(q)) return false
    }
    const s = (m.last_status || '').toLowerCase()
    if (state.value === 'up' && (m.enabled === false || s !== 'up')) return false
    if (state.value === 'down' && (m.enabled === false || s === 'up')) return false
    if (state.value === 'paused' && m.enabled !== false) return false
    return true
  })
})

async function loadMonitors() {
  try {
    const monitorPayload = await fetchMonitors()
    monitors.value = Array.isArray(monitorPayload.monitors) ? (monitorPayload.monitors as Monitor[]) : []
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

onMounted(() => {
  const onDetailRoute = typeof route.params.id === 'string' && route.params.id.length > 0
  const delayMs = onDetailRoute ? 1 : 0
  window.setTimeout(() => {
    void load({ deferHeartbeats: true })
  }, delayMs)
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
  if (monitor.enabled === false) return t('monitoring.paused')
  const status = (monitor.last_status || 'unknown').toLowerCase()
  if (status === 'up') return t('status.up')
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
  let source = (heartbeats.value[monitorID] || []).slice(-SIDEBAR_PILL_COUNT)
  const tail = source[source.length - 1]
  if ((tail || '').toLowerCase() === 'unknown') {
    source = source.slice(0, -1)
  }
  if (source.length >= SIDEBAR_PILL_COUNT) return source
  return [...Array(SIDEBAR_PILL_COUNT - source.length).fill('unknown'), ...source]
}

function sidebarPoints(monitorID: string): Array<{ status?: string; checkedAt?: string; latencyMs?: number } | null> {
  let source = (heartbeatPoints.value[monitorID] || [])
    .slice(-SIDEBAR_PILL_COUNT)
    .map((point) => ({
      status: point?.status,
      checkedAt: point?.checked_at,
      latencyMs: point?.latency_ms,
    }))
  const tail = source[source.length - 1]
  if (!tail?.checkedAt) {
    source = source.slice(0, -1)
  }
  if (source.length >= SIDEBAR_PILL_COUNT) return source
  return [...Array(SIDEBAR_PILL_COUNT - source.length).fill(null), ...source]
}
</script>

<template>
  <SidebarGroup class="group-data-[collapsible=icon]:hidden">
    <SidebarGroupLabel>{{ t('nav.monitors') }}</SidebarGroupLabel>
    <SidebarGroupContent class="space-y-2">
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
                        : (monitor.last_status || '').toLowerCase() === 'up'
                        ? 'border-emerald-500/40 text-emerald-400'
                        : (monitor.last_status || '').toLowerCase() === 'pending'
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
  </SidebarGroup>
</template>
