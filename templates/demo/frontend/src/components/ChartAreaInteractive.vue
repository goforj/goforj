<script setup lang="ts">
import { computed, ref } from 'vue'
import { VisArea, VisAxis, VisLine, VisXYContainer } from '@unovis/vue'
import { Scale } from '@unovis/ts'
import { Activity } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'
import { monitorTypeIcon } from '@/lib/monitor-icons'

type Check = {
  checked_at?: string
  duration_ms?: number
  status?: string
}

const props = defineProps<{
  monitorName?: string
  monitorType?: string
  checks: Check[]
  range: '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'
}>()
const emit = defineEmits<{
  'update:range': [value: '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d']
}>()
const range = computed({
  get: () => props.range,
  set: (value: '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d') => emit('update:range', value),
})
const { t } = useI18n()
const latencyTitleIcon = computed(() => monitorTypeIcon(props.monitorType))

const chartConfig = {
  latency: {
    label: 'Latency',
    color: 'var(--chart-2)',
  },
} satisfies ChartConfig

const xScale = Scale.scaleTime()

type ChartPoint = {
  ts: number
  ms: number
}

function parseTime(value?: string): number {
  if (!value) return Date.now()
  const t = Date.parse(value)
  if (Number.isNaN(t)) return Date.now()
  return t
}

function horizonDurationMs(value: '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d'): number {
  if (value === '15m') return 15 * 60 * 1000
  if (value === '1h') return 60 * 60 * 1000
  if (value === '3h') return 3 * 60 * 60 * 1000
  if (value === '6h') return 6 * 60 * 60 * 1000
  if (value === '12h') return 12 * 60 * 60 * 1000
  if (value === '7d') return 7 * 24 * 60 * 60 * 1000
  if (value === '30d') return 30 * 24 * 60 * 60 * 1000
  return 24 * 60 * 60 * 1000
}

const chartData = computed<ChartPoint[]>(() => {
  const horizonMs = horizonDurationMs(range.value)
  const mapped: Array<{ ts: number; ms: number }> = []
  let looksAscending = true
  let looksDescending = true
  let prevTs: number | null = null
  for (const row of props.checks) {
    const ts = parseTime(row.checked_at)
    const status = String(row.status || '').toLowerCase()
    const ms = status === 'paused' ? 0 : Math.max(0, Number(row.duration_ms || 0))
    mapped.push({ ts, ms })
    if (prevTs !== null) {
      if (ts < prevTs) looksAscending = false
      if (ts > prevTs) looksDescending = false
    }
    prevTs = ts
  }
  if (!looksAscending) {
    if (looksDescending) {
      mapped.reverse()
    } else {
      mapped.sort((a, b) => a.ts - b.ts)
    }
  }

  if (!mapped.length) {
    return []
  }

  const endTs = mapped[mapped.length - 1].ts
  const startTs = endTs - horizonMs
  const windowRows = mapped.filter((row) => row.ts >= startTs && row.ts <= endTs)
  if (!windowRows.length) {
    return []
  }

  const merged = new Map<number, number>()
  for (const row of windowRows) {
    merged.set(row.ts, row.ms)
  }
  const normalized = Array.from(merged.entries())
    .map(([ts, ms]) => ({ ts, ms }))
    .sort((a, b) => a.ts - b.ts)

  if (!normalized.length) {
    return []
  }
  const deltas: number[] = []
  for (let i = 1; i < normalized.length; i++) {
    const delta = normalized[i].ts - normalized[i - 1].ts
    if (delta > 0) deltas.push(delta)
  }
  const sortedDeltas = [...deltas].sort((a, b) => a - b)
  const medianDelta = sortedDeltas.length
    ? sortedDeltas[Math.floor(sortedDeltas.length / 2)]
    : 60_000
  const carryThresholdMs = Math.max(30_000, Math.min(10 * 60 * 1000, medianDelta * 3))
  const filled = [...normalized]
  if (filled[0].ts > startTs) {
    const leftGapMs = filled[0].ts - startTs
    if (leftGapMs <= carryThresholdMs) {
      filled.unshift({ ts: startTs, ms: filled[0].ms })
    } else {
      const leftStopTs = Math.max(startTs, filled[0].ts - 1)
      filled.unshift({ ts: leftStopTs, ms: 0 })
      filled.unshift({ ts: startTs, ms: 0 })
    }
  }
  if (filled[filled.length - 1].ts < endTs) {
    const rightGapMs = endTs - filled[filled.length - 1].ts
    if (rightGapMs <= carryThresholdMs) {
      filled.push({ ts: endTs, ms: filled[filled.length - 1].ms })
    } else {
      const rightStartTs = Math.min(endTs, filled[filled.length - 1].ts + 1)
      filled.push({ ts: rightStartTs, ms: 0 })
      filled.push({ ts: endTs, ms: 0 })
    }
  }
  return filled.map((row) => ({
    ts: row.ts,
    ms: row.ms,
  }))
})

