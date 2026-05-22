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
import {
  buildMonitoringChartData,
  monitoringRangeDurationMs,
  parseMonitoringTime,
  type MonitoringChartPoint,
  type MonitoringChartRange,
  type MonitoringCheck,
} from '@/lib/monitoring-chart'
import { monitorTypeIcon } from '@/lib/monitor-icons'

type StatusMarker = {
  ts: number
  type: 'down' | 'recovered'
}

type AnomalySeverity = 'elevated' | 'critical'

type AnomalyZone = {
  severity: AnomalySeverity
  startIndex: number
  endIndex: number
  startTs: number
  endTs: number
}

const props = defineProps<{
  monitorName?: string
  monitorType?: string
  checks: MonitoringCheck[]
  range: MonitoringChartRange
  zoomFromTs?: number | null
  zoomToTs?: number | null
}>()
const emit = defineEmits<{
  'update:range': [value: MonitoringChartRange]
  'update:zoom-window': [value: { from: number; to: number } | null]
}>()
const range = computed({
  get: () => props.range,
  set: (value: MonitoringChartRange) => emit('update:range', value),
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

const chartData = computed<MonitoringChartPoint[]>(() => buildMonitoringChartData(props.checks, range.value))

const chartSeriesData = computed(() =>
  chartData.value.map((point) => ({
    ts: point.ts,
    ms: point.ms,
    rawMs: point.rawMs,
  })),
)


const hasRenderableSeries = computed(() =>
  chartSeriesData.value.some((point) => Number.isFinite(point.ms)),
)

const chartMax = computed(() => {
  const values = chartSeriesData.value
    .map((point) => point.rawMs)
    .filter((ms): ms is number => Number.isFinite(ms))
    .sort((a, b) => a - b)
  if (!values.length) return 10
  const p95Index = Math.max(0, Math.ceil(values.length * 0.95) - 1)
  const referenceMax = values[Math.min(values.length - 1, p95Index)]
  return Math.max(10, Math.ceil(referenceMax * 1.3))
})

const chartYTicks = computed(() => {
  const ceiling = chartMax.value
  return [0, ceiling * 0.25, ceiling * 0.5, ceiling * 0.75, ceiling]
})

const plottedSeriesData = computed(() =>
  chartSeriesData.value.map((point) => ({
    ts: point.ts,
    ms: Number.isFinite(point.ms) ? Math.min(point.ms, chartMax.value) : point.ms,
    rawMs: point.rawMs,
  })),
)

const anomalyThresholds = computed(() => {
  const values = chartSeriesData.value
    .map((point) => point.rawMs)
    .filter((ms): ms is number => Number.isFinite(ms))
    .sort((a, b) => a - b)
  if (!values.length) {
    return { elevatedMs: 800, criticalMs: 1400 }
  }
  const median = values[Math.floor(values.length / 2)] || 0
  return {
    elevatedMs: Math.max(800, Math.round(median * 1.35)),
    criticalMs: Math.max(1400, Math.round(median * 1.85)),
  }
})

const anomalyZones = computed<AnomalyZone[]>(() => {
  const minimumRunLength = 3
  const points = plottedSeriesData.value
  const zones: AnomalyZone[] = []
  let runSeverity: AnomalySeverity | null = null
  let runStart = -1

  // Only tint sustained regions so isolated spikes stay part of the calm blue baseline.
  const severityAt = (rawMs: number): AnomalySeverity | null => {
    if (!Number.isFinite(rawMs)) return null
    if (rawMs >= anomalyThresholds.value.criticalMs) return 'critical'
    if (rawMs >= anomalyThresholds.value.elevatedMs) return 'elevated'
    return null
  }

  const flushRun = (endIndexExclusive: number) => {
    if (!runSeverity || runStart < 0) return
    const runLength = endIndexExclusive - runStart
    if (runLength >= minimumRunLength) {
      const startIndex = runStart
      const endIndex = endIndexExclusive - 1
      zones.push({
        severity: runSeverity,
        startIndex,
        endIndex,
        startTs: points[startIndex].ts,
        endTs: points[endIndex].ts,
      })
    }
    runSeverity = null
    runStart = -1
  }

  for (let index = 0; index < points.length; index++) {
    const nextSeverity = severityAt(points[index].rawMs)
    if (nextSeverity === runSeverity) continue
    flushRun(index)
    if (nextSeverity) {
      runSeverity = nextSeverity
      runStart = index
    }
  }
  flushRun(points.length)

  return zones
})

function anomalySegmentsForSeverity(severity: AnomalySeverity) {
  return anomalyZones.value
    .filter((zone) => zone.severity === severity)
    .map((zone) => {
      const start = Math.max(0, zone.startIndex - 1)
      const end = Math.min(plottedSeriesData.value.length - 1, zone.endIndex + 1)
      return plottedSeriesData.value.slice(start, end + 1).map((point) => ({
        ts: point.ts,
        ms: point.ms,
        rawMs: point.rawMs,
      }))
    })
    .filter((segment) => segment.length >= 2)
}

const elevatedAnomalySegments = computed(() => anomalySegmentsForSeverity('elevated'))
const criticalAnomalySegments = computed(() => anomalySegmentsForSeverity('critical'))

function yTickFormat(value: number | Date) {
  const raw = value instanceof Date ? value.getTime() : Number(value)
  if (!Number.isFinite(raw)) return ''
  if (raw >= 100) return String(Math.round(raw))
  if (raw >= 10) return String(Math.round(raw * 10) / 10).replace(/\.0$/, '')
  return String(Math.round(raw * 100) / 100).replace(/(\.\d*[1-9])0+$|\.0+$/, '$1')
}

const rangeSummary = computed(() => {
  const points = chartData.value
  const now = Date.now()
  const endTs = points.length ? Math.max(now, points[points.length - 1].ts) : now
  const startTs = endTs - monitoringRangeDurationMs(range.value)
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
  const point = plottedSeriesData.value[hoveredIndex.value] || null
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
      minTs: now - monitoringRangeDurationMs(range.value),
      maxTs: now,
    }
  }
  const maxTs = Math.max(now, points[points.length - 1].ts)
  const minTs = maxTs - monitoringRangeDurationMs(range.value)
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

const statusMarkers = computed<StatusMarker[]>(() => {
  const rows = props.checks
    .map((row) => ({
      ts: parseMonitoringTime(row.checked_at),
      status: String(row.status || '').trim().toLowerCase(),
    }))
    .filter((row) => row.status === 'up' || row.status === 'down')
    .filter((row) => row.ts >= displayBounds.value.minTs && row.ts <= displayBounds.value.maxTs)
    .sort((a, b) => a.ts - b.ts)

  if (!rows.length) return []

  const markers: StatusMarker[] = []
  let previous = ''
  for (const row of rows) {
    if (!previous) {
      previous = row.status
      continue
    }
    if (row.status === previous) continue
    markers.push({
      ts: row.ts,
      type: row.status === 'down' ? 'down' : 'recovered',
    })
    previous = row.status
  }

  return markers
})

function markerLeftStyle(ts: number): string {
  const minTs = displayBounds.value.minTs
  const maxTs = displayBounds.value.maxTs
  if (maxTs <= minTs) return `${PLOT_LEFT}px`
  const ratio = (ts - minTs) / (maxTs - minTs)
  const clamped = Math.max(0, Math.min(1, ratio))
  return `calc(${PLOT_LEFT}px + ${clamped} * (100% - ${PLOT_LEFT + PLOT_RIGHT}px))`
}

function anomalyBandStyle(zone: AnomalyZone) {
  const minTs = displayBounds.value.minTs
  const maxTs = displayBounds.value.maxTs
  if (maxTs <= minTs) {
    return {
      left: `${PLOT_LEFT}px`,
      width: '0px',
    }
  }
  const leftRatio = Math.max(0, Math.min(1, (zone.startTs - minTs) / (maxTs - minTs)))
  const rightRatio = Math.max(0, Math.min(1, (zone.endTs - minTs) / (maxTs - minTs)))
  const widthRatio = Math.max(0, rightRatio - leftRatio)
  return {
    left: `calc(${PLOT_LEFT}px + ${leftRatio} * (100% - ${PLOT_LEFT + PLOT_RIGHT}px))`,
    width: `calc(${widthRatio} * (100% - ${PLOT_LEFT + PLOT_RIGHT}px))`,
  }
}

function markerLabel(marker: StatusMarker): string {
  const label = marker.type === 'down' ? 'Down' : 'Recovered'
  const when = new Date(marker.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  return `${label} at ${when}`
}

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
  if (!plottedSeriesData.value.length) return
  const hoverableIndexes = plottedSeriesData.value
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
  let nearestDelta = Math.abs(plottedSeriesData.value[nearestIdx].ts - hoverTs)
  for (let i = 1; i < hoverableIndexes.length; i++) {
    const idx = hoverableIndexes[i]
    const delta = Math.abs(plottedSeriesData.value[idx].ts - hoverTs)
    if (delta < nearestDelta) {
      nearestDelta = delta
      nearestIdx = idx
    }
  }
  hoveredIndex.value = nearestIdx
  const pointTs = plottedSeriesData.value[nearestIdx].ts
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
  <Card class="chart-shell pt-0">
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
        class="chart-panel relative h-[320px] w-full rounded-md border border-border p-3"
        @mousedown="startBrush"
        @mousemove="onChartMove"
        @mouseup="endBrush"
        @mouseleave="clearHover"
        @dblclick="resetZoom"
      >
        <div
          v-for="zone in anomalyZones"
          :key="`${zone.severity}:${zone.startTs}:${zone.endTs}`"
          class="pointer-events-none absolute inset-y-3 z-0 rounded-md"
          :class="zone.severity === 'critical' ? 'chart-anomaly-band chart-anomaly-band--critical' : 'chart-anomaly-band chart-anomaly-band--elevated'"
          :style="anomalyBandStyle(zone)"
        />
        <div
          v-for="marker in statusMarkers"
          :key="`${marker.type}:${marker.ts}`"
          class="pointer-events-none absolute inset-y-3 z-10 w-px border-l border-dashed opacity-80"
          :class="marker.type === 'down' ? 'border-rose-400/80' : 'border-emerald-400/80'"
          :style="{ left: markerLeftStyle(marker.ts) }"
          :title="markerLabel(marker)"
        />
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
          <div class="font-medium text-foreground">
            {{ hoveredPoint.rawMs }}ms
            <span v-if="hoveredPoint.rawMs > chartMax" class="text-[10px] text-muted-foreground">(clipped)</span>
          </div>
          <div class="text-muted-foreground">{{ hoveredLabel }}</div>
        </div>
        <ChartContainer :config="chartConfig" class="h-full w-full !aspect-auto !justify-start !block">
          <div v-if="!chartData.length" class="flex h-full items-center justify-center text-xs text-muted-foreground">
            {{ t('monitorDetail.noChecksYet') }}
          </div>
          <VisXYContainer
            v-else
            :data="plottedSeriesData"
            :height="280"
            :duration="0"
            :xScale="xScale"
            :xDomain="chartXDomain"
            :yDomain="[0, chartMax]"
            class="h-full w-full"
          >
            <VisArea v-if="hasRenderableSeries" :x="x" :y="y" :duration="0" color="var(--chart-2)" :opacity="0.18" />
            <VisLine v-if="hasRenderableSeries" :x="x" :y="y" :duration="0" color="var(--chart-2)" />
            <template v-for="(segment, segmentIndex) in elevatedAnomalySegments" :key="`elevated-${segmentIndex}`">
              <VisArea
                :data="segment"
                :x="x"
                :y="y"
                :duration="0"
                color="var(--color-amber-400)"
                :opacity="0.08"
              />
              <VisLine
                :data="segment"
                :x="x"
                :y="y"
                :duration="0"
                color="var(--color-amber-400)"
              />
            </template>
            <template v-for="(segment, segmentIndex) in criticalAnomalySegments" :key="`critical-${segmentIndex}`">
              <VisArea
                :data="segment"
                :x="x"
                :y="y"
                :duration="0"
                color="var(--destructive)"
                :opacity="0.09"
              />
              <VisLine
                :data="segment"
                :x="x"
                :y="y"
                :duration="0"
                color="var(--destructive)"
              />
            </template>
            <VisAxis type="x" :numTicks="6" :tickFormat="xTickFormat" />
            <VisAxis type="y" :tickValues="chartYTicks" :tickFormat="yTickFormat" />
          </VisXYContainer>
        </ChartContainer>
      </div>
    </CardContent>
  </Card>
</template>

<style scoped>
.chart-shell {
  background:
    linear-gradient(180deg, rgb(255 255 255 / 0.012), rgb(255 255 255 / 0.006)),
    var(--card);
  box-shadow:
    inset 0 1px 0 rgb(255 255 255 / 0.018),
    0 0 0 1px color-mix(in oklab, var(--border) 70%, transparent);
}

.chart-panel {
  background:
    radial-gradient(circle at 18% 12%, color-mix(in oklab, var(--chart-2) 7%, transparent), transparent 26%),
    linear-gradient(180deg, rgb(255 255 255 / 0.01), rgb(255 255 255 / 0.004)),
    color-mix(in oklab, var(--background) 88%, var(--card));
  box-shadow:
    inset 0 1px 0 rgb(255 255 255 / 0.015),
    0 0 0 1px color-mix(in oklab, var(--border) 55%, transparent),
    0 12px 28px rgb(3 8 14 / 0.14);
}

.chart-panel :deep(.vis-axis-grid line) {
  stroke: rgb(255 255 255 / 0.045);
}

.chart-panel :deep(.vis-axis line),
.chart-panel :deep(.vis-axis path) {
  stroke: rgb(255 255 255 / 0.06);
}

.chart-panel :deep(.vis-axis text) {
  fill: color-mix(in oklab, var(--muted-foreground) 88%, transparent);
}

.chart-panel :deep(path[stroke='var(--chart-2)']) {
  filter: drop-shadow(0 0 8px color-mix(in oklab, var(--chart-2) 36%, transparent));
  stroke-width: 2.1px;
}

.chart-panel :deep(path[stroke='var(--color-amber-400)']) {
  filter: drop-shadow(0 0 6px rgb(251 191 36 / 0.28));
  stroke-width: 2.15px;
}

.chart-panel :deep(path[stroke='var(--destructive)']) {
  filter: drop-shadow(0 0 6px color-mix(in oklab, var(--destructive) 32%, transparent));
  stroke-width: 2.2px;
}

.chart-anomaly-band {
  backdrop-filter: blur(0.5px);
}

.chart-anomaly-band--elevated {
  background: linear-gradient(180deg, rgb(251 191 36 / 0.08), rgb(251 191 36 / 0.02));
}

.chart-anomaly-band--critical {
  background: linear-gradient(180deg, color-mix(in oklab, var(--destructive) 12%, transparent), color-mix(in oklab, var(--destructive) 3%, transparent));
}
</style>
