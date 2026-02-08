<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import MonitorDetailPanel from '@/components/MonitorDetailPanel.vue'
import { fetchMonitors } from '@/lib/monitoring-requests'

type Monitor = {
  id?: string
  name?: string
  type?: string
  target?: string
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
const selectedCheckRange = ref<'1h' | '24h' | '7d' | '30d'>('1h')
const creatingMonitor = ref(false)
let selectedMonitorRequestSeq = 0
const route = useRoute()
const router = useRouter()
const validCheckRanges = new Set(['1h', '24h', '7d', '30d'])

function checkRangeFromQuery(): '1h' | '24h' | '7d' | '30d' {
  const raw = route.query.range
  const value = Array.isArray(raw) ? raw[0] : raw
  if (typeof value === 'string' && validCheckRanges.has(value)) {
    return value as '1h' | '24h' | '7d' | '30d'
  }
  return '1h'
}

function monitorIDFromRoute(): string {
  const raw = route.params.id
  if (typeof raw === 'string') return raw
  return ''
}

async function load() {
  loading.value = true
  try {
    selectedCheckRange.value = checkRangeFromQuery()
    const monitorPayload = await fetchMonitors()
    monitors.value = Array.isArray(monitorPayload.monitors) ? (monitorPayload.monitors as Monitor[]) : []
    const routed = monitorIDFromRoute()
    if (routed && monitors.value.some((m) => m.id === routed)) {
      selectedMonitorID.value = routed
    } else if (!selectedMonitorID.value && monitors.value.length > 0 && monitors.value[0].id) {
      selectedMonitorID.value = monitors.value[0].id
      await router.replace({ path: `/monitors/${selectedMonitorID.value}`, query: route.query })
    } else if (routed && !monitors.value.some((m) => m.id === routed)) {
      await router.replace({ path: '/monitors', query: route.query })
    }
    await loadSelectedMonitor()
  } finally {
    loading.value = false
  }
}

async function loadSelectedMonitor() {
  const requestSeq = ++selectedMonitorRequestSeq
  if (creatingMonitor.value || !selectedMonitorID.value) {
    selectedMonitor.value = null
    selectedChecks.value = []
    selectedIncidents.value = []
    selectedStats.value = null
    return
  }
  const [detailRes, checksRes, incidentsRes, heartbeatRes] = await Promise.all([
    fetch(`/api/v1/monitoring/monitors/${selectedMonitorID.value}`),
    fetch(`/api/v1/monitoring/monitors/${selectedMonitorID.value}/checks?range=${selectedCheckRange.value}&_ts=${Date.now()}`, {
      cache: 'no-store',
    }),
    fetch(`/api/v1/monitoring/monitors/${selectedMonitorID.value}/incidents`),
    fetch(`/api/v1/monitoring/heartbeats?limit=30&_ts=${Date.now()}`, { cache: 'no-store' }),
  ])
  if (requestSeq !== selectedMonitorRequestSeq) return
  if (detailRes.ok) {
    const payload = await detailRes.json()
    if (requestSeq !== selectedMonitorRequestSeq) return
    selectedMonitor.value = payload.monitor ?? null
  }
  if (checksRes.ok) {
    const payload = await checksRes.json()
    if (requestSeq !== selectedMonitorRequestSeq) return
    selectedChecks.value = Array.isArray(payload.checks) ? payload.checks : []
    selectedStats.value = payload.stats ?? null
  }
  if (incidentsRes.ok) {
    const payload = await incidentsRes.json()
    if (requestSeq !== selectedMonitorRequestSeq) return
    selectedIncidents.value = Array.isArray(payload.incidents) ? payload.incidents : []
  }
  if (heartbeatRes.ok) {
    const payload = await heartbeatRes.json()
    if (requestSeq !== selectedMonitorRequestSeq) return
    heartbeats.value = payload.heartbeats && typeof payload.heartbeats === 'object' ? payload.heartbeats : {}
    heartbeatPoints.value =
      payload.heartbeat_points && typeof payload.heartbeat_points === 'object' ? payload.heartbeat_points : {}
  }
}

async function checkNow(id: string) {
  await fetch(`/api/v1/monitoring/monitors/${id}/check-now`, { method: 'POST' })
  await load()
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
  await load()
}

function onCheckRangeChange(next: '1h' | '24h' | '7d' | '30d') {
  if (selectedCheckRange.value === next) return
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
onMounted(() => {
  detailRefreshTimer = window.setInterval(() => {
    void loadSelectedMonitor()
  }, 5000)
})
onUnmounted(() => {
  if (detailRefreshTimer !== null) {
    window.clearInterval(detailRefreshTimer)
    detailRefreshTimer = null
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
    selectedCheckRange.value = value as '1h' | '24h' | '7d' | '30d'
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
    <div class="px-4 lg:px-6">
      <div v-if="!selectedMonitor" class="rounded-md border border-border p-4 text-sm text-muted-foreground">
        Select a monitor from the list or sidebar to inspect details.
      </div>
    </div>
  </div>
</template>
