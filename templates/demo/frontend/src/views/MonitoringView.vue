<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import MonitorDetailPanel from '@/components/MonitorDetailPanel.vue'
import { fetchHeartbeats, fetchMonitorDashboard, fetchMonitors } from '@/lib/monitoring-requests'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from 'vue-sonner'

type Monitor = {
  id?: string
  name?: string
  type?: string
  target?: string
  target_url?: string
  target_host?: string
  target_port?: number
  target_record_type?: string
  target_keyword?: string
  target_expected?: string
  target_container?: string
  target_docker_host?: string
  target_push_token?: string
  interval_seconds?: number
  enabled?: boolean
  uptime_24h?: number
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
const checkNowInFlightID = ref<string>('')
const checkNowMinLoadingMs = 350
const selectedCheckRange = ref<'15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'>('1h')
const selectedZoomFromTs = ref<number | null>(null)
const selectedZoomToTs = ref<number | null>(null)
const creatingMonitor = ref(false)
let selectedMonitorRequestSeq = 0
let suppressNextRangeQueryWatch = false
const route = useRoute()
const router = useRouter()
const validCheckRanges = new Set(['15m', '1h', '3h', '6h', '12h', '24h', '7d', '30d'])

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
  const monitorPayload = await fetchMonitors()
  monitors.value = Array.isArray(monitorPayload.monitors) ? (monitorPayload.monitors as Monitor[]) : []
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
  const toastID = toast.loading(t('monitoring.pollingNow'))
  try {
    const resp = await fetch(`/api/v1/monitoring/monitors/${id}/check-now?sync=1`, { method: 'POST' })
    let payload: any = null
    try {
      payload = await resp.json()
    } catch {
      payload = null
    }
    if (!resp.ok) {
      toast.error(payload?.error || t('monitoring.pollFailed'), { id: toastID })
      return
    }
    if ((payload?.failed ?? 0) > 0) {
      toast.error(t('monitoring.pollCompletedWithFailures', { count: payload.failed }), { id: toastID })
    } else {
      toast.success(t('monitoring.pollComplete'), { id: toastID })
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
  const resp = await fetch(`/api/v1/monitoring/monitors/${id}`, { method: 'DELETE' })
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
  const resp = await fetch(`/api/v1/monitoring/monitors/${id}/enabled`, {
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
  selectedChecks.value = []
  selectedStats.value = null
  void loadSelectedMonitor()
}

onMounted(load)

let detailRefreshTimer: number | null = null
let resumeRefreshTimer: number | null = null
let resumeRefreshBound = false
let detailRefreshInFlight = false

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
  const tasks: Promise<unknown>[] = [loadMonitors(), loadHeartbeats()]
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
    selectedChecks.value = []
    selectedStats.value = null
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
    <div class="px-4 lg:px-6" v-if="selectedMonitor">
      <MonitorDetailPanel
        :monitor="selectedMonitor"
        :check-now-loading="checkNowInFlightID === selectedMonitorID"
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
    <div class="px-4 lg:px-6" v-else-if="loading">
      <div class="space-y-3 rounded-md border border-border p-4">
        <Skeleton class="h-8 w-60" />
        <Skeleton class="h-5 w-40" />
        <Skeleton class="h-16 w-full" />
        <div class="grid grid-cols-1 gap-2 md:grid-cols-5">
          <Skeleton class="h-12 w-full" />
          <Skeleton class="h-12 w-full" />
          <Skeleton class="h-12 w-full" />
          <Skeleton class="h-12 w-full" />
          <Skeleton class="h-12 w-full" />
        </div>
        <Skeleton class="h-80 w-full" />
      </div>
    </div>
    <div class="px-4 lg:px-6" v-else>
      <div class="rounded-md border border-border p-4 text-sm text-muted-foreground">
        {{ t('monitoring.selectMonitorPrompt') }}
      </div>
    </div>
  </div>
</template>
