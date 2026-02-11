<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Activity, BarChart3, Clock3, LoaderCircle, Pause, Pencil, Play, ShieldCheck, Zap } from 'lucide-vue-next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import ChartAreaInteractive from '@/components/ChartAreaInteractive.vue'
import HeartbeatStrip from '@/components/HeartbeatStrip.vue'
import { monitorTypeIcon, monitorTypeLabel, monitorSupportsFavicon } from '@/lib/monitor-icons'
import { displayTargetFromFields } from '@/lib/monitor-target'

type Monitor = {
  id?: string
  name?: string
  type?: string
  monitor_type?: string
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
  timeout_ms?: number
  enabled?: boolean
  updated_at?: string
}

type Check = {
  id?: string
  checked_at?: string
  status?: string
  status_code?: number
  duration_ms?: number
  error_message?: string
}

const props = defineProps<{
  monitor: Monitor | null
  heartbeatStatuses?: string[]
  heartbeatPoints?: Array<{ status?: string; checked_at?: string; latency_ms?: number }>
  checks: Check[]
  checkRange: '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'
  incidents: Array<{ id?: string; opened_at?: string; resolved_at?: string | null; summary?: string }>
  stats?: {
    sample_count?: number
    uptime_pct?: number
    p50_ms?: number
    p95_ms?: number
  } | null
  checkNowLoading?: boolean
}>()

const emit = defineEmits<{
  toggleEnabled: [id: string, enabled: boolean]
  checkNow: [id: string]
  'update:check-range': [value: '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d']
}>()

const safeChecks = computed(() => (Array.isArray(props.checks) ? props.checks : []))
const safeIncidents = computed(() => (Array.isArray(props.incidents) ? props.incidents : []))

const recentStatuses = computed(() => {
  if (Array.isArray(props.heartbeatStatuses) && props.heartbeatStatuses.length > 0) {
    return props.heartbeatStatuses.map((s) => (s || 'unknown').toLowerCase())
  }
  const newestFirst = safeChecks.value.map((c) => (c.status || 'unknown').toLowerCase())
  const trimmed = newestFirst.slice(0, 30)
  if (!trimmed.length) return Array(30).fill('unknown')
  const reversed = [...trimmed].reverse()
  if (reversed.length < 30) {
    return [...Array(30 - reversed.length).fill('unknown'), ...reversed]
  }
  return reversed
})

const recentPillPoints = computed(() => {
  if (Array.isArray(props.heartbeatPoints) && props.heartbeatPoints.length > 0) {
    return props.heartbeatPoints.map((p) => ({
      status: (p?.status || 'unknown').toLowerCase(),
      checkedAt: p?.checked_at,
      latencyMs: Number(p?.latency_ms || 0),
    }))
  }
  const newest = safeChecks.value.slice(0, 30).map((c) => ({
    status: (c.status || 'unknown').toLowerCase(),
    checkedAt: c.checked_at,
    latencyMs: Number(c.duration_ms || 0),
  }))
  const ordered = [...newest].reverse()
  if (ordered.length < 30) {
    return [...Array(30 - ordered.length).fill(null), ...ordered]
  }
  return ordered
})

const latestCheck = computed(() => safeChecks.value[0] || null)
const latestTerminalCheck = computed(() => {
  for (const row of safeChecks.value) {
    const status = (row.status || '').toLowerCase()
    if (status === 'pending' || status === 'unknown' || !status) {
      continue
    }
    return row
  }
  return null
})

const currentStatus = computed(() => {
  if (props.monitor?.enabled === false) return 'paused'
  const latest = latestCheck.value
  const latestStatus = (latest?.status || 'unknown').toLowerCase()
  if (latestStatus !== 'pending') return latestStatus

  // Pending should be transient. If a pending row lingers past one check interval,
  // fall back to the freshest terminal status so the UI reflects actual state.
  const intervalMs = Math.max(5, Number(props.monitor?.interval_seconds || 60)) * 1000
  const latestAt = latest?.checked_at ? Date.parse(latest.checked_at) : Number.NaN
  const pendingAgeMs = Number.isNaN(latestAt) ? 0 : Date.now() - latestAt
  if (pendingAgeMs <= intervalMs) return 'pending'

  const terminalStatus = (latestTerminalCheck.value?.status || '').toLowerCase()
  return terminalStatus || 'pending'
})
const currentLatency = computed(() => {
  const status = currentStatus.value
  if (status === 'pending') return Number(latestCheck.value?.duration_ms || 0)
  return Number((latestTerminalCheck.value || latestCheck.value)?.duration_ms || 0)
})
const avgLatency24h = computed(() => {
  if (!safeChecks.value.length) return 0
  const values = safeChecks.value.map((c) => Number(c.duration_ms || 0))
  const total = values.reduce((acc, n) => acc + n, 0)
  return Math.round(total / Math.max(1, values.length))
})

