<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

type MonitorOption = {
  id?: string
  name?: string
}

type CadenceRow = {
  monitor_id: string
  name: string
  interval_seconds: number
  tolerance_sec: number
  samples: number
  delta_samples: number
  delta_min_sec: number
  delta_p50_sec: number
  delta_p95_sec: number
  delta_max_sec: number
  on_time_count: number
  missed_count: number
  duplicate_count: number
  last_checked_at?: string
  freshness_sec: number
}

type AlertEventRow = {
  id: number
  created_at?: string
  checked_at?: string
  event?: string
  monitor_id?: string
  monitor_name?: string
  monitor_type?: string
  target?: string
  status?: string
  incident_id?: string
  summary?: string
  channel_id?: number
  channel_name?: string
  provider?: string
  delivery?: 'delivered' | 'failed' | 'skipped' | string
  error_message?: string
}

const ranges = ['1h', '24h', '7d', '30d'] as const
const selectedRange = ref<(typeof ranges)[number]>('24h')
const selectedMonitor = ref<string>('all')
const loading = ref(false)
const rows = ref<CadenceRow[]>([])
const alertEvents = ref<AlertEventRow[]>([])
const monitors = ref<MonitorOption[]>([])
const { t } = useI18n()

const onTimeRate = (row: CadenceRow) => {
  if (row.delta_samples <= 0) return 0
  return (row.on_time_count / row.delta_samples) * 100
}

const freshnessLabel = (seconds: number) => {
  if (seconds <= 0) return t('common.now')
  if (seconds < 60) return t('relative.secondsAgo', { count: seconds })
  if (seconds < 3600) return t('relative.minutesAgo', { count: Math.floor(seconds / 60) })
  return t('relative.hoursAgo', { count: Math.floor(seconds / 3600) })
}

const sortedRows = computed(() => {
  return [...rows.value].sort((a, b) => {
    if (a.missed_count !== b.missed_count) return b.missed_count - a.missed_count
    if (a.duplicate_count !== b.duplicate_count) return b.duplicate_count - a.duplicate_count
    return a.name.localeCompare(b.name)
  })
})

function onRangeChange(value: string) {
  if (!ranges.includes(value as (typeof ranges)[number])) return
  selectedRange.value = value as (typeof ranges)[number]
  void loadDiagnostics()
}

function onMonitorChange(value: string) {
  selectedMonitor.value = value || 'all'
  void loadDiagnostics()
}

async function loadMonitors() {
  const res = await fetch('/api/v1/monitoring/monitors')
  if (!res.ok) return
  const payload = await res.json()
  const values = Array.isArray(payload.monitors) ? payload.monitors : []
  monitors.value = values.map((it: any) => ({
    id: it?.id,
    name: it?.name || it?.id || t('monitoring.monitorFallback'),
  }))
}

async function loadDiagnostics() {
  loading.value = true
  try {
    const params = new URLSearchParams({ range: selectedRange.value })
    if (selectedMonitor.value !== 'all') {
      params.set('monitor_id', selectedMonitor.value)
    }
    const [cadenceRes, alertRes] = await Promise.all([
      fetch(`/api/v1/monitoring/diagnostics/cadence?${params.toString()}`),
      fetch(`/api/v1/monitoring/diagnostics/alerts?${params.toString()}`),
    ])
    if (cadenceRes.ok) {
      const cadencePayload = await cadenceRes.json()
      rows.value = Array.isArray(cadencePayload.monitors) ? cadencePayload.monitors : []
    } else {
      rows.value = []
    }
    if (alertRes.ok) {
      const alertPayload = await alertRes.json()
      alertEvents.value = Array.isArray(alertPayload.events) ? alertPayload.events : []
    } else {
      alertEvents.value = []
    }
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await loadMonitors()
  await loadDiagnostics()
})

let refreshTimer: number | null = null
onMounted(() => {
  refreshTimer = window.setInterval(() => {
    void loadDiagnostics()
  }, 15000)
})
onUnmounted(() => {
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
})
</script>

