<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
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
const autoRangeManaged = ref(false)
const creatingMonitor = ref(false)
let selectedMonitorRequestSeq = 0
const route = useRoute()
const router = useRouter()
const validCheckRanges = new Set(['15m', '1h', '3h', '6h', '12h', '24h', '7d', '30d'])
const rangeWindowMs: Record<'15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d', number> = {
  '15m': 15 * 60 * 1000,
  '1h': 60 * 60 * 1000,
  '3h': 3 * 60 * 60 * 1000,
  '6h': 6 * 60 * 60 * 1000,
  '12h': 12 * 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
  '30d': 30 * 24 * 60 * 60 * 1000,
}

function hasRangeQuery(): boolean {
  const raw = route.query.range
  const value = Array.isArray(raw) ? raw[0] : raw
  return typeof value === 'string' && validCheckRanges.has(value)
}

function rangeFromQueryOrEmpty(): string {
  const raw = route.query.range
  const value = Array.isArray(raw) ? raw[0] : raw
  return typeof value === 'string' ? value : ''
}

function checkRangeFromQuery(): '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d' {
  const raw = route.query.range
  const value = Array.isArray(raw) ? raw[0] : raw
  if (typeof value === 'string' && validCheckRanges.has(value)) {
    return value as '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'
  }
  // Probe with broad history; we auto-select a tighter range after first payload.
  return '30d'
}

function inferBestRangeFromChecks(
  checks: Array<{ checked_at?: string }>,
): '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d' {
  if (!Array.isArray(checks) || checks.length === 0) return '15m'
  const samplesMs: number[] = []
  for (const check of checks) {
    if (!check?.checked_at) continue
    const parsed = Date.parse(check.checked_at)
    if (!Number.isNaN(parsed)) samplesMs.push(parsed)
  }
  if (samplesMs.length === 0) return '15m'

  samplesMs.sort((a, b) => b - a)
  const newestMs = samplesMs[0]
  const oldestMs = samplesMs[samplesMs.length - 1]
  const spanMs = Math.max(0, newestMs - oldestMs)

  const orderedRanges: Array<'15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'> = ['15m', '1h', '3h', '6h', '12h', '24h', '7d', '30d']
  let best = '15m' as '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'
  let bestDistance = Number.POSITIVE_INFINITY
  for (const range of orderedRanges) {
    const distance = Math.abs(spanMs - rangeWindowMs[range])
    if (distance < bestDistance) {
      best = range
      bestDistance = distance
    }
  }
  return best
}