const faviconFailed = ref(false)
const faviconSrc = computed(() => {
  const monitorType = props.monitor?.type || props.monitor?.monitor_type
  if (!props.monitor?.id || faviconFailed.value || !monitorSupportsFavicon(monitorType)) return ''
  return `/api/v1/monitoring/monitors/${props.monitor.id}/favicon`
})
const titleIcon = computed(() => monitorTypeIcon(props.monitor?.type || props.monitor?.monitor_type))
const monitorTypeText = computed(() => monitorTypeLabel(props.monitor?.type || props.monitor?.monitor_type))
const displayTarget = computed(() =>
  displayTargetFromFields(props.monitor?.type || props.monitor?.monitor_type || '', {
    target: props.monitor?.target,
    target_url: props.monitor?.target_url,
    target_host: props.monitor?.target_host,
    target_port: props.monitor?.target_port,
    target_record_type: props.monitor?.target_record_type,
    target_keyword: props.monitor?.target_keyword,
    target_expected: props.monitor?.target_expected,
    target_container: props.monitor?.target_container,
    target_docker_host: props.monitor?.target_docker_host,
    target_push_token: props.monitor?.target_push_token,
  }),
)

function formatRelativeTime(value?: string): string {
  if (!value) return 'n/a'
  const dt = new Date(value)
  if (Number.isNaN(dt.getTime())) return 'n/a'
  const diffMs = Date.now() - dt.getTime()
  if (diffMs < 0) return 'just now'
  const sec = Math.floor(diffMs / 1000)
  if (sec < 10) return 'just now'
  if (sec < 60) return `${sec}s ago`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m ago`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr}h ago`
  const day = Math.floor(hr / 24)
  return `${day}d ago`
}

watch(
  () => props.monitor?.id,
  () => {
    faviconFailed.value = false
  },
)
</script>