<template>
  <div class="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
    <div class="px-4 lg:px-6">
      <Card>
        <CardHeader class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <CardTitle>{{ t('diagnostics.cadenceTitle') }}</CardTitle>
            <CardDescription>{{ t('diagnostics.cadenceDescription') }}</CardDescription>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <Select :model-value="selectedRange" @update:model-value="onRangeChange">
              <SelectTrigger class="w-28">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="range in ranges" :key="range" :value="range">
                  {{ t('common.lastRange', { range }) }}
                </SelectItem>
              </SelectContent>
            </Select>
            <Select :model-value="selectedMonitor" @update:model-value="onMonitorChange">
              <SelectTrigger class="w-52">
                <SelectValue :placeholder="t('diagnostics.allMonitors')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{{ t('diagnostics.allMonitors') }}</SelectItem>
                <SelectItem v-for="monitor in monitors" :key="monitor.id" :value="monitor.id || ''">
                  {{ monitor.name }}
                </SelectItem>
              </SelectContent>
            </Select>
            <Button type="button" variant="outline" size="sm" @click="loadDiagnostics">
              <RefreshCw class="mr-1 size-3.5" />
              {{ t('common.refresh') }}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{{ t('monitoring.monitorFallback') }}</TableHead>
                <TableHead>{{ t('monitoring.interval') }}</TableHead>
                <TableHead>{{ t('diagnostics.p50p95') }}</TableHead>
                <TableHead>{{ t('diagnostics.missed') }}</TableHead>
                <TableHead>{{ t('diagnostics.duplicate') }}</TableHead>
                <TableHead>{{ t('diagnostics.onTime') }}</TableHead>
                <TableHead>{{ t('diagnostics.freshness') }}</TableHead>
                <TableHead>{{ t('diagnostics.lastCheck') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-if="loading">
                <TableCell colspan="8" class="h-16 text-center text-muted-foreground">
                  {{ t('diagnostics.loadingCadence') }}
                </TableCell>
              </TableRow>
              <TableRow v-else-if="!sortedRows.length">
                <TableCell colspan="8" class="h-16 text-center text-muted-foreground">
                  {{ t('diagnostics.noCadenceData') }}
                </TableCell>
              </TableRow>
              <TableRow v-for="row in sortedRows" :key="row.monitor_id">
                <TableCell class="font-medium">{{ row.name }}</TableCell>
                <TableCell class="text-muted-foreground">
                  {{ row.interval_seconds }}s ± {{ row.tolerance_sec }}s
                </TableCell>
                <TableCell class="text-muted-foreground">
                  {{ row.delta_p50_sec }}s / {{ row.delta_p95_sec }}s
                </TableCell>
                <TableCell>
                  <Badge variant="outline" :class="row.missed_count > 0 ? 'border-rose-500/40 text-rose-400' : 'text-muted-foreground'">
                    {{ row.missed_count }}
                  </Badge>
                </TableCell>
                <TableCell>
                  <Badge variant="outline" :class="row.duplicate_count > 0 ? 'border-amber-500/40 text-amber-400' : 'text-muted-foreground'">
                    {{ row.duplicate_count }}
                  </Badge>
                </TableCell>
                <TableCell>
                  <Badge variant="outline" :class="onTimeRate(row) >= 90 ? 'border-emerald-500/40 text-emerald-400' : 'border-amber-500/40 text-amber-400'">
                    {{ onTimeRate(row).toFixed(0) }}%
                  </Badge>
                </TableCell>
                <TableCell class="text-muted-foreground">
                  {{ freshnessLabel(row.freshness_sec) }}
                </TableCell>
                <TableCell class="text-muted-foreground">
                  {{ row.last_checked_at ? new Date(row.last_checked_at).toLocaleString() : '-' }}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
    <div class="px-4 lg:px-6">
      <Card>
        <CardHeader>
          <CardTitle>{{ t('diagnostics.alertEventsTitle') }}</CardTitle>
          <CardDescription>{{ t('diagnostics.alertEventsDescription') }}</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{{ t('monitorDetail.time') }}</TableHead>
                <TableHead>{{ t('diagnostics.event') }}</TableHead>
                <TableHead>{{ t('monitoring.monitorFallback') }}</TableHead>
                <TableHead>{{ t('diagnostics.channel') }}</TableHead>
                <TableHead>{{ t('diagnostics.result') }}</TableHead>
                <TableHead>{{ t('diagnostics.error') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-if="loading">
                <TableCell colspan="6" class="h-16 text-center text-muted-foreground">
                  {{ t('diagnostics.loadingAlertEvents') }}
                </TableCell>
              </TableRow>
              <TableRow v-else-if="!alertEvents.length">
                <TableCell colspan="6" class="h-16 text-center text-muted-foreground">
                  {{ t('diagnostics.noAlertEvents') }}
                </TableCell>
              </TableRow>
              <TableRow v-for="row in alertEvents" :key="row.id">
                <TableCell class="text-muted-foreground">
                  {{ row.created_at ? new Date(row.created_at).toLocaleString() : '-' }}
                </TableCell>
                <TableCell class="font-medium">
                  <div class="flex flex-col">
                    <span>{{ row.event || '-' }}</span>
                    <span class="text-xs text-muted-foreground">{{ row.status || '-' }}</span>
                  </div>
                </TableCell>
                <TableCell class="text-muted-foreground">
                  <div class="flex flex-col">
                    <span>{{ row.monitor_name || row.monitor_id || '-' }}</span>
                    <span class="text-xs">{{ row.monitor_type || '-' }}</span>
                  </div>
                </TableCell>
                <TableCell class="text-muted-foreground">
                  <div class="flex flex-col">
                    <span>{{ row.channel_name || t('diagnostics.broadcastNone') }}</span>
                    <span class="text-xs">{{ row.provider || '-' }}</span>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge
                    variant="outline"
                    :class="
                      (row.delivery || '').toLowerCase() === 'delivered'
                        ? 'border-emerald-500/40 text-emerald-400'
                        : (row.delivery || '').toLowerCase() === 'failed'
                        ? 'border-rose-500/40 text-rose-400'
                        : 'border-amber-500/40 text-amber-400'
                    "
                  >
                    {{ row.delivery || t('status.unknown') }}
                  </Badge>
                </TableCell>
                <TableCell class="max-w-md truncate text-xs text-muted-foreground">
                  {{ row.error_message || '-' }}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
