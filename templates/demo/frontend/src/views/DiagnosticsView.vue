<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
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

const ranges = ['1h', '24h', '7d', '30d'] as const
const selectedRange = ref<(typeof ranges)[number]>('24h')
const selectedMonitor = ref<string>('all')
const loading = ref(false)
const rows = ref<CadenceRow[]>([])
const monitors = ref<MonitorOption[]>([])

const onTimeRate = (row: CadenceRow) => {
  if (row.delta_samples <= 0) return 0
  return (row.on_time_count / row.delta_samples) * 100
}

const freshnessLabel = (seconds: number) => {
  if (seconds <= 0) return 'now'
  if (seconds < 60) return `${seconds}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  return `${Math.floor(seconds / 3600)}h ago`
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
    name: it?.name || it?.id || 'Monitor',
  }))
}

async function loadDiagnostics() {
  loading.value = true
  try {
    const params = new URLSearchParams({ range: selectedRange.value })
    if (selectedMonitor.value !== 'all') {
      params.set('monitor_id', selectedMonitor.value)
    }
    const res = await fetch(`/api/v1/monitoring/diagnostics/cadence?${params.toString()}`)
    if (!res.ok) return
    const payload = await res.json()
    rows.value = Array.isArray(payload.monitors) ? payload.monitors : []
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
            <CardTitle>Cadence diagnostics</CardTitle>
            <CardDescription>Scheduler timing consistency and missed/duplicate check windows.</CardDescription>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <Select :model-value="selectedRange" @update:model-value="onRangeChange">
              <SelectTrigger class="w-28">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="range in ranges" :key="range" :value="range">
                  Last {{ range }}
                </SelectItem>
              </SelectContent>
            </Select>
            <Select :model-value="selectedMonitor" @update:model-value="onMonitorChange">
              <SelectTrigger class="w-52">
                <SelectValue placeholder="All monitors" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All monitors</SelectItem>
                <SelectItem v-for="monitor in monitors" :key="monitor.id" :value="monitor.id || ''">
                  {{ monitor.name }}
                </SelectItem>
              </SelectContent>
            </Select>
            <Button type="button" variant="outline" size="sm" @click="loadDiagnostics">
              <RefreshCw class="mr-1 size-3.5" />
              Refresh
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Monitor</TableHead>
                <TableHead>Interval</TableHead>
                <TableHead>p50 / p95</TableHead>
                <TableHead>Missed</TableHead>
                <TableHead>Duplicate</TableHead>
                <TableHead>On-time</TableHead>
                <TableHead>Freshness</TableHead>
                <TableHead>Last check</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-if="loading">
                <TableCell colspan="8" class="h-16 text-center text-muted-foreground">
                  Loading diagnostics...
                </TableCell>
              </TableRow>
              <TableRow v-else-if="!sortedRows.length">
                <TableCell colspan="8" class="h-16 text-center text-muted-foreground">
                  No diagnostics data yet.
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
  </div>
</template>