const chartSeriesData = computed(() =>
  chartData.value.map((point) => ({
    ts: point.ts,
    ms: point.ms,
  })),
)


const hasRenderableSeries = computed(() =>
  chartSeriesData.value.some((point) => Number.isFinite(point.ms)),
)

const chartMax = computed(() => {
  const values = chartSeriesData.value
    .map((point) => point.ms)
    .filter((ms): ms is number => Number.isFinite(ms))
    .sort((a, b) => a - b)
  if (!values.length) return 1000
  const absoluteMax = values[values.length - 1]
  return Math.max(100, Math.ceil(absoluteMax * 1.1))
})

const rangeSummary = computed(() => {
  const points = chartData.value
  const now = Date.now()
  const endTs = points.length ? points[points.length - 1].ts : now
  const startTs = endTs - horizonDurationMs(range.value)
  const startLabel = new Date(startTs).toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
  const endLabel = new Date(endTs).toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
  return `${points.length} points · ${startLabel} - ${endLabel}`
})

const x = (d: { ts: number }) => new Date(d.ts)
const y = (d: { ms: number }) => d.ms

const hoveredIndex = ref<number | null>(null)
const hoverX = ref<number>(0)

const hoveredPoint = computed(() => {
  if (hoveredIndex.value === null) return null
  const point = chartData.value[hoveredIndex.value] || null
  if (!point) return null
  return point
})

const hoveredLabel = computed(() => {
  if (!hoveredPoint.value) return ''
  const date = new Date(hoveredPoint.value.ts)
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
})

