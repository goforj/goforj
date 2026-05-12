<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import MonitorDetailPanel from '@/components/MonitorDetailPanel.vue'
import { subscribeMonitoringSettingsUpdated } from '@/lib/monitoring-settings-events'
import { fetchHeartbeatsForMonitorIDs, fetchMonitorDashboard, fetchSidebarMonitors } from '@/lib/monitoring-requests'
import { applyMonitorStatusSnapshot, subscribeMonitoringStatusEvents, type MonitorStatusEvent } from '@/lib/monitoring-live'
import { apiFetch } from '@/lib/auth'
import { toast } from 'vue-sonner'

type Monitor = {
  id?: string
  name?: string
  type?: string
  last_status?: string
  target_display?: string
  interval_seconds?: number
  enabled?: boolean
  uptime_24h?: number
  maintenance_active?: boolean
  maintenance_starts_at?: string
  maintenance_ends_at?: string
}

const loading = ref(true)
const { t } = useI18n()
const monitors = ref<Monitor[]>([])
const heartbeats = ref<Record<string, string[]>>({})
const heartbeatPoints = ref<Record<string, Array<{ status?: string; checked_at?: string; latency_ms?: number }>>>({})
const selectedMonitorID = ref<string>('')
const selectedMonitor = ref<any | null>(null)
const selectedChecks = ref<any[]>([])
const selectedIncidents = ref<any[]>([])
const selectedStats = ref<any | null>(null)
const globalMaintenanceActive = ref(false)
const checkNowInFlightID = ref<string>('')
const pollingVisibleUntilByMonitor = ref<Record<string, number>>({})
const lastManualCheckAtByMonitor = ref<Record<string, number>>({})
const cooldownNowMs = ref(Date.now())
const checkNowMinLoadingMs = 350
const pollingMinVisibleMs = 900
const selectedCheckRange = ref<'15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'>('1h')
const selectedZoomFromTs = ref<number | null>(null)
const selectedZoomToTs = ref<number | null>(null)
const creatingMonitor = ref(false)
let selectedMonitorRequestSeq = 0
let suppressNextRangeQueryWatch = false
const route = useRoute()
const router = useRouter()
const validCheckRanges = new Set(['15m', '1h', '3h', '6h', '12h', '24h', '7d', '30d'])
const selectedMonitorContentReady = computed(
  () =>
    !!(
      selectedMonitor.value &&
      typeof selectedMonitor.value === 'object' &&
      String(selectedMonitor.value.id || '') === selectedMonitorID.value
    ),
)
const selectedMonitorShell = computed(() => {
  if (selectedMonitorContentReady.value) {
    return selectedMonitor.value
  }
  const fallback = monitors.value.find((monitor) => String(monitor.id || '') === selectedMonitorID.value)
  if (fallback) return fallback
  if (!selectedMonitorID.value) return null
  return {
    id: selectedMonitorID.value,
    name: t('routes.monitorDetail'),
  }
})

function checkRangeFromQuery(): '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d' {
  const raw = route.query.range
  const value = Array.isArray(raw) ? raw[0] : raw
  if (typeof value === 'string' && validCheckRanges.has(value)) {
    return value as '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'
  }
  // Keep first paint fast and deterministic when query has no range.
  return '1h'
}

function monitorIDFromRoute(): string {
  const raw = route.params.id
  if (typeof raw === 'string') return raw
  return ''
}

function parseQueryTimestamp(value: unknown): number | null {
  const raw = Array.isArray(value) ? value[0] : value
  if (typeof raw !== 'string' || !raw.trim()) return null
  const parsed = Number(raw)
  if (!Number.isFinite(parsed) || parsed <= 0) return null
  return parsed
}

function syncZoomFromQuery() {
  const from = parseQueryTimestamp(route.query.from)
  const to = parseQueryTimestamp(route.query.to)
  if (from !== null && to !== null && to > from) {
    selectedZoomFromTs.value = from
    selectedZoomToTs.value = to
    return
  }
  selectedZoomFromTs.value = null
  selectedZoomToTs.value = null
}

async function loadMonitors() {
  const monitorPayload = await fetchSidebarMonitors()
  monitors.value = Array.isArray(monitorPayload.monitors) ? (monitorPayload.monitors as Monitor[]) : []
  applyMonitorStatusSnapshot(monitors.value)
}