<template>
  <Card>
    <CardHeader>
      <div class="flex flex-wrap items-start justify-between gap-2">
        <div class="space-y-2.5 pr-2">
          <CardTitle class="flex items-center gap-2">
            <img
              v-if="faviconSrc"
              :src="faviconSrc"
              alt=""
              class="size-5 rounded-sm"
              @error="faviconFailed = true"
            />
            <component v-else :is="titleIcon" class="size-5 text-muted-foreground" />
            <span>{{ props.monitor?.name || 'Monitor Detail' }}</span>
          </CardTitle>
          <div>
            <Badge variant="outline" class="inline-flex h-6 items-center gap-1 px-2 text-xs">
              <component :is="titleIcon" class="size-3.5 text-muted-foreground" />
              <span>{{ monitorTypeText }}</span>
            </Badge>
          </div>
          <CardDescription class="leading-snug">
            <a
              v-if="displayTarget && props.monitor?.target_url"
              :href="props.monitor.target_url"
              target="_blank"
              rel="noopener noreferrer"
              class="block break-all text-emerald-400 underline-offset-2 hover:underline"
            >
              {{ displayTarget }}
            </a>
            <span v-else-if="displayTarget" class="block break-all text-emerald-400">
              {{ displayTarget }}
            </span>
            <span v-else>Select a monitor to inspect history.</span>
          </CardDescription>
        </div>
        <div v-if="props.monitor?.id" class="flex items-center gap-2">
          <Button as-child variant="outline" size="sm">
            <RouterLink :to="`/monitors/${props.monitor.id}/edit`">
              <Pencil class="size-4" />
              Edit
            </RouterLink>
          </Button>
          <Button
            variant="outline"
            size="sm"
            :disabled="props.checkNowLoading"
            @click="emit('checkNow', props.monitor.id)"
          >
            <LoaderCircle v-if="props.checkNowLoading" class="size-4 animate-spin" />
            <Play v-else class="size-4" />
            {{ props.checkNowLoading ? 'Checking...' : 'Check now' }}
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="emit('toggleEnabled', props.monitor.id, !props.monitor.enabled)"
          >
            <Pause v-if="props.monitor.enabled" class="size-4" />
            <Play v-else class="size-4" />
            {{ props.monitor.enabled ? 'Pause' : 'Resume' }}
          </Button>
        </div>
      </div>
    </CardHeader>
    <CardContent class="space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border p-3">
        <div class="flex-1">
          <HeartbeatStrip :statuses="recentStatuses" :points="recentPillPoints" />
          <p class="mt-2 text-xs text-muted-foreground">
            Check every {{ props.monitor?.interval_seconds || 0 }} seconds.
          </p>
        </div>
        <Badge
          class="h-11 rounded-full px-5 text-xl"
          :class="
            currentStatus === 'up'
              ? 'bg-emerald-400 text-background'
              : currentStatus === 'paused'
              ? 'bg-amber-400 text-background'
              : currentStatus === 'pending'
              ? 'bg-yellow-400 text-background'
              : 'bg-rose-500 text-white'
          "
        >
          {{
            currentStatus === 'up'
              ? 'Up'
              : currentStatus === 'paused'
              ? 'Paused'
              : currentStatus === 'pending'
              ? 'Pending'
              : currentStatus === 'down'
              ? 'Down'
              : 'Unknown'
          }}
        </Badge>
      </div>

      <div class="grid gap-2 text-sm sm:grid-cols-2 xl:grid-cols-5">
        <div class="rounded-md border border-border p-2">
          <p class="flex items-center gap-1 text-xs text-muted-foreground">
            <Zap class="size-3.5" />
            Response (Current)
          </p>
          <p class="font-medium">{{ currentLatency }}ms</p>
        </div>
        <div class="rounded-md border border-border p-2">
          <p class="flex items-center gap-1 text-xs text-muted-foreground">
            <Activity class="size-3.5" />
            Avg. Response (24h)
          </p>
          <p class="font-medium">{{ avgLatency24h }}ms</p>
        </div>
        <div class="rounded-md border border-border p-2">
          <p class="flex items-center gap-1 text-xs text-muted-foreground">
            <ShieldCheck class="size-3.5" />
            Uptime (24h)
          </p>
          <p class="font-medium">{{ Number(props.stats?.uptime_pct || 0).toFixed(2) }}%</p>
        </div>
        <div class="rounded-md border border-border p-2">
          <p class="flex items-center gap-1 text-xs text-muted-foreground">
            <BarChart3 class="size-3.5" />
            P95 (24h)
          </p>
          <p class="font-medium">{{ props.stats?.p95_ms || 0 }}ms</p>
        </div>
        <div class="rounded-md border border-border p-2">
          <p class="flex items-center gap-1 text-xs text-muted-foreground">
            <Clock3 class="size-3.5" />
            Checks
          </p>
          <p class="font-medium">{{ props.stats?.sample_count || 0 }}</p>
        </div>
      </div>

      <ChartAreaInteractive
        :monitor-name="props.monitor?.name || 'Monitor'"
        :monitor-type="props.monitor?.type || props.monitor?.monitor_type || ''"
        :checks="safeChecks"
        :range="props.checkRange"
        @update:range="emit('update:check-range', $event)"
      />

      <div class="rounded-md border border-border p-3">
        <p class="mb-2 text-sm font-medium">Recent incidents</p>
        <div v-if="safeIncidents.length" class="space-y-2">
          <div
            v-for="incident in safeIncidents.slice(0, 6)"
            :key="incident.id"
            class="rounded-md border border-border p-2 text-sm"
          >
            <p class="font-medium">{{ incident.summary || 'Incident' }}</p>
            <p class="text-xs text-muted-foreground">
              Opened {{ incident.opened_at || '-' }} ·
              {{ incident.resolved_at ? `resolved ${incident.resolved_at}` : 'open' }}
            </p>
          </div>
        </div>
        <p v-else class="text-sm text-muted-foreground">No incidents for this monitor.</p>
      </div>

      <div class="overflow-hidden rounded-md border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Time</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>HTTP</TableHead>
              <TableHead>Latency</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-if="!safeChecks.length">
              <TableCell colspan="4" class="text-center text-muted-foreground">
                No checks yet.
              </TableCell>
            </TableRow>
            <TableRow v-for="row in safeChecks.slice(0, 10)" :key="row.id">
              <TableCell class="text-xs text-muted-foreground">
                <span :title="row.checked_at || ''">{{ formatRelativeTime(row.checked_at) }}</span>
              </TableCell>
              <TableCell>
                <Badge
                  :variant="row.status === 'up' ? 'default' : 'outline'"
                  :class="
                    row.status === 'pending'
                      ? 'border-yellow-500/40 text-yellow-300'
                      : row.status === 'down'
                      ? 'border-rose-500/40 text-rose-400'
                      : row.status === 'paused'
                      ? 'border-amber-500/40 text-amber-400'
                      : ''
                  "
                >
                  {{ row.status || '-' }}
                </Badge>
              </TableCell>
              <TableCell>{{ row.status_code || '-' }}</TableCell>
              <TableCell>{{ row.duration_ms || 0 }}ms</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </CardContent>
  </Card>
</template>