function monitorIDFromRoute(): string {
  const raw = route.params.id
  if (typeof raw === 'string') return raw
  return ''
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
    autoRangeManaged.value = !hasRangeQuery()
    selectedCheckRange.value = checkRangeFromQuery()
    const routed = monitorIDFromRoute()
    if (routed) {
      selectedMonitorID.value = routed
    }

    // Start heartbeat fetch immediately, but never block first paint on it.
    void loadHeartbeats()

    // For deep links, fetch detail immediately in parallel with monitor list.
    const detailPromise = routed ? loadSelectedMonitorByID(routed) : Promise.resolve()
    await loadMonitors()

    if (routed && monitors.value.some((m) => m.id === routed)) {
      selectedMonitorID.value = routed
    } else if (!selectedMonitorID.value && monitors.value.length > 0 && monitors.value[0].id) {
      selectedMonitorID.value = monitors.value[0].id
      await router.replace({ path: `/monitors/${selectedMonitorID.value}`, query: route.query })
    } else if (routed && !monitors.value.some((m) => m.id === routed)) {
      selectedMonitorID.value = ''
      await router.replace({ path: '/monitors', query: route.query })
    }

    // Wait for routed detail only if it is still valid after list resolve.
    if (routed && monitors.value.some((m) => m.id === routed)) {
      await detailPromise
    } else {
      await loadSelectedMonitor()
    }
  } finally {
    loading.value = false
  }
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
  const shell = monitors.value.find((m) => m.id === monitorID)
  if (!selectedMonitor.value || selectedMonitor.value.id !== monitorID) {
    selectedMonitor.value = {
      id: monitorID,
      name: shell?.name || 'Loading monitor…',
      target: shell?.target || '',
      target_url: shell?.target_url || '',
      target_host: shell?.target_host || '',
      target_port: shell?.target_port || 0,
      target_record_type: shell?.target_record_type || '',
      target_keyword: shell?.target_keyword || '',
      target_expected: shell?.target_expected || '',
      target_container: shell?.target_container || '',
      target_docker_host: shell?.target_docker_host || '',
      target_push_token: shell?.target_push_token || '',
      interval_seconds: shell?.interval_seconds || 60,
      enabled: shell?.enabled ?? true,
    }
    selectedChecks.value = []
    selectedIncidents.value = []
    selectedStats.value = null
  }
  const shouldAutoInferRange = autoRangeManaged.value || !hasRangeQuery()
  const probeRange: '30d' = '30d'
  const requestedRange = shouldAutoInferRange ? probeRange : selectedCheckRange.value
  let payload = await fetchMonitorDashboard(monitorID, requestedRange)
  if (requestSeq !== selectedMonitorRequestSeq) return

  if (shouldAutoInferRange) {
    const probeChecks = Array.isArray(payload.checks) ? payload.checks : []
    const inferred = inferBestRangeFromChecks(probeChecks)
    if (selectedCheckRange.value !== inferred) {
      selectedCheckRange.value = inferred
    }
    if (rangeFromQueryOrEmpty() !== inferred) {
      await router.replace({
        query: {
          ...route.query,
          range: inferred,
        },
      })
      if (requestSeq !== selectedMonitorRequestSeq) return
    }
    if (inferred !== requestedRange) {
      payload = await fetchMonitorDashboard(monitorID, inferred)
      if (requestSeq !== selectedMonitorRequestSeq) return
    }
  }

  selectedMonitor.value = payload.monitor ?? null
  selectedChecks.value = Array.isArray(payload.checks) ? payload.checks : []
  selectedStats.value = payload.stats ?? null
  selectedIncidents.value = Array.isArray(payload.incidents) ? payload.incidents : []
}

async function checkNow(id: string) {
  if (!id || checkNowInFlightID.value === id) return
  const startedAt = Date.now()
  checkNowInFlightID.value = id
  const toastID = toast.loading('Polling monitor now...')
  try {
    const resp = await fetch(`/api/v1/monitoring/monitors/${id}/check-now?sync=1`, { method: 'POST' })
    let payload: any = null
    try {
      payload = await resp.json()
    } catch {
      payload = null
    }
    if (!resp.ok) {
      toast.error(payload?.error || 'Monitor poll failed', { id: toastID })
      return
    }
    if ((payload?.failed ?? 0) > 0) {
      toast.error(`Poll complete with failures (${payload.failed})`, { id: toastID })
    } else {
      toast.success('Poll complete', { id: toastID })
    }
    await loadSelectedMonitor()
    await loadMonitors()
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
  if (!confirm('Delete this monitor?')) return
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
  await loadMonitors()
  void loadHeartbeats()
}

function onCheckRangeChange(next: '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d') {
  if (selectedCheckRange.value === next) return
  autoRangeManaged.value = false
  selectedCheckRange.value = next
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

const refreshOnResume = () => {
  if (document.visibilityState === 'hidden') return
  if (!selectedMonitorID.value) return
  if (resumeRefreshTimer !== null) {
    window.clearTimeout(resumeRefreshTimer)
  }
  resumeRefreshTimer = window.setTimeout(() => {
    resumeRefreshTimer = null
    void refreshSelectedMonitorDetail()
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
    const value = Array.isArray(next) ? next[0] : next
    if (typeof value !== 'string' || !validCheckRanges.has(value)) return
    if (selectedCheckRange.value === value) return
    selectedCheckRange.value = value as '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'
    selectedChecks.value = []
    selectedStats.value = null
    void loadSelectedMonitor()
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
        :incidents="selectedIncidents"
        :stats="selectedStats"
        @update:check-range="onCheckRangeChange"
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
        Select a monitor from the list or sidebar to inspect details.
      </div>
    </div>
  </div>
</template>