async function loadHeartbeats() {
  if (!selectedMonitorID.value) {
    heartbeats.value = {}
    heartbeatPoints.value = {}
    return
  }
  try {
    const heartbeatPayload = await fetchHeartbeatsForMonitorIDs([selectedMonitorID.value], 30)
    heartbeats.value =
      heartbeatPayload.heartbeats && typeof heartbeatPayload.heartbeats === 'object'
        ? (heartbeatPayload.heartbeats as Record<string, string[]>)
        : {}
    heartbeatPoints.value =
      heartbeatPayload.heartbeat_points && typeof heartbeatPayload.heartbeat_points === 'object'
        ? (heartbeatPayload.heartbeat_points as Record<string, Array<{ status?: string; checked_at?: string; latency_ms?: number }>>)
        : {}
  } catch {
    heartbeats.value = {}
    heartbeatPoints.value = {}
  }
}

async function load() {
  loading.value = true
  try {
    selectedCheckRange.value = checkRangeFromQuery()
    syncZoomFromQuery()
    const routed = monitorIDFromRoute()
    if (routed) {
      // For routed detail pages, render detail first; sidebar list has its own loader.
      selectedMonitorID.value = routed
      await loadSelectedMonitorByID(routed)
      window.setTimeout(() => {
        void loadHeartbeats()
      }, 1)
      return
    }

    // Non-routed view can still use list-first selection behavior.
    void loadHeartbeats()
    await loadMonitors()
    if (!selectedMonitorID.value && monitors.value.length > 0 && monitors.value[0].id) {
      selectedMonitorID.value = monitors.value[0].id
      await router.replace({ path: `/monitors/${selectedMonitorID.value}`, query: route.query })
    }
    await loadSelectedMonitor()
  } finally {
    loading.value = false
  }
}

function applyMonitorStatusEvent(event: MonitorStatusEvent) {
  if (!event.monitor_id) return
  if (event.type === 'monitor.polling') {
    if (event.phase === 'started') {
      pollingVisibleUntilByMonitor.value = {
        ...pollingVisibleUntilByMonitor.value,
        [event.monitor_id]: Date.now() + pollingMinVisibleMs,
      }
      checkNowInFlightID.value = event.monitor_id
    } else if (event.phase === 'completed' && checkNowInFlightID.value === event.monitor_id) {
      checkNowInFlightID.value = ''
    }
    return
  }
  monitors.value = monitors.value.map((monitor) =>
    String(monitor.id || '') === event.monitor_id
      ? { ...monitor, last_status: event.status || monitor.last_status }
      : monitor,
  )
  if (checkNowInFlightID.value === event.monitor_id) {
    checkNowInFlightID.value = ''
  }
  if (selectedMonitorID.value === event.monitor_id) {
    if (selectedMonitor.value && typeof selectedMonitor.value === 'object') {
      selectedMonitor.value = {
        ...selectedMonitor.value,
        last_status: event.status || selectedMonitor.value.last_status,
        status: event.status || selectedMonitor.value.status,
      }
    }
    void loadHeartbeats()
    void refreshSelectedMonitorDetail()
  }
}

function monitorWindowActive(startsAt?: string, endsAt?: string): boolean {
  if (!startsAt || !endsAt) return false
  const startMs = Date.parse(startsAt)
  const endMs = Date.parse(endsAt)
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs)) return false
  const now = Date.now()
  return startMs <= now && now < endMs
}

function applyMaintenanceSnapshot(maintenance?: { active?: boolean }) {
  globalMaintenanceActive.value = Boolean(maintenance?.active)
  monitors.value = monitors.value.map((monitor) => ({
    ...monitor,
    maintenance_active: globalMaintenanceActive.value || monitorWindowActive(monitor.maintenance_starts_at, monitor.maintenance_ends_at),
  }))
  if (selectedMonitor.value && typeof selectedMonitor.value === 'object') {
    selectedMonitor.value = {
      ...selectedMonitor.value,
      maintenance_active:
        globalMaintenanceActive.value ||
        monitorWindowActive(selectedMonitor.value.maintenance_starts_at, selectedMonitor.value.maintenance_ends_at),
    }
  }
}

function onZoomWindowChange(value: { from: number; to: number } | null) {
  if (!value) {
    selectedZoomFromTs.value = null
    selectedZoomToTs.value = null
    const query = { ...route.query } as Record<string, any>
    delete query.from
    delete query.to
    void router.replace({ query })
    return
  }
  selectedZoomFromTs.value = value.from
  selectedZoomToTs.value = value.to
  void router.replace({
    query: {
      ...route.query,
      from: String(Math.round(value.from)),
      to: String(Math.round(value.to)),
    },
  })
}

async function loadSelectedMonitor() {
  if (!selectedMonitorID.value) {
    selectedMonitor.value = null
    selectedChecks.value = []
    selectedIncidents.value = []
    selectedStats.value = null
    return
  }
  await loadSelectedMonitorByID(selectedMonitorID.value)
}