function formatAxisTime(ts: number): string {
  const date = new Date(ts)
  if (range.value === '7d' || range.value === '30d') {
    return date.toLocaleString([], { month: 'short', day: 'numeric' })
  }
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

const chartBounds = computed(() => {
  const now = Date.now()
  const points = chartData.value
  if (!points.length) {
    return {
      minTs: now - horizonDurationMs(range.value),
      maxTs: now,
    }
  }
  const maxTs = points[points.length - 1].ts
  const minTs = maxTs - horizonDurationMs(range.value)
  return {
    minTs,
    maxTs,
  }
})

const chartXDomain = computed<[Date, Date]>(() => [
  new Date(chartBounds.value.minTs),
  new Date(chartBounds.value.maxTs),
])

function xTickFormat(value: number | Date) {
  const ts = value instanceof Date ? value.getTime() : Number(value)
  return formatAxisTime(ts)
}

function onChartMove(event: MouseEvent) {
  if (!chartData.value.length) return
  const hoverableIndexes = chartData.value
    .map((point, idx) => ({ idx, point }))
    .filter(({ point }) => Number.isFinite(point.ms))
    .map(({ idx }) => idx)
  if (!hoverableIndexes.length) return
  const target = event.currentTarget as HTMLElement | null
  if (!target) return
  const rect = target.getBoundingClientRect()
  if (rect.width <= 0) return
  // Match hover coordinate to the actual series plotting area, not full card width.
  const plotLeft = 56
  const plotRight = 16
  const plotWidth = Math.max(1, rect.width - plotLeft - plotRight)
  const rawX = event.clientX - rect.left - plotLeft
  const clampedX = Math.max(0, Math.min(plotWidth, rawX))
  const ratio = clampedX / plotWidth
  const minTs = chartBounds.value.minTs
  const maxTs = chartBounds.value.maxTs
  if (maxTs <= minTs) {
    hoveredIndex.value = 0
    hoverX.value = plotLeft
    return
  }
  const hoverTs = minTs + ratio * (maxTs - minTs)
  let nearestIdx = hoverableIndexes[0]
  let nearestDelta = Math.abs(chartData.value[nearestIdx].ts - hoverTs)
  for (let i = 1; i < hoverableIndexes.length; i++) {
    const idx = hoverableIndexes[i]
    const delta = Math.abs(chartData.value[idx].ts - hoverTs)
    if (delta < nearestDelta) {
      nearestDelta = delta
      nearestIdx = idx
    }
  }
  hoveredIndex.value = nearestIdx
  const pointTs = chartData.value[nearestIdx].ts
  const pointRatio = (pointTs - minTs) / (maxTs - minTs)
  hoverX.value = plotLeft + pointRatio * plotWidth
}

function clearHover() {
  hoveredIndex.value = null
}
</script>

<template>
  <Card class="pt-0">
    <CardHeader class="flex items-center gap-2 space-y-0 border-b py-5 sm:flex-row">
      <div class="grid flex-1 gap-1">
        <CardTitle class="flex items-center gap-2">
          <component :is="latencyTitleIcon" class="size-4 text-muted-foreground" />
          <span>{{ t('chart.latencyTrend') }}</span>
        </CardTitle>
        <CardDescription>
          {{ t('chart.monitorResponseHistory', { monitor: monitorName || t('monitoring.monitorFallback') }) }}
        </CardDescription>
      </div>
      <Select v-model="range">
        <SelectTrigger class="hidden w-[140px] rounded-lg sm:ml-auto sm:flex">
          <SelectValue placeholder="24h" />
        </SelectTrigger>
        <SelectContent class="rounded-xl">
          <SelectItem value="15m" class="rounded-lg">{{ t('common.lastRange', { range: '15m' }) }}</SelectItem>
          <SelectItem value="1h" class="rounded-lg">{{ t('common.lastRange', { range: '1h' }) }}</SelectItem>
          <SelectItem value="3h" class="rounded-lg">{{ t('common.lastRange', { range: '3h' }) }}</SelectItem>
          <SelectItem value="6h" class="rounded-lg">{{ t('common.lastRange', { range: '6h' }) }}</SelectItem>
          <SelectItem value="12h" class="rounded-lg">{{ t('common.lastRange', { range: '12h' }) }}</SelectItem>
          <SelectItem value="24h" class="rounded-lg">{{ t('common.lastRange', { range: '24h' }) }}</SelectItem>
          <SelectItem value="7d" class="rounded-lg">{{ t('common.lastRange', { range: '7d' }) }}</SelectItem>
          <SelectItem value="30d" class="rounded-lg">{{ t('common.lastRange', { range: '30d' }) }}</SelectItem>
        </SelectContent>
      </Select>
    </CardHeader>
    <CardContent class="px-2 pt-0 sm:px-5 sm:pt-0 pb-4">
      <div class="mb-3 flex items-center gap-4 text-xs text-muted-foreground">
        <div class="flex items-center gap-2">
          <Activity class="size-3.5 text-[var(--chart-2)]" />
          <span>{{ t('chart.responseMs') }}</span>
        </div>
        <div class="ml-auto text-[11px] text-muted-foreground">{{ rangeSummary }}</div>
      </div>
      <div
        class="relative h-[320px] w-full rounded-md border border-border bg-background/40 p-3"
        @mousemove="onChartMove"
        @mouseleave="clearHover"
      >
        <div
          v-if="hoveredPoint"
          class="pointer-events-none absolute inset-y-3 z-10 w-px bg-primary/35"
          :style="{ left: `${hoverX}px` }"
        />
        <div
          v-if="hoveredPoint"
          class="pointer-events-none absolute top-2 z-20 -translate-x-1/2 rounded-md border border-border bg-card/95 px-2 py-1 text-xs shadow-sm"
          :style="{ left: `${hoverX}px` }"
        >
          <div class="font-medium text-foreground">{{ hoveredPoint.ms }}ms</div>
          <div class="text-muted-foreground">{{ hoveredLabel }}</div>
        </div>
        <ChartContainer :config="chartConfig" class="h-full w-full !aspect-auto !justify-start !block">
          <div v-if="!chartData.length" class="flex h-full items-center justify-center text-xs text-muted-foreground">
            {{ t('monitorDetail.noChecksYet') }}
          </div>
          <VisXYContainer
            v-else
            :data="chartSeriesData"
            :height="280"
            :duration="0"
            :xScale="xScale"
            :xDomain="chartXDomain"
            :yDomain="[0, chartMax]"
            class="h-full w-full"
          >
            <VisArea v-if="hasRenderableSeries" :x="x" :y="y" :duration="0" color="var(--chart-2)" :opacity="0.14" />
            <VisLine v-if="hasRenderableSeries" :x="x" :y="y" :duration="0" color="var(--chart-2)" />
            <VisAxis type="x" :numTicks="6" :tickFormat="xTickFormat" />
            <VisAxis type="y" :numTicks="5" />
          </VisXYContainer>
        </ChartContainer>
      </div>
    </CardContent>
  </Card>
</template>
