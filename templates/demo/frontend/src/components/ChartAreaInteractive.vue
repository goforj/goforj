<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
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
import {
  buildMonitoringChartData,
  monitoringRangeDurationMs,
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

type PlotFrame = {
  left: number
  top: number
  width: number
  height: number
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

const chartPanel = ref<HTMLElement | null>(null)
const chartHost = ref<HTMLElement | null>(null)
const plot = ref<uPlot | null>(null)
const plotFrame = ref<PlotFrame | null>(null)
const hoveredIndex = ref<number | null>(null)
const hoverX = ref(0)
const hoverY = ref(0)
const tooltipX = ref(0)
const tooltipY = ref(0)
const zoomBounds = ref<{ minTs: number; maxTs: number } | null>(null)

let resizeObserver: ResizeObserver | null = null
let rafID = 0

const chartData = computed<MonitoringChartPoint[]>(() => buildMonitoringChartData(props.checks, range.value))

const finiteSeries = computed(() =>
  chartData.value.filter((point) => Number.isFinite(point.ms)),
)

const hasRenderableSeries = computed(() => finiteSeries.value.length > 1)

const chartMax = computed(() => {
  const values = finiteSeries.value
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
  chartData.value.map((point) => ({
    ts: point.ts,
    ms: Number.isFinite(point.ms) ? Math.min(point.ms, chartMax.value) : point.ms,
    rawMs: point.rawMs,
  })),
)

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
  return { minTs, maxTs }
})

const displayBounds = computed(() => zoomBounds.value || chartBounds.value)

const anomalyThresholds = computed(() => {
  const values = finiteSeries.value
    .map((point) => point.rawMs)
    .filter((ms): ms is number => Number.isFinite(ms))
    .sort((a, b) => a - b)
  if (!values.length) return { elevatedMs: 800, criticalMs: 1400 }
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

const visibleSeries = computed(() =>
  plottedSeriesData.value.filter((point) =>
    point.ts >= displayBounds.value.minTs && point.ts <= displayBounds.value.maxTs,
  ),
)

const plotData = computed<[number[], Array<number | null>]>(() => {
  const xValues: number[] = []
  const yValues: Array<number | null> = []

  for (const point of visibleSeries.value) {
    xValues.push(point.ts)
    yValues.push(Number.isFinite(point.ms) ? point.ms : null)
  }

  return [xValues, yValues]
})

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

const statusMarkers = computed<StatusMarker[]>(() => {
  const rows = props.checks
    .map((row) => ({
      ts: row.checked_at ? Date.parse(row.checked_at) : Number.NaN,
      status: String(row.status || '').trim().toLowerCase(),
    }))
    .filter((row) => row.status === 'up' || row.status === 'down')
    .filter((row) => Number.isFinite(row.ts))
    .filter((row) => row.ts >= displayBounds.value.minTs && row.ts <= displayBounds.value.maxTs)
    .sort((a, b) => a.ts - b.ts)

  if (rows.length < 2) return []

  const transitions: StatusMarker[] = []
  const settledRunMinPoints = 2
  const settledRunMinDurationMs = 45_000

  let runStatus = rows[0].status
  let runStart = 0

  const isSettledRun = (startIndex: number, endIndex: number) => {
    const points = endIndex - startIndex + 1
    if (points >= settledRunMinPoints) return true
    const startTs = rows[startIndex]?.ts ?? Number.NaN
    const endTs = rows[endIndex]?.ts ?? Number.NaN
    return Number.isFinite(startTs) && Number.isFinite(endTs) && endTs - startTs >= settledRunMinDurationMs
  }

  for (let index = 1; index <= rows.length; index++) {
    const nextStatus = index < rows.length ? rows[index].status : ''
    if (nextStatus === runStatus) continue

    const runEnd = index - 1
    if (runStart > 0 && isSettledRun(runStart, runEnd)) {
      transitions.push({
        ts: rows[runStart].ts,
        type: runStatus === 'down' ? 'down' : 'recovered',
      })
    }

    runStatus = nextStatus
    runStart = index
  }

  return transitions
})

const hoveredPoint = computed(() => {
  if (hoveredIndex.value === null) return null
  return visibleSeries.value[hoveredIndex.value] || null
})

const hoveredLabel = computed(() => {
  if (!hoveredPoint.value) return ''
  return new Date(hoveredPoint.value.ts).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })
})

const hoverTooltipPlacement = computed<'above' | 'below'>(() => {
  const frame = plotFrame.value
  if (!frame) return 'above'
  const preferredGapPx = 10
  const estimatedTooltipHeightPx = 56
  return hoverY.value - frame.top < estimatedTooltipHeightPx + preferredGapPx ? 'below' : 'above'
})

const hoverTooltipStyle = computed(() => ({
  left: `${tooltipX.value + 12}px`,
  top: `${tooltipY.value}px`,
}))

function resolveCssVar(name: string, fallback: string): string {
  if (typeof window === 'undefined') return fallback
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return value || fallback
}

function formatAxisTime(ts: number): string {
  const date = new Date(ts)
  if (range.value === '7d' || range.value === '30d') {
    return date.toLocaleString([], { month: 'short', day: 'numeric' })
  }
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function yTickFormat(value: number): string {
  const raw = Number(value)
  if (!Number.isFinite(raw)) return ''
  const formatted =
    raw >= 100
      ? String(Math.round(raw))
      : raw >= 10
        ? String(Math.round(raw * 10) / 10).replace(/\.0$/, '')
        : String(Math.round(raw * 100) / 100).replace(/(\.\d*[1-9])0+$|\.0+$/, '$1')
  return `${formatted} ms`
}

function setPlotFrameFromInstance(instance: uPlot) {
  plotFrame.value = {
    left: instance.bbox.left,
    top: instance.bbox.top,
    width: instance.bbox.width,
    height: instance.bbox.height,
  }
}

function markerLeftStyle(ts: number): string {
  const frame = plotFrame.value
  if (!frame) return '0px'
  const minTs = displayBounds.value.minTs
  const maxTs = displayBounds.value.maxTs
  if (maxTs <= minTs) return `${frame.left}px`
  const ratio = (ts - minTs) / (maxTs - minTs)
  const clamped = Math.max(0, Math.min(1, ratio))
  return `${frame.left + clamped * frame.width}px`
}

function anomalyBandStyle(zone: AnomalyZone) {
  const frame = plotFrame.value
  if (!frame) {
    return { left: '0px', width: '0px', top: '0px', height: '0px' }
  }
  const minTs = displayBounds.value.minTs
  const maxTs = displayBounds.value.maxTs
  if (maxTs <= minTs) {
    return { left: `${frame.left}px`, width: '0px', top: `${frame.top}px`, height: `${frame.height}px` }
  }
  const leftRatio = Math.max(0, Math.min(1, (zone.startTs - minTs) / (maxTs - minTs)))
  const rightRatio = Math.max(0, Math.min(1, (zone.endTs - minTs) / (maxTs - minTs)))
  return {
    left: `${frame.left + leftRatio * frame.width}px`,
    width: `${Math.max(0, rightRatio - leftRatio) * frame.width}px`,
    top: `${frame.top}px`,
    height: `${frame.height}px`,
  }
}

function resetZoom() {
  zoomBounds.value = null
  emit('update:zoom-window', null)
}

function destroyPlot() {
  plot.value?.destroy()
  plot.value = null
  plotFrame.value = null
  hoveredIndex.value = null
  hoverY.value = 0
  tooltipX.value = 0
  tooltipY.value = 0
}

function buildPlot() {
  const host = chartHost.value
  if (!host || !hasRenderableSeries.value) {
    destroyPlot()
    return
  }

  const width = Math.max(280, Math.floor(host.clientWidth))
  const height = Math.max(240, Math.floor(host.clientHeight || 280))

  const blue = resolveCssVar('--chart-2', '#4da2ff')
  const blueFill = 'rgba(77, 162, 255, 0.14)'
  const grid = 'rgba(255, 255, 255, 0.045)'
  const axisLine = 'rgba(255, 255, 255, 0.08)'
  const label = 'rgb(154, 167, 184)'

  destroyPlot()

  plot.value = new uPlot(
    {
      width,
      height,
      pxAlign: 1,
      legend: { show: false },
      cursor: {
        drag: {
          x: true,
          y: false,
          setScale: false,
        },
        focus: {
          prox: 1000,
        },
        hover: {
          prox: 32,
        },
        points: {
          show: () => document.createElement('div'),
          one: true,
          size: 8,
          width: 2,
          stroke: blue,
          fill: '#ffffff',
        },
      },
      select: {
        show: true,
        fill: 'rgba(16, 185, 129, 0.12)',
        stroke: 'rgba(16, 185, 129, 0.35)',
      },
      scales: {
        x: {
          time: true,
          range: [displayBounds.value.minTs, displayBounds.value.maxTs],
        },
        y: {
          range: [0, chartMax.value],
        },
      },
      axes: [
        {
          stroke: label,
          grid: { stroke: grid, width: 1 },
          ticks: { stroke: axisLine, size: 4 },
          border: { stroke: axisLine, width: 1 },
          space: 80,
          values: (_u, splits) => splits.map((value) => formatAxisTime(Number(value))),
          font: '500 12px ui-sans-serif, system-ui, sans-serif',
        },
        {
          stroke: label,
          grid: { stroke: grid, width: 1 },
          ticks: { stroke: axisLine, size: 4 },
          border: { stroke: axisLine, width: 1 },
          values: (_u, splits) => splits.map((value) => yTickFormat(Number(value))),
          splits: chartYTicks.value,
          size: 56,
          font: '500 12px ui-sans-serif, system-ui, sans-serif',
        },
      ],
      series: [
        {},
        {
          label: t('chart.responseMs'),
          stroke: blue,
          width: 2,
          fill: blueFill,
          spanGaps: false,
          points: {
            show: false,
          },
        },
      ],
      hooks: {
        ready: [
          (instance) => {
            setPlotFrameFromInstance(instance)
          },
        ],
        setSize: [
          (instance) => {
            setPlotFrameFromInstance(instance)
          },
        ],
        setCursor: [
          (instance) => {
            const idx = instance.cursor.idx
            if (idx == null || idx < 0 || idx >= visibleSeries.value.length) {
              hoveredIndex.value = null
              return
            }
            hoveredIndex.value = idx
            const point = visibleSeries.value[idx]
            hoverX.value = instance.cursor.left
            hoverY.value = instance.cursor.top
            const cursorEvent = instance.cursor.event as MouseEvent | undefined
            const panelRect = chartPanel.value?.getBoundingClientRect()
            if (cursorEvent && panelRect) {
              tooltipX.value = cursorEvent.clientX - panelRect.left
              tooltipY.value = cursorEvent.clientY - panelRect.top
            } else {
              tooltipX.value = hoverX.value
              tooltipY.value = hoverY.value
            }
          },
        ],
        setSelect: [
          (instance) => {
            const selection = instance.select
            if (!selection || selection.width < 8) return
            const from = instance.posToVal(selection.left, 'x')
            const to = instance.posToVal(selection.left + selection.width, 'x')
            if (!Number.isFinite(from) || !Number.isFinite(to) || to <= from) return
            zoomBounds.value = { minTs: from, maxTs: to }
            emit('update:zoom-window', { from, to })
            instance.setSelect({ left: 0, top: 0, width: 0, height: 0 }, false)
          },
        ],
      },
    },
    plotData.value,
    host,
  )
}

function scheduleBuild() {
  if (rafID) {
    cancelAnimationFrame(rafID)
  }
  rafID = requestAnimationFrame(() => {
    rafID = 0
    buildPlot()
  })
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

watch(
  [plotData, chartMax, chartYTicks, displayBounds, () => props.monitorType],
  async () => {
    await nextTick()
    scheduleBuild()
  },
  { deep: true },
)

onMounted(async () => {
  await nextTick()
  resizeObserver = new ResizeObserver(() => {
    scheduleBuild()
  })
  if (chartHost.value) {
    resizeObserver.observe(chartHost.value)
  }
  scheduleBuild()
})

onUnmounted(() => {
  if (rafID) {
    cancelAnimationFrame(rafID)
    rafID = 0
  }
  resizeObserver?.disconnect()
  resizeObserver = null
  destroyPlot()
})
</script>

<template>
  <Card class="chart-shell pt-0">
    <CardHeader class="flex items-center gap-2 space-y-0 border-b py-4 sm:flex-row">
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
    <CardContent class="px-2 pt-0 pb-3 sm:px-4 sm:pt-0">
      <div class="mb-2 flex items-center gap-4 text-xs font-medium text-[rgb(154_167_184)]">
        <div class="flex items-center gap-2 text-[rgb(154_167_184)]">
          <Activity class="size-3.5 text-[var(--chart-2)]" />
          <span>{{ t('chart.responseMs') }}</span>
        </div>
        <div class="ml-auto text-[11px] font-medium text-[rgb(140_154_171)]">{{ rangeSummary }}</div>
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

      <div ref="chartPanel" class="chart-panel relative h-[320px] w-full overflow-hidden rounded-md border border-border p-1.5">
        <div
          v-for="zone in anomalyZones"
          :key="`${zone.severity}:${zone.startTs}:${zone.endTs}`"
          class="pointer-events-none absolute z-0 rounded-md"
          :class="zone.severity === 'critical' ? 'chart-anomaly-band chart-anomaly-band--critical' : 'chart-anomaly-band chart-anomaly-band--elevated'"
          :style="anomalyBandStyle(zone)"
        />
        <div
          v-for="marker in statusMarkers"
          :key="`${marker.type}:${marker.ts}`"
          class="pointer-events-none absolute z-10 w-px border-l border-dashed opacity-80"
          :class="marker.type === 'down' ? 'border-rose-400/80' : 'border-emerald-400/80'"
          :style="{ left: markerLeftStyle(marker.ts), top: `${plotFrame?.top ?? 0}px`, height: `${plotFrame?.height ?? 0}px` }"
        />
        <div
          v-if="hoveredPoint && plotFrame"
          class="pointer-events-none absolute z-30 rounded-md border border-border bg-card/95 px-2 py-1 text-xs shadow-sm"
          :class="hoverTooltipPlacement === 'above' ? '-translate-y-[calc(100%+12px)]' : 'translate-y-[20px]'"
          :style="hoverTooltipStyle"
        >
          <div class="font-medium text-foreground">
            {{ hoveredPoint.rawMs }}ms
            <span v-if="hoveredPoint.rawMs > chartMax" class="text-[10px] text-muted-foreground">(clipped)</span>
          </div>
          <div class="text-muted-foreground">{{ hoveredLabel }}</div>
        </div>

        <div v-if="!chartData.length" class="flex h-full items-center justify-center text-xs text-muted-foreground">
          {{ t('monitorDetail.noChecksYet') }}
        </div>
        <div
          v-else
          ref="chartHost"
          class="uplot-host h-full w-full"
        />
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

.uplot-host {
  position: relative;
}

.chart-panel :deep(.uplot) {
  background: transparent;
  font-family: inherit;
}

.chart-panel :deep(.u-over) {
  background: transparent;
}

.chart-panel :deep(.u-select) {
  border: 1px solid rgb(16 185 129 / 0.34);
  background: rgb(16 185 129 / 0.1);
}

.chart-panel :deep(.u-cursor-pt) {
  box-shadow:
    0 0 0 2px rgb(77 162 255 / 0.16),
    0 0 8px rgb(77 162 255 / 0.2);
  z-index: 25;
}

.chart-panel :deep(.u-axis),
.chart-panel :deep(.u-axis > *),
.chart-panel :deep(.u-axis text) {
  color: rgb(154 167 184) !important;
  fill: rgb(154 167 184) !important;
  opacity: 1 !important;
  font-weight: 500;
}

.chart-panel :deep(.u-grid line) {
  stroke: rgb(255 255 255 / 0.045);
}

.chart-panel :deep(.u-axis line),
.chart-panel :deep(.u-axis path) {
  stroke: rgb(255 255 255 / 0.06);
}

.chart-anomaly-band--elevated {
  background: linear-gradient(180deg, rgb(251 191 36 / 0.08), rgb(251 191 36 / 0.02));
}

.chart-anomaly-band--critical {
  background: linear-gradient(180deg, color-mix(in oklab, var(--destructive) 12%, transparent), color-mix(in oklab, var(--destructive) 3%, transparent));
}
</style>