async function loadSelectedMonitorByID(monitorID: string) {
  const requestSeq = ++selectedMonitorRequestSeq
  if (creatingMonitor.value || !monitorID) {
    selectedMonitor.value = null
    selectedChecks.value = []
    selectedIncidents.value = []
    selectedStats.value = null
    return
  }
  const requestedRange = selectedCheckRange.value
  const payload = await fetchMonitorDashboard(monitorID, requestedRange)
  if (requestSeq !== selectedMonitorRequestSeq) return

  selectedMonitor.value = payload.monitor ?? null
  selectedChecks.value = Array.isArray(payload.checks) ? payload.checks : []
  selectedStats.value = payload.stats ?? null
  selectedIncidents.value = Array.isArray(payload.incidents) ? payload.incidents : []
}

async function checkNow(id: string) {
  if (!id || checkNowInFlightID.value === id) return
  const startedAt = Date.now()
  checkNowInFlightID.value = id
  try {
    const resp = await apiFetch(`/api/v1/monitoring/monitors/${id}/check-now?sync=1`, { method: 'POST' })
    let payload: any = null
    try {
      payload = await resp.json()
    } catch {
      payload = null
    }
    if (!resp.ok) {
      toast.error(payload?.error || t('monitoring.pollFailed'))
      return
    }
    lastManualCheckAtByMonitor.value = {
      ...lastManualCheckAtByMonitor.value,
      [id]: startedAt,
    }
    if ((payload?.failed ?? 0) > 0) {
      toast.error(t('monitoring.pollCompletedWithFailures', { count: payload.failed }))
    }
    await loadSelectedMonitor()
    void loadHeartbeats()
  } finally {
    const elapsedMs = Date.now() - startedAt
    if (elapsedMs < checkNowMinLoadingMs) {
      await new Promise((resolve) => window.setTimeout(resolve, checkNowMinLoadingMs - elapsedMs))
    }
    checkNowInFlightID.value = ''
  }
}

async function removeMonitor(id: string) {
  if (!confirm(t('monitoring.confirmDeleteMonitor'))) return
  const resp = await apiFetch(`/api/v1/monitoring/monitors/${id}`, { method: 'DELETE' })
  if (!resp.ok) return
  if (selectedMonitorID.value === id) {
    selectedMonitorID.value = ''
    selectedMonitor.value = null
    selectedChecks.value = []
    selectedIncidents.value = []
    selectedStats.value = null
    void router.replace({ path: '/monitors', query: route.query })
  }
  await load()
}

async function toggleMonitorEnabled(id: string, enabled: boolean) {
  const resp = await apiFetch(`/api/v1/monitoring/monitors/${id}/enabled`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  })
  if (!resp.ok) return
  if (enabled) {
    await checkNow(id)
    return
  }
  await loadSelectedMonitor()
  void loadHeartbeats()
}

function onCheckRangeChange(next: '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d') {
  if (selectedCheckRange.value === next) return
  selectedCheckRange.value = next
  suppressNextRangeQueryWatch = true
  void router.replace({
    query: {
      ...route.query,
      range: next,
    },
  })
  void loadSelectedMonitor()
}

onMounted(load)

let detailRefreshTimer: number | null = null
let resumeRefreshTimer: number | null = null
let cooldownClockTimer: number | null = null
let resumeRefreshBound = false
let detailRefreshInFlight = false
let unsubscribeMonitoringLive: (() => void) | null = null
let unsubscribeMonitoringSettings: (() => void) | null = null

function selectedMonitorCheckNowDisabled(): boolean {
  if (!selectedMonitorID.value || !selectedMonitor.value) return false
  if (Boolean(selectedMonitor.value.maintenance_active)) return true
  const lastStartedAt = lastManualCheckAtByMonitor.value[selectedMonitorID.value]
  if (!lastStartedAt) return false
  const intervalSeconds = Math.max(0, Number(selectedMonitor.value.interval_seconds || 0))
  if (intervalSeconds <= 0) return false
  return cooldownNowMs.value < lastStartedAt + intervalSeconds * 1000
}

function selectedMonitorCheckNowCooldownRemainingMs(): number {
  if (!selectedMonitorID.value || !selectedMonitor.value) return 0
  const lastStartedAt = lastManualCheckAtByMonitor.value[selectedMonitorID.value]
  if (!lastStartedAt) return 0
  const intervalSeconds = Math.max(0, Number(selectedMonitor.value.interval_seconds || 0))
  if (intervalSeconds <= 0) return 0
  return Math.max(0, lastStartedAt + intervalSeconds * 1000 - cooldownNowMs.value)
}

function selectedMonitorCheckNowLoading(): boolean {
  if (!selectedMonitorID.value) return false
  if (checkNowInFlightID.value === selectedMonitorID.value) return true
  const visibleUntil = pollingVisibleUntilByMonitor.value[selectedMonitorID.value] || 0
  return cooldownNowMs.value < visibleUntil
}

