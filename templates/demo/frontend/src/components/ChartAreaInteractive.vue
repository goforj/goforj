<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { VisArea, VisAxis, VisLine, VisXYContainer } from '@unovis/vue'
import { Scale } from '@unovis/ts'
import { Activity, RotateCcw } from 'lucide-vue-next'
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
import { Button } from '@/components/ui/button'
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
  zoomFromTs?: number | null
  zoomToTs?: number | null
}>()
const emit = defineEmits<{
  'update:range': [value: '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '7d' | '30d']
  'update:zoom-window': [value: { from: number; to: number } | null]
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
const PLOT_LEFT = 56
const PLOT_RIGHT = 16

type ChartPoint = {
  ts: number
  ms: number
}

const HOLE_VALUE = Number.NaN

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
  const baselineDelta = sortedDeltas.length
    ? sortedDeltas[Math.floor((sortedDeltas.length - 1) * 0.2)]
    : medianDelta
  const pollingIntervalMs = Math.max(1_000, baselineDelta)
  const nullGapThresholdMs = pollingIntervalMs * 5
  const carryThresholdMs = Math.max(30_000, Math.min(10 * 60 * 1000, medianDelta * 3))
  const filled = [...normalized]
  if (filled[0].ts > startTs) {
    const leftGapMs = filled[0].ts - startTs
    if (leftGapMs <= carryThresholdMs) {
      filled.unshift({ ts: startTs, ms: filled[0].ms })
    } else {
      const leftStopTs = Math.max(startTs, filled[0].ts - 1)
      filled.unshift({ ts: leftStopTs, ms: HOLE_VALUE })
      filled.unshift({ ts: startTs, ms: HOLE_VALUE })
    }
  }
  if (filled[filled.length - 1].ts < endTs) {
    const rightGapMs = endTs - filled[filled.length - 1].ts
    if (rightGapMs <= carryThresholdMs) {
      filled.push({ ts: endTs, ms: filled[filled.length - 1].ms })
    } else {
      const rightStartTs = Math.min(endTs, filled[filled.length - 1].ts + 1)
      filled.push({ ts: rightStartTs, ms: HOLE_VALUE })
      filled.push({ ts: endTs, ms: HOLE_VALUE })
    }
  }
  const segmented: ChartPoint[] = []
  for (let i = 0; i < filled.length; i++) {
    const point = filled[i]
    if (segmented.length > 0) {
      const prev = filled[i - 1]
      const delta = point.ts - prev.ts
      if (delta > nullGapThresholdMs) {
        const leftBreakTs = Math.min(point.ts - 1, prev.ts + 1)
        const rightBreakTs = Math.max(prev.ts + 1, point.ts - 1)
        segmented.push({ ts: leftBreakTs, ms: HOLE_VALUE })
        segmented.push({ ts: rightBreakTs, ms: HOLE_VALUE })
      }
    }
    segmented.push({ ts: point.ts, ms: point.ms })
  }
  return segmented
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
const brushing = ref(false)
const brushStartX = ref(0)
const brushCurrentX = ref(0)
const zoomBounds = ref<{ minTs: number; maxTs: number } | null>(null)

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

const displayBounds = computed(() => zoomBounds.value || chartBounds.value)

const chartXDomain = computed<[Date, Date]>(() => [
  new Date(displayBounds.value.minTs),
  new Date(displayBounds.value.maxTs),
])

const brushOverlay = computed(() => {
  if (!brushing.value) return null
  const left = Math.min(brushStartX.value, brushCurrentX.value)
  const right = Math.max(brushStartX.value, brushCurrentX.value)
  return {
    left,
    width: Math.max(0, right - left),
  }
})

function plotMetrics(target: HTMLElement) {
  const rect = target.getBoundingClientRect()
  const plotWidth = Math.max(1, rect.width - PLOT_LEFT - PLOT_RIGHT)
  return { rect, plotWidth }
}

function clampPlotX(rawX: number, plotWidth: number): number {
  return Math.max(0, Math.min(plotWidth, rawX))
}

function tsAtPlotX(plotX: number, plotWidth: number): number {
  const ratio = plotX / plotWidth
  const minTs = displayBounds.value.minTs
  const maxTs = displayBounds.value.maxTs
  if (maxTs <= minTs) return minTs
  return minTs + ratio * (maxTs - minTs)
}

function startBrush(event: MouseEvent) {
  if (event.button !== 0) return
  const target = event.currentTarget as HTMLElement | null
  if (!target) return
  const { rect, plotWidth } = plotMetrics(target)
  const rawX = event.clientX - rect.left - PLOT_LEFT
  const x = clampPlotX(rawX, plotWidth)
  brushing.value = true
  brushStartX.value = x
  brushCurrentX.value = x
  hoveredIndex.value = null
}

function endBrush(event: MouseEvent) {
  if (!brushing.value) return
  const target = event.currentTarget as HTMLElement | null
  if (!target) {
    brushing.value = false
    return
  }
  const { rect, plotWidth } = plotMetrics(target)
  const rawX = event.clientX - rect.left - PLOT_LEFT
  brushCurrentX.value = clampPlotX(rawX, plotWidth)
  const dragPx = Math.abs(brushCurrentX.value - brushStartX.value)
  brushing.value = false
  if (dragPx < 6) return

  const startTs = tsAtPlotX(Math.min(brushStartX.value, brushCurrentX.value), plotWidth)
  const endTs = tsAtPlotX(Math.max(brushStartX.value, brushCurrentX.value), plotWidth)
  if (endTs <= startTs) return
  zoomBounds.value = { minTs: startTs, maxTs: endTs }
  emit('update:zoom-window', { from: startTs, to: endTs })
}

function resetZoom() {
  zoomBounds.value = null
  emit('update:zoom-window', null)
}

function xTickFormat(value: number | Date) {
  const ts = value instanceof Date ? value.getTime() : Number(value)
  return formatAxisTime(ts)
}

function onChartMove(event: MouseEvent) {
  const target = event.currentTarget as HTMLElement | null
  if (!target) return
  const { rect, plotWidth } = plotMetrics(target)
  const rawX = event.clientX - rect.left - PLOT_LEFT
  const clampedX = clampPlotX(rawX, plotWidth)
  if (brushing.value) {
    brushCurrentX.value = clampedX
    return
  }
  if (!chartData.value.length) return
  const hoverableIndexes = chartData.value
    .map((point, idx) => ({ idx, point }))
    .filter(({ point }) => Number.isFinite(point.ms))
    .map(({ idx }) => idx)
  if (!hoverableIndexes.length) return
  const ratio = clampedX / plotWidth
  const minTs = displayBounds.value.minTs
  const maxTs = displayBounds.value.maxTs
  if (maxTs <= minTs) {
    hoveredIndex.value = 0
    hoverX.value = PLOT_LEFT
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
  hoverX.value = PLOT_LEFT + pointRatio * plotWidth
}

function clearHover() {
  if (brushing.value) return
  hoveredIndex.value = null
}

watch(
  () => props.range,
  () => {
    if (zoomBounds.value) {
      emit('update:zoom-window', null)
    }
    zoomBounds.value = null
  },
)

watch(
  () => [props.zoomFromTs, props.zoomToTs] as const,
  ([from, to]) => {
    if (Number.isFinite(from) && Number.isFinite(to) && Number(to) > Number(from)) {
      zoomBounds.value = { minTs: Number(from), maxTs: Number(to) }
      return
    }
    zoomBounds.value = null
  },
  { immediate: true },
)
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
        <Button
          v-if="zoomBounds"
          size="sm"
          variant="outline"
          class="h-6 gap-1 px-2 text-[11px]"
          @click="resetZoom"
        >
          <RotateCcw class="size-3.5" />
          reset zoom
        </Button>
      </div>
      <div
        class="relative h-[320px] w-full rounded-md border border-border bg-background/40 p-3"
        @mousedown="startBrush"
        @mousemove="onChartMove"
        @mouseup="endBrush"
        @mouseleave="clearHover"
        @dblclick="resetZoom"
      >
        <div
          v-if="brushOverlay"
          class="pointer-events-none absolute inset-y-3 z-10 bg-primary/15 border-x border-primary/40"
          :style="{ left: `${PLOT_LEFT + brushOverlay.left}px`, width: `${brushOverlay.width}px` }"
        />
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