async function refreshSelectedMonitorDetail() {
  if (!selectedMonitorID.value) return
  if (detailRefreshInFlight) return
  detailRefreshInFlight = true
  try {
    await loadSelectedMonitor()
  } finally {
    detailRefreshInFlight = false
  }
}

async function refreshMonitoringDataOnResume() {
  const tasks: Promise<unknown>[] = [loadMonitors()]
  if (selectedMonitorID.value) {
    tasks.push(loadHeartbeats())
  }
  if (selectedMonitorID.value) {
    tasks.push(refreshSelectedMonitorDetail())
  }
  await Promise.allSettled(tasks)
}

const refreshOnResume = () => {
  if (document.visibilityState === 'hidden') return
  if (resumeRefreshTimer !== null) {
    window.clearTimeout(resumeRefreshTimer)
  }
  resumeRefreshTimer = window.setTimeout(() => {
    resumeRefreshTimer = null
    void refreshMonitoringDataOnResume()
  }, 100)
}

onMounted(() => {
  detailRefreshTimer = window.setInterval(() => {
    void refreshSelectedMonitorDetail()
  }, 5000)
  cooldownClockTimer = window.setInterval(() => {
    cooldownNowMs.value = Date.now()
  }, 200)
  if (!unsubscribeMonitoringLive) {
    unsubscribeMonitoringLive = subscribeMonitoringStatusEvents(applyMonitorStatusEvent)
  }
  if (!unsubscribeMonitoringSettings) {
    unsubscribeMonitoringSettings = subscribeMonitoringSettingsUpdated((maintenance) => {
      applyMaintenanceSnapshot(maintenance)
      void load()
    })
  }
  if (!resumeRefreshBound) {
    window.addEventListener('focus', refreshOnResume)
    document.addEventListener('visibilitychange', refreshOnResume)
    window.addEventListener('pageshow', refreshOnResume)
    resumeRefreshBound = true
  }
})
onUnmounted(() => {
  if (detailRefreshTimer !== null) {
    window.clearInterval(detailRefreshTimer)
    detailRefreshTimer = null
  }
  if (cooldownClockTimer !== null) {
    window.clearInterval(cooldownClockTimer)
    cooldownClockTimer = null
  }
  if (resumeRefreshBound) {
    window.removeEventListener('focus', refreshOnResume)
    document.removeEventListener('visibilitychange', refreshOnResume)
    window.removeEventListener('pageshow', refreshOnResume)
    resumeRefreshBound = false
  }
  if (resumeRefreshTimer !== null) {
    window.clearTimeout(resumeRefreshTimer)
    resumeRefreshTimer = null
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

watch(
  () => route.params.id,
  (next) => {
    if (typeof next !== 'string' || !next || next === selectedMonitorID.value) return
    selectedMonitorID.value = next
    creatingMonitor.value = false
    void loadSelectedMonitor()
  },
)

watch(
  () => route.query.range,
  (next) => {
    if (suppressNextRangeQueryWatch) {
      suppressNextRangeQueryWatch = false
      return
    }
    const value = Array.isArray(next) ? next[0] : next
    if (typeof value !== 'string' || !validCheckRanges.has(value)) return
    if (selectedCheckRange.value === value) return
    selectedCheckRange.value = value as '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'
    void loadSelectedMonitor()
  },
)

watch(
  () => [route.query.from, route.query.to] as const,
  () => {
    syncZoomFromQuery()
  },
)

</script>

<template>
  <div class="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
    <div class="px-4 lg:px-6" v-if="selectedMonitorShell">
      <MonitorDetailPanel
        :monitor="selectedMonitorShell"
        :loading="!selectedMonitorContentReady"
        :check-now-loading="selectedMonitorCheckNowLoading()"
        :check-now-disabled="selectedMonitorCheckNowDisabled()"
        :check-now-cooldown-remaining-ms="selectedMonitorCheckNowCooldownRemainingMs()"
        :heartbeat-statuses="heartbeats[selectedMonitorID] || []"
        :heartbeat-points="heartbeatPoints[selectedMonitorID] || []"
        :checks="selectedChecks"
        :check-range="selectedCheckRange"
        :zoom-from-ts="selectedZoomFromTs"
        :zoom-to-ts="selectedZoomToTs"
        :incidents="selectedIncidents"
        :stats="selectedStats"
        @update:check-range="onCheckRangeChange"
        @update:zoom-window="onZoomWindowChange"
        @toggle-enabled="toggleMonitorEnabled"
        @check-now="checkNow"
      />
    </div>
    <div class="px-4 lg:px-6" v-else-if="!loading">
      <div class="rounded-md border border-border p-4 text-sm text-muted-foreground">
        {{ t('monitoring.selectMonitorPrompt') }}
      </div>
    </div>
  </div>
</template>
