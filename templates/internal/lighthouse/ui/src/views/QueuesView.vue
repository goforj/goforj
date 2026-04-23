<template>
  <div><section class="grid gap-6">
      <Card class="card-texture">
        <CardHeader class="mb-0">
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <ListChecks class="h-4 w-4 text-muted-foreground" />
              Job Queues
            </CardTitle>
          </template>
          <template #description>
            <CardDescription>Inspect queue health and manage jobs across supported drivers.</CardDescription>
          </template>
          <template #action>
            <div class="flex flex-wrap items-center justify-end gap-2">
              <div class="flex items-center gap-2 text-xs text-muted-foreground">
                <span class="text-[10px] uppercase tracking-[0.2em] text-muted">Refresh</span>
                <Select v-model="refreshIntervalModel">
                  <SelectTrigger class="w-[5.5rem]">
                    <SelectValue placeholder="Refresh" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="option in refreshOptions" :key="option" :value="String(option)">
                    {{ option }}s
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Button variant="outline" @click="toggleRefresh">
                <component :is="autoRefresh ? Pause : Play" class="mr-1 h-3.5 w-3.5" />
                {{ autoRefresh ? "Pause refresh" : "Start refresh" }}
              </Button>
              <RefreshButton :refreshing="refreshingQueues" :on-click="refreshQueues" />
            </div>
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 flex flex-wrap items-center gap-3 text-xs">
            <span v-if="status" class="text-xs text-muted">{{ status }}</span>
          </div>
          <div class="max-h-[60vh] overflow-auto rounded-xl border border-border/60">
            <table class="w-full text-xs">
              <thead class="bg-muted/40 text-muted">
                <tr>
                  <th class="px-4 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <ListOrdered class="h-3.5 w-3.5" />
                      Queue
                    </span>
                  </th>
                  <th class="px-3 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <CircleDot class="h-3.5 w-3.5" />
                      Pending
                    </span>
                  </th>
                  <th class="px-3 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Activity class="h-3.5 w-3.5" />
                      Active
                    </span>
                  </th>
                  <th class="px-3 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <CalendarClock class="h-3.5 w-3.5" />
                      Scheduled
                    </span>
                  </th>
                  <th class="px-3 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <RotateCw class="h-3.5 w-3.5" />
                      Retry
                    </span>
                  </th>
                  <th class="px-3 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Archive class="h-3.5 w-3.5" />
                      Archived
                    </span>
                  </th>
                  <th class="px-3 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <CheckCircle2 class="h-3.5 w-3.5" />
                      Processed
                    </span>
                  </th>
                  <th class="px-3 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <XCircle class="h-3.5 w-3.5" />
                      Failed
                    </span>
                  </th>
                  <th class="px-3 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <PauseCircle class="h-3.5 w-3.5" />
                      Paused
                    </span>
                  </th>
                  <th class="px-3 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <SlidersHorizontal class="h-3.5 w-3.5" />
                      Actions
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="queues.length === 0" class="border-t border-border/60">
                  <td colspan="10" class="px-4 py-2 text-muted">No queues found.</td>
                </tr>
                <tr
                  v-for="queue in queues"
                  :key="queue.name"
                  class="group border-t border-border/60 cursor-pointer"
                  :class="[
                    queue.name === selectedQueue ? 'bg-muted/40' : '',
                    queue.paused ? 'opacity-60' : '',
                  ]"
                  @click="selectQueue(queue.name)"
                >
                  <td class="px-4 py-2 text-foreground">{{ queue.name }}</td>
                  <td class="px-3 py-2 text-muted">{{ queue.pending }}</td>
                  <td class="px-3 py-2 text-muted">{{ queue.active }}</td>
                  <td class="px-3 py-2 text-muted">{{ queue.scheduled }}</td>
                  <td class="px-3 py-2 text-muted">{{ queue.retry }}</td>
                  <td class="px-3 py-2 text-muted">{{ queue.archived }}</td>
                  <td class="px-3 py-2 text-muted">{{ queue.processed }}</td>
                  <td class="px-3 py-2 text-muted">{{ queue.failed }}</td>
                  <td class="px-3 py-2 text-muted">{{ queue.paused ? "yes" : "no" }}</td>
                  <td class="px-3 py-2 text-left">
                    <div class="flex flex-wrap items-center gap-3">
                      <Button
                        variant="outline"
                        size="icon-xs"
                        class="rounded-full border-destructive/60 text-destructive hover:bg-destructive/10 hover:text-destructive"
                        @click.stop="togglePause(queue)"
                      >
                        <component :is="queue.paused ? Play : Pause" class="h-3 w-3" />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon-xs"
                        class="rounded-full border-destructive/60 text-destructive hover:bg-destructive/10 hover:text-destructive"
                        @click.stop="clearQueue(queue)"
                      >
                        <Trash2 class="h-3 w-3" />
                      </Button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="mt-4 rounded-xl border border-border/60 bg-card/60 p-4">
            <div class="mb-3 flex flex-wrap items-start justify-between gap-3 border-b border-border/50 pb-3">
              <div>
                <p class="text-xl font-semibold text-foreground">Throughput</p>
                <p class="text-sm text-muted-foreground">Processed vs failed jobs per bucket · {{ resolutionLabel }}</p>
              </div>
              <div class="flex flex-wrap items-center gap-2">
                <div class="flex flex-wrap items-center gap-2 text-xs">
                  <span
                    v-for="entry in queueWindowTotals"
                    :key="entry.name"
                    class="inline-flex items-center gap-2 whitespace-nowrap rounded-full border border-border/60 bg-muted/30 px-2.5 py-1"
                  >
                    <span class="font-medium text-foreground">{{ entry.name }}</span>
                    <span class="text-blue-300">P:{{ formatJobCount(entry.processed) }}</span>
                    <span class="text-muted-foreground">/</span>
                    <span class="text-red-300">F:{{ formatJobCount(entry.failed) }}</span>
                  </span>
                </div>
                <div class="flex items-center gap-2">
                <button
                  v-for="option in resolutions"
                  :key="option.value"
                  class="rounded-full border border-border/60 px-3 py-1 text-sm transition"
                  :class="option.value === resolution ? 'bg-background text-foreground' : 'hover:border-border'"
                  @click="setResolution(option.value)"
                >
                  {{ option.label }}
                </button>
                </div>
              </div>
            </div>
            <div class="mt-4">
              <div v-if="history.length === 0" class="text-xs text-muted">No history yet.</div>
              <div v-else class="space-y-3">
                <div v-if="insufficientSamples" class="text-sm text-muted-foreground">
                  Collecting samples... throughput needs at least 2 timeline points.
                </div>
                <div
                  ref="chartRef"
                  class="relative rounded-xl border border-border/50 bg-[rgba(7,10,16,0.78)] px-3 py-4 pl-11 shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]"
                  @mousemove="onChartMove"
                  @mouseleave="clearHover"
                >
                  <svg viewBox="0 0 100 40" preserveAspectRatio="none" class="block h-32 w-full">
                    <defs>
                      <linearGradient id="processed-fill" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stop-color="rgba(96, 165, 250, 0.32)" />
                        <stop offset="100%" stop-color="rgba(96, 165, 250, 0.02)" />
                      </linearGradient>
                      <linearGradient id="failed-fill" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stop-color="rgba(239, 118, 122, 0.24)" />
                        <stop offset="100%" stop-color="rgba(239, 118, 122, 0.02)" />
                      </linearGradient>
                    </defs>
                    <line
                      v-for="x in chartGridX"
                      :key="`grid-x-${x}`"
                      :x1="x"
                      y1="2"
                      :x2="x"
                      y2="38"
                      stroke="rgba(255, 255, 255, 0.06)"
                      stroke-width="0.2"
                    />
                    <line
                      v-for="y in chartGridY"
                      :key="`grid-y-${y}`"
                      x1="0"
                      :y1="y"
                      x2="100"
                      :y2="y"
                      stroke="rgba(255, 255, 255, 0.06)"
                      stroke-width="0.2"
                    />
                    <path
                      v-for="series in chartLineSeriesWithPath"
                      :key="`area-${series.id}`"
                      :d="series.areaPath"
                      :fill="series.fillColor"
                    />
                    <path
                      v-for="series in chartLineSeriesWithPath"
                      :key="series.id"
                      :d="series.path"
                      fill="none"
                      :stroke="series.color"
                      stroke-width="1.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      :stroke-dasharray="series.kind === 'failed' ? '4 3' : undefined"
                      vector-effect="non-scaling-stroke"
                      shape-rendering="geometricPrecision"
                    />
                  </svg>
                  <div
                    v-if="hoverPoint"
                    class="pointer-events-none absolute inset-y-0"
                    :style="{ left: `${hoverLeft}%` }"
                  >
                    <div class="h-full w-px bg-border"></div>
                  </div>
                  <div
                    v-if="hoverPoint"
                    class="pointer-events-none absolute top-0 min-w-[140px] -translate-y-[110%] rounded-lg border border-border/60 bg-popover px-3 py-2 text-[10px] text-popover-foreground shadow-md"
                    :style="{ left: `calc(${tooltipLeft}% - 24px)` }"
                  >
                    <div class="text-[9px] uppercase tracking-[0.2em] text-muted">{{ hoverLabel }}</div>
                    <div v-if="hoverSeriesRows.length === 0" class="mt-1 text-muted">No activity</div>
                    <div
                      v-for="row in hoverSeriesRows"
                      :key="row.id"
                      class="mt-1 flex items-center justify-between gap-2"
                    >
                      <span :style="{ color: row.color }">{{ row.label }}</span>
                      <span>{{ formatJobCount(row.value) }}</span>
                    </div>
                  </div>
                  <div class="absolute left-0 top-0 flex h-full flex-col justify-between text-[10px] text-muted">
                  <span>{{ formatJobCount(chartMax) }}</span>
                    <span>0</span>
                  </div>
                  <div class="absolute -left-1 top-1/2 -translate-y-1/2 -rotate-90 text-[10px] uppercase tracking-[0.3em] text-muted">
                    Jobs
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-3 text-[10px] text-muted">
                  <span
                    v-for="series in chartLineSeriesWithPath"
                    :key="`legend-${series.id}`"
                    class="flex items-center gap-2"
                  >
                    <span class="h-2 w-2 rounded-full" :style="{ backgroundColor: series.color }"></span>
                    {{ series.label }}
                  </span>
                  <span class="text-[10px] uppercase tracking-[0.2em] text-muted">count</span>
                </div>
                <div class="flex items-center justify-between text-[10px] text-muted">
                  <span v-for="tick in timeLegendTicks" :key="tick.ts">{{ tick.label }}</span>
                </div>
                <div class="text-right text-[10px] text-muted">{{ chartMetaLabel }}</div>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle>Jobs in selected queue.</CardTitle>
          </template>
          <template #description>
            <CardDescription>Filter by state and manage individual jobs.</CardDescription>
          </template>
          <template #action>
            <RefreshButton :disabled="!selectedQueue" :refreshing="refreshingJobs" :on-click="refreshJobs" />
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 grid gap-4 text-xs sm:grid-cols-2">
            <FormField label="State">
              <Select v-model="selectedState">
                <SelectTrigger class="w-full">
                  <SelectValue placeholder="State" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="pending">Pending</SelectItem>
                  <SelectItem value="active">Active</SelectItem>
                  <SelectItem value="scheduled">Scheduled</SelectItem>
                  <SelectItem value="retry">Retry</SelectItem>
                  <SelectItem value="archived">Archived</SelectItem>
                  <SelectItem value="completed">Completed</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
            <FormField label="Search">
              <Input v-model="jobQuery" placeholder="Search jobs..." />
            </FormField>
          </div>
          <div class="max-h-[60vh] overflow-auto rounded-xl border border-border/60">
            <table class="w-full text-xs">
              <thead class="bg-muted/40 text-muted">
                <tr>
                  <th class="px-4 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Fingerprint class="h-3.5 w-3.5" />
                      ID
                    </span>
                  </th>
                  <th class="px-4 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Tag class="h-3.5 w-3.5" />
                      Type
                    </span>
                  </th>
                  <th class="px-4 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <FileText class="h-3.5 w-3.5" />
                      Payload
                    </span>
                  </th>
                  <th class="px-4 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Clock class="h-3.5 w-3.5" />
                      Next
                    </span>
                  </th>
                  <th class="px-2 py-2 text-left">
                    <span class="inline-flex items-center gap-1">
                      <SlidersHorizontal class="h-3.5 w-3.5" />
                      Actions
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredJobs.length === 0" class="border-t border-border/60">
                  <td colspan="5" class="px-4 py-2 text-muted">No jobs found.</td>
                </tr>
                <tr v-for="job in filteredJobs" :key="job.id" class="group border-t border-border/60">
                  <td class="px-4 py-2 text-muted">{{ job.id }}</td>
                  <td class="px-4 py-2 text-foreground">{{ job.type }}</td>
                  <td class="px-4 py-2 text-muted">{{ job.payload }}</td>
                  <td class="px-4 py-2 text-muted">{{ job.next_process_at || job.completed_at || "-" }}</td>
                  <td class="px-2 py-2 text-left">
                    <div class="flex flex-wrap items-center gap-3">
                      <Button
                        variant="outline"
                        size="icon-xs"
                        class="rounded-full"
                        @click="retryJob(job)"
                      >
                        <RotateCw class="h-3 w-3" />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon-xs"
                        class="rounded-full border-destructive/60 text-destructive hover:bg-destructive/10 hover:text-destructive"
                        @click="cancelJob(job)"
                      >
                        <XCircle class="h-3 w-3" />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon-xs"
                        class="rounded-full border-destructive/60 text-destructive hover:bg-destructive/10 hover:text-destructive"
                        @click="deleteJob(job)"
                      >
                        <Trash2 class="h-3 w-3" />
                      </Button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useLighthouseStore } from "../stores/lighthouse";
import {
  Activity,
  Archive,
  CalendarClock,
  CheckCircle2,
  CircleDot,
  Clock,
  FileText,
  Fingerprint,
  ListChecks,
  ListOrdered,
  Pause,
  PauseCircle,
  Play,
  RotateCw,
  SlidersHorizontal,
  Tag,
  Trash2,
  XCircle,
} from "lucide-vue-next";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/input/Input.vue";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../components/ui/select";
import RefreshButton from "../components/ui/button/RefreshButton.vue";

type QueueSnapshot = {
  name: string;
  pending: number;
  active: number;
  scheduled: number;
  retry: number;
  archived: number;
  processed: number;
  failed: number;
  paused: boolean;
};

type JobSnapshot = {
  id: string;
  type: string;
  payload: string;
  next_process_at?: string;
  completed_at?: string;
};

type HistoryPoint = {
  date: string;
  processed: number;
  failed: number;
  ts?: number;
  synthetic?: boolean;
};

const store = useLighthouseStore();
const { state, sendCommand } = store;
const route = useRoute();
const router = useRouter();
const target = ref("");
const queues = ref<QueueSnapshot[]>([]);
const jobs = ref<JobSnapshot[]>([]);
const history = ref<HistoryPoint[]>([]);
const historyByQueue = ref<Record<string, HistoryPoint[]>>({});
const selectedQueue = ref("");
const selectedState = ref("pending");
const jobQuery = ref("");
const status = ref("");
const refreshingQueues = ref(false);
const refreshingJobs = ref(false);
const resolution = ref<"15m" | "hour" | "day" | "week">("hour");
let refreshTimer: number | null = null;
const autoRefresh = ref(true);
const refreshInterval = ref(5);
const refreshOptions = [5, 10, 30, 60];
const chartRef = ref<HTMLElement | null>(null);
const hoverIndex = ref<number | null>(null);
let historyRequestSeq = 0;
let historyRefreshInFlight = false;
let historyRefreshQueued = false;
let focusRefreshTimer: number | null = null;
const metricsDebugLogging = true;

const resolutions = [
  { value: "15m" as const, label: "15m" },
  { value: "hour" as const, label: "1h" },
  { value: "day" as const, label: "1d" },
  { value: "week" as const, label: "1w" },
];

const refreshIntervalModel = computed({
  get: () => String(refreshInterval.value),
  set: (value: string) => {
    refreshInterval.value = Number(value);
  },
});

const isQueueAgent = (agent: { source: string; capabilities: string[] }) =>
  agent.capabilities.includes("queue") ||
  agent.capabilities.includes("jobs") ||
  agent.source === "jobs";

const queueAgents = computed(() =>
  state.agents.filter(isQueueAgent)
);
const resolutionLabel = computed(() => {
  switch (resolution.value) {
    case "15m":
      return "last 15 minutes";
    case "hour":
      return "last hour";
    case "week":
      return "last 7 days";
    default:
      return "last day";
  }
});

const filteredJobs = computed(() => {
  const needle = jobQuery.value.trim().toLowerCase();
  if (!needle) return jobs.value;
  return jobs.value.filter((job) => job.type.toLowerCase().includes(needle) || job.id.toLowerCase().includes(needle));
});

const resolutionWindowMs = computed(() => {
  switch (resolution.value) {
    case "15m":
      return 15 * 60 * 1000;
    case "hour":
      return 60 * 60 * 1000;
    case "week":
      return 7 * 24 * 60 * 60 * 1000;
    default:
      return 24 * 60 * 60 * 1000;
  }
});

const bucketCount = computed(() => {
  switch (resolution.value) {
    case "15m":
      return 15;
    case "hour":
      return 60;
    case "week":
      return 56;
    default:
      return 48;
  }
});

const bucketDurationMs = computed(() => {
  switch (resolution.value) {
    case "15m":
      return 60 * 1000; // 1m
    case "hour":
      return 60 * 1000; // 1m
    case "week":
      return 3 * 60 * 60 * 1000; // 3h
    default:
      return 30 * 60 * 1000; // 30m (1d view)
  }
});

const normalizePointTs = (point: HistoryPoint) => {
  const raw = point.ts ?? Date.parse(point.date);
  return raw < 1_000_000_000_000 ? raw * 1000 : raw;
};

const normalizeHistorySeries = (points: HistoryPoint[]) => {
  if (points.length === 0) return points;
  const sorted = [...points].sort((a, b) => (a.ts || 0) - (b.ts || 0));

  let comparisons = 0;
  let decreases = 0;
  for (let i = 1; i < sorted.length; i += 1) {
    comparisons += 1;
    const prev = sorted[i - 1];
    const curr = sorted[i];
    if ((curr.processed || 0) < (prev.processed || 0) || (curr.failed || 0) < (prev.failed || 0)) {
      decreases += 1;
    }
  }
  const decreaseRatio = comparisons > 0 ? decreases / comparisons : 0;

  // Some drivers expose bucket counters for long windows while others expose
  // cumulative totals. Normalize to cumulative so chart math is consistent.
  if (decreaseRatio > 0.25) {
    let runningProcessed = 0;
    let runningFailed = 0;
    return sorted.map((point) => {
      runningProcessed += Math.max(0, point.processed || 0);
      runningFailed += Math.max(0, point.failed || 0);
      return {
        ...point,
        processed: runningProcessed,
        failed: runningFailed,
      };
    });
  }

  // Handle occasional counter resets while preserving monotonic totals.
  let processedOffset = 0;
  let failedOffset = 0;
  let lastProcessed = Math.max(0, sorted[0].processed || 0);
  let lastFailed = Math.max(0, sorted[0].failed || 0);
  const out: HistoryPoint[] = [
    {
      ...sorted[0],
      processed: lastProcessed,
      failed: lastFailed,
    },
  ];
  for (let i = 1; i < sorted.length; i += 1) {
    const point = sorted[i];
    const rawProcessed = Math.max(0, point.processed || 0);
    const rawFailed = Math.max(0, point.failed || 0);

    if (rawProcessed+processedOffset < lastProcessed) {
      processedOffset += lastProcessed - (rawProcessed + processedOffset);
    }
    if (rawFailed+failedOffset < lastFailed) {
      failedOffset += lastFailed - (rawFailed + failedOffset);
    }

    lastProcessed = rawProcessed + processedOffset;
    lastFailed = rawFailed + failedOffset;
    out.push({
      ...point,
      processed: lastProcessed,
      failed: lastFailed,
    });
  }
  return out;
};

const chartPointsRaw = computed(() => {
  const start = chartWindowStart.value;
  const points = [...history.value]
    .map((point) => ({
      ...point,
      ts: normalizePointTs(point),
    }))
    .sort((a, b) => (a.ts || 0) - (b.ts || 0));
  const inWindow = points.filter((point) => (point.ts || 0) >= start);
  if (inWindow.length === 0) return [];
  const anchor = [...points].reverse().find((point) => (point.ts || 0) < start);
  if (!anchor) return inWindow;
  return [anchor, ...inWindow];
});
const insufficientSamples = computed(() => chartPointsRaw.value.length < 2);

const chartWindowEnd = computed(() => {
  const bucket = Math.max(1, bucketDurationMs.value);
  return Math.floor(Date.now() / bucket) * bucket;
});
const chartWindowStart = computed(() => chartWindowEnd.value - bucketCount.value * bucketDurationMs.value);

const perQueueBucketSeries = computed(() => {
  const byQueue: Record<string, Array<{ ts: number; processed: number; failed: number }>> = {};
  const start = chartWindowStart.value;
  const end = chartWindowEnd.value;
  const count = Math.max(bucketCount.value, 1);
  const bucketSize = Math.max(1, bucketDurationMs.value);
  const names = Object.keys(historyByQueue.value).sort((a, b) => a.localeCompare(b));

  for (const name of names) {
    const rawPoints = Array.isArray(historyByQueue.value[name]) ? historyByQueue.value[name] : [];
    const raw = normalizeHistorySeries(
      rawPoints
      .map((point) => ({
        ...point,
        ts: normalizePointTs(point),
        processed: Math.max(0, point.processed || 0),
        failed: Math.max(0, point.failed || 0),
      }))
      .filter((point) => (point.ts || 0) > 0)
      .sort((a, b) => (a.ts || 0) - (b.ts || 0))
    );

    const buckets = Array.from({ length: count }, (_, index) => ({
      ts: start + (index + 1) * bucketSize,
      processed: 0,
      failed: 0,
    }));
    if (raw.length < 2) {
      byQueue[name] = buckets;
      continue;
    }

    const distributeDelta = (
      startTs: number,
      endTs: number,
      processedDelta: number,
      failedDelta: number
    ) => {
      const span = Math.max(1, endTs - startTs);
      if (processedDelta <= 0 && failedDelta <= 0) {
        return;
      }
      const clippedStart = Math.max(start, startTs);
      const clippedEnd = Math.min(end, endTs);
      if (clippedEnd <= clippedStart) {
        return;
      }
      let idx = Math.max(0, Math.min(count - 1, Math.floor((clippedStart - start) / bucketSize)));
      while (idx < count) {
        const bucketStart = start + idx * bucketSize;
        const bucketEnd = bucketStart + bucketSize;
        const overlapStart = Math.max(bucketStart, clippedStart);
        const overlapEnd = Math.min(bucketEnd, clippedEnd);
        if (overlapEnd > overlapStart) {
          const ratio = (overlapEnd - overlapStart) / span;
          buckets[idx].processed += processedDelta * ratio;
          buckets[idx].failed += failedDelta * ratio;
        }
        if (bucketEnd >= clippedEnd) {
          break;
        }
        idx += 1;
      }
    };

    for (let i = 1; i < raw.length; i += 1) {
      const prev = raw[i - 1];
      const curr = raw[i];
      const prevTs = prev.ts || 0;
      const currTs = curr.ts || 0;
      if (prevTs <= 0 || currTs <= 0 || currTs <= prevTs) {
        continue;
      }
      if (currTs < start || prevTs > end) {
        continue;
      }
      const processedDelta = Math.max(0, (curr.processed || 0) - (prev.processed || 0));
      const failedDelta = Math.max(0, (curr.failed || 0) - (prev.failed || 0));
      distributeDelta(prevTs, currTs, processedDelta, failedDelta);
    }
    byQueue[name] = buckets;
  }
  return byQueue;
});

const chartSeriesColors = [
  "#60a5fa",
  "#22d3ee",
  "#34d399",
  "#fbbf24",
  "#a78bfa",
  "#f472b6",
];

const chartLineSeries = computed(() => {
  const entries: Array<{
    id: string;
    queue: string;
    kind: "processed" | "failed";
    label: string;
    color: string;
    points: Array<{ ts: number; value: number }>;
    path: string;
  }> = [];
  const names = Object.keys(perQueueBucketSeries.value).sort((a, b) => a.localeCompare(b));
  for (let i = 0; i < names.length; i += 1) {
    const name = names[i];
    const buckets = perQueueBucketSeries.value[name] || [];
    const base = chartSeriesColors[i % chartSeriesColors.length];
    const processedPoints = buckets.map((point) => ({ ts: point.ts, value: point.processed }));
    const failedPoints = buckets.map((point) => ({ ts: point.ts, value: point.failed }));
    const processedMax = processedPoints.reduce((max, point) => Math.max(max, point.value), 0);
    const failedMax = failedPoints.reduce((max, point) => Math.max(max, point.value), 0);
    if (processedMax > 0) {
      entries.push({
        id: `${name}:processed`,
        queue: name,
        kind: "processed",
        label: `${name} processed`,
        color: base,
        points: processedPoints,
        path: "",
      });
    }
    if (failedMax > 0) {
      entries.push({
        id: `${name}:failed`,
        queue: name,
        kind: "failed",
        label: `${name} failed`,
        color: "#fca5a5",
        points: failedPoints,
        path: "",
      });
    }
  }
  return entries;
});

const chartMax = computed(() => {
  const values = chartLineSeries.value.flatMap((series) => series.points.map((point) => point.value));
  return Math.max(1, ...values);
});

const buildCoords = (points: Array<{ ts: number; value: number }>) => {
  if (points.length === 0) return [];
  const lastIndex = Math.max(points.length - 1, 1);
  return points.map((point, index) => ({
    x: (index / lastIndex) * 100,
    y: 38 - (point.value / chartMax.value) * 34,
  }));
};

const smoothPath = (coords: Array<{ x: number; y: number }>) => {
  if (coords.length === 0) return "";
  if (coords.length === 1) return `M ${coords[0].x.toFixed(2)} ${coords[0].y.toFixed(2)}`;
  let path = `M ${coords[0].x.toFixed(2)} ${coords[0].y.toFixed(2)}`;
  for (let i = 1; i < coords.length - 1; i += 1) {
    const xc = (coords[i].x + coords[i + 1].x) / 2;
    const yc = (coords[i].y + coords[i + 1].y) / 2;
    path += ` Q ${coords[i].x.toFixed(2)} ${coords[i].y.toFixed(2)}, ${xc.toFixed(2)} ${yc.toFixed(2)}`;
  }
  const last = coords.length - 1;
  path += ` Q ${coords[last - 1].x.toFixed(2)} ${coords[last - 1].y.toFixed(2)}, ${coords[last].x.toFixed(2)} ${coords[last].y.toFixed(2)}`;
  return path;
};

const chartLineSeriesWithPath = computed(() =>
  chartLineSeries.value.map((series) => {
    const coords = buildCoords(series.points);
    const line = smoothPath(coords);
    const area =
      coords.length === 0
        ? ""
        : `${line} L ${coords[coords.length - 1].x.toFixed(2)} 38.00 L ${coords[0].x.toFixed(2)} 38.00 Z`;
    return {
      ...series,
      path: line,
      areaPath: area,
      fillColor: series.kind === "failed" ? "rgba(252, 165, 165, 0.08)" : "rgba(96, 165, 250, 0.12)",
    };
  })
);
const chartGridX = [0, 12.5, 25, 37.5, 50, 62.5, 75, 87.5, 100];
const chartGridY = [4, 10, 16, 22, 28, 34, 38];
const hoverPoint = computed(() => {
  const idx = hoverIndex.value;
  if (idx === null) return null;
  const firstSeries = chartLineSeriesWithPath.value[0];
  if (!firstSeries || !firstSeries.points[idx]) return null;
  return firstSeries.points[idx];
});
const hoverSeriesRows = computed(() => {
  const idx = hoverIndex.value;
  if (idx === null) return [];
  return chartLineSeriesWithPath.value
    .map((series) => ({
      id: series.id,
      label: series.label,
      color: series.color,
      value: series.points[idx]?.value ?? 0,
    }))
    .filter((row) => row.value > 0);
});
const formatJobCount = (value: number) => Math.max(0, Math.round(value)).toString();
const hoverLeft = computed(() => {
  const count = Math.max(bucketCount.value, 1);
  if (count <= 1) return 0;
  const idx = hoverIndex.value ?? 0;
  return Math.min(100, Math.max(0, (idx / (count - 1)) * 100));
});
const tooltipLeft = computed(() => {
  const left = hoverLeft.value;
  if (left < 5) return 5;
  if (left > 70) return 70;
  return left;
});
const hoverLabel = computed(() => {
  const point = hoverPoint.value;
  if (!point) return "";
  return new Date(point.ts).toLocaleString();
});

const formatLegendTime = (ts: number) => {
  const date = new Date(ts);
  switch (resolution.value) {
    case "week":
      return date.toLocaleDateString([], { month: "short", day: "numeric" });
    case "day":
      return date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
    default:
      return date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  }
};

const timeLegendTicks = computed(() => {
  const start = chartWindowStart.value;
  const end = chartWindowEnd.value;
  const mid = start + (end - start) / 2;
  return [
    { ts: start, label: formatLegendTime(start) },
    { ts: mid, label: formatLegendTime(mid) },
    { ts: end, label: formatLegendTime(end) },
  ];
});

const queueWindowTotals = computed(() => {
  return Object.entries(perQueueBucketSeries.value)
    .map(([name, buckets]) => ({
      name,
      processed: buckets.reduce((sum, bucket) => sum + Math.max(0, bucket.processed || 0), 0),
      failed: buckets.reduce((sum, bucket) => sum + Math.max(0, bucket.failed || 0), 0),
    }))
    .sort((a, b) => a.name.localeCompare(b.name));
});

const chartMetaLabel = computed(() => {
  const pointsCount = chartLineSeriesWithPath.value[0]?.points.length ?? 0;
  const start = new Date(chartWindowStart.value);
  const end = new Date(chartWindowEnd.value);
  const fmt = (d: Date) =>
    d.toLocaleTimeString([], {
      hour: "numeric",
      minute: "2-digit",
    });
  if (resolution.value === "week") {
    const dateFmt = (d: Date) => d.toLocaleDateString([], { month: "short", day: "numeric" });
    return `${pointsCount} points · ${dateFmt(start)} - ${dateFmt(end)}`;
  }
  if (resolution.value === "day") {
    return `${pointsCount} points · ${start.toLocaleDateString([], { month: "short", day: "numeric" })}, ${fmt(start)} - ${end.toLocaleDateString([], { month: "short", day: "numeric" })}, ${fmt(end)}`;
  }
  return `${pointsCount} points · ${fmt(start)} - ${fmt(end)}`;
});

const onChartMove = (event: MouseEvent) => {
  const el = chartRef.value;
  if (!el) return;
  const count = chartLineSeriesWithPath.value[0]?.points.length ?? 0;
  if (count === 0) return;
  const rect = el.getBoundingClientRect();
  const x = Math.min(Math.max(event.clientX - rect.left, 0), rect.width);
  const idx = count === 1 ? 0 : Math.round((x / rect.width) * (count - 1));
  hoverIndex.value = idx;
};

const clearHover = () => {
  hoverIndex.value = null;
};

const setResolution = (value: "15m" | "hour" | "day" | "week") => {
  resolution.value = value;
  refreshHistory();
};

const metricsWindow = computed(() => (resolution.value === "15m" ? "hour" : resolution.value));

const withRefreshing = async (flag: typeof refreshingQueues, action: () => Promise<void>) => {
  const started = Date.now();
  flag.value = true;
  try {
    await action();
  } finally {
    const elapsed = Date.now() - started;
    const remaining = Math.max(0, 500 - elapsed);
    window.setTimeout(() => {
      flag.value = false;
    }, remaining);
  }
};

const refreshQueues = async () => {
  await withRefreshing(refreshingQueues, async () => {
    status.value = "";
    if (!target.value && queueAgents.value.length > 0) {
      target.value = queueAgents.value[0].source;
    }
    if (!target.value) {
      status.value = "Select an agent.";
      return;
    }
    const result = await sendCommand(target.value, "queue:queues", {});
    if ((result as any)?.error) {
      status.value = `queue:queues failed on ${target.value}: ${(result as any).error}`;
      return;
    }
    if (result?.data) {
      const payload = typeof result.data === "string" ? JSON.parse(result.data) : result.data;
      queues.value = Array.isArray(payload?.queues) ? payload.queues : [];
      if (queues.value.length === 0) {
        status.value = `No queues reported by ${target.value}.`;
      }
      if (!selectedQueue.value && queues.value.length > 0) {
        const defaultQueue = queues.value.find((entry) => entry.name === "default");
        selectedQueue.value = defaultQueue?.name || queues.value[0].name;
        refreshJobs();
      }
      refreshHistory();
    }
  });
};

const refreshJobs = async () => {
  await withRefreshing(refreshingJobs, async () => {
    if (!target.value || !selectedQueue.value) return;
    const result = await sendCommand(target.value, "queue:jobs", {
      queue: selectedQueue.value,
      state: selectedState.value,
      page: 1,
      page_size: 50,
    });
    if (result?.data) {
      const payload = typeof result.data === "string" ? JSON.parse(result.data) : result.data;
      jobs.value = (payload.jobs || []).map((job: JobSnapshot) => ({
        ...job,
        payload: (job.payload || "").slice(0, 120),
      }));
    }
  });
};

const syntheticHistoryPoint = () => {
  return {
    processed: queues.value.reduce((sum, queue) => sum + Math.max(0, queue.processed || 0), 0),
    failed: queues.value.reduce((sum, queue) => sum + Math.max(0, queue.failed || 0), 0),
  };
};

const hasHistoryData = () => {
  if (history.value.length > 1) return true;
  return Object.values(historyByQueue.value).some((points) => Array.isArray(points) && points.length > 1);
};

const refreshHistory = async () => {
  if (historyRefreshInFlight) {
    historyRefreshQueued = true;
    return;
  }
  historyRefreshInFlight = true;
  const requestSeq = ++historyRequestSeq;
  try {
    const commitHistory = (points: HistoryPoint[]) => {
      if (requestSeq !== historyRequestSeq) return false;
      history.value = points;
      if (metricsDebugLogging) {
        void nextTick(() => {
          const series = chartLineSeriesWithPath.value[0]?.points ?? [];
          console.groupCollapsed(
            "[queues-metrics] history committed",
            "queue=__all",
            `window=${resolution.value}`,
            `history_points=${points.length}`,
            `chart_points=${series.length}`
          );
          console.log("history points (tail)", points.slice(-8));
          console.log("chart bucket counts (tail)", series.slice(-8));
          console.groupEnd();
        });
      }
      return true;
    };
    if (!target.value) return;
    const queueNames = queues.value.map((entry) => entry.name);
    if (queueNames.length === 0) {
      if (hasHistoryData()) {
        return;
      }
      historyByQueue.value = {};
      const fallback = syntheticHistoryPoint();
      if (fallback) {
        const now = Date.now();
        commitHistory([
          {
            date: new Date(now).toISOString(),
            ts: now,
            processed: Math.max(0, fallback.processed || 0),
            failed: Math.max(0, fallback.failed || 0),
            synthetic: true,
          },
        ]);
      }
      return;
    }

    const allResult = (await sendCommand(target.value, "queue:metrics:all", {
      queues: queueNames,
      window: metricsWindow.value,
    })) as any;
    if (requestSeq !== historyRequestSeq) {
      return;
    }
    if (allResult?.error) {
      status.value = `queue:metrics:all failed on ${target.value}: ${allResult.error}`;
      return;
    }
    if (!allResult?.data) {
      status.value = `queue:metrics:all returned no data on ${target.value}`;
      return;
    }

    const now = Date.now();
    const start = now - resolutionWindowMs.value;
    const payload = allResult?.data
      ? typeof allResult.data === "string"
        ? JSON.parse(allResult.data)
        : allResult.data
      : {};
    const pointsByQueue =
      payload && payload.points_by_queue && typeof payload.points_by_queue === "object"
        ? payload.points_by_queue
        : {};
    if (metricsDebugLogging) {
      const perQueueCounts = Object.fromEntries(
        Object.entries(pointsByQueue).map(([name, points]) => [name, Array.isArray(points) ? points.length : 0])
      );
      console.log("[queues-metrics] raw points_by_queue counts", {
        queue: "__all",
        window: resolution.value,
        perQueueCounts,
      });
    }
    const normalizedByQueue = new Map<string, HistoryPoint[]>();
    let totalRawPoints = 0;
    const timestamps = new Set<number>();

    for (const queueName of queueNames) {
      const points = ((pointsByQueue[queueName] || []) as HistoryPoint[])
        .map((point: HistoryPoint) => ({
          ...point,
          ts: normalizePointTs(point),
          processed: Math.max(0, point.processed || 0),
          failed: Math.max(0, point.failed || 0),
        }))
        .filter((point: HistoryPoint) => (point.ts || 0) > 0)
        .sort((a: HistoryPoint, b: HistoryPoint) => (a.ts || 0) - (b.ts || 0));
      const normalized = normalizeHistorySeries(points);
      totalRawPoints += normalized.length;
      normalizedByQueue.set(queueName, normalized);
      for (const point of normalized) {
        const ts = point.ts || 0;
        if (ts >= start && ts <= now) timestamps.add(ts);
      }
    }
    if (totalRawPoints === 0 && hasHistoryData()) {
      return;
    }
    if (totalRawPoints === 0) {
      status.value = `No timeline points returned by ${target.value} for ${queueNames.join(", ")}.`;
    }
    historyByQueue.value = Object.fromEntries(
      queueNames.map((queueName) => [queueName, normalizedByQueue.get(queueName) || []])
    ) as Record<string, HistoryPoint[]>;

    if (timestamps.size > 0) {
      const orderedTs = [...timestamps].sort((a, b) => a - b);
      const points: HistoryPoint[] = orderedTs.map((ts) => {
        let processed = 0;
        let failed = 0;
        for (const queueName of queueNames) {
          const series = normalizedByQueue.get(queueName) || [];
          let latestProcessed = 0;
          let latestFailed = 0;
          for (const point of series) {
            if ((point.ts || 0) > ts) break;
            latestProcessed = Math.max(0, point.processed || 0);
            latestFailed = Math.max(0, point.failed || 0);
          }
          processed += latestProcessed;
          failed += latestFailed;
        }
        return {
          ts,
          date: new Date(ts).toISOString(),
          processed,
          failed,
        };
      });
      if (points.length > 0) {
        status.value = "";
        commitHistory(points);
        return;
      }
    }

    const fallback = syntheticHistoryPoint();
    if (fallback) {
      const now = Date.now();
      commitHistory([
        {
          date: new Date(now).toISOString(),
          ts: now,
          processed: Math.max(0, fallback.processed || 0),
          failed: Math.max(0, fallback.failed || 0),
          synthetic: true,
        },
      ]);
      status.value = `No history points returned by ${target.value}; showing current counters snapshot.`;
    } else {
      commitHistory([]);
      status.value = `No queue history available from ${target.value}.`;
    }
  } finally {
    historyRefreshInFlight = false;
    if (historyRefreshQueued) {
      historyRefreshQueued = false;
      void refreshHistory();
    }
  }
};

const selectQueue = (name: string) => {
  selectedQueue.value = name;
  refreshJobs();
  refreshHistory();
};

const clearQueue = async (queue: QueueSnapshot) => {
  if (!target.value || !queue?.name) return;
  if (
    !window.confirm(
      `Clear all tasks in "${queue.name}"? This will remove pending, scheduled, retry, archived, and completed entries.`
    )
  ) {
    return;
  }
  await sendCommand(target.value, "queue:clear", {
    queue: queue.name,
  });
  refreshQueues();
  refreshJobs();
};

const togglePause = async (queue: QueueSnapshot) => {
  if (!target.value || !queue?.name) return;
  if (!queue.paused) {
    if (!window.confirm(`Pause "${queue.name}"? Jobs in this queue will stop processing.`)) {
      return;
    }
  } else {
    if (!window.confirm(`Resume "${queue.name}"? Jobs will continue processing.`)) {
      return;
    }
  }
  const cmd = queue.paused ? "queue:resume" : "queue:pause";
  await sendCommand(target.value, cmd, { queue: queue.name });
  refreshQueues();
};

const cancelJob = async (job: JobSnapshot) => {
  if (!target.value) return;
  if (!window.confirm(`Cancel job "${job.id}"?`)) {
    return;
  }
  await sendCommand(target.value, "queue:job:cancel", { id: job.id });
  refreshJobs();
};

const retryJob = async (job: JobSnapshot) => {
  if (!target.value || !selectedQueue.value) return;
  if (!window.confirm(`Retry job "${job.id}"?`)) {
    return;
  }
  await sendCommand(target.value, "queue:job:retry", {
    queue: selectedQueue.value,
    id: job.id,
  });
  refreshJobs();
};

const deleteJob = async (job: JobSnapshot) => {
  if (!target.value || !selectedQueue.value) return;
  if (!window.confirm(`Delete job "${job.id}"? This cannot be undone.`)) {
    return;
  }
  await sendCommand(target.value, "queue:job:delete", {
    queue: selectedQueue.value,
    id: job.id,
  });
  refreshJobs();
};

watch(
  () => queueAgents.value,
  (agents) => {
    if (agents.length === 0) {
      target.value = "";
      return;
    }
    if (!target.value || !agents.some((agent) => agent.source === target.value)) {
      target.value = agents[0].source;
    }
  },
  { immediate: true }
);

watch(
  () => target.value,
  () => {
    queues.value = [];
    jobs.value = [];
    history.value = [];
    selectedQueue.value = "";
    if (target.value) {
      refreshQueues();
    }
  }
);

watch(
  () => selectedState.value,
  () => {
    refreshJobs();
  }
);

const runAutoRefresh = () => {
  if (!target.value) return Promise.resolve();
  return refreshQueues().then(() => {
    if (selectedQueue.value) {
      refreshJobs();
    }
  });
};

const startAutoRefresh = () => {
  if (!autoRefresh.value) return;
  if (refreshTimer) window.clearInterval(refreshTimer);
  refreshTimer = window.setInterval(() => {
    void runAutoRefresh();
  }, Math.max(1, refreshInterval.value) * 1000);
};

const stopAutoRefresh = () => {
  if (refreshTimer) {
    window.clearInterval(refreshTimer);
    refreshTimer = null;
  }
};

const toggleRefresh = () => {
  autoRefresh.value = !autoRefresh.value;
  if (autoRefresh.value) {
    startAutoRefresh();
  } else {
    stopAutoRefresh();
  }
};

const queueViewRefreshOnFocus = () => {
  if (document.visibilityState !== "visible") {
    return;
  }
  if (focusRefreshTimer) {
    window.clearTimeout(focusRefreshTimer);
  }
  focusRefreshTimer = window.setTimeout(() => {
    focusRefreshTimer = null;
    if (!target.value) {
      return;
    }
    void refreshHistory();
  }, 120);
};

onMounted(() => {
  const queryRange = route.query.range;
  if (queryRange === "15m" || queryRange === "hour" || queryRange === "day" || queryRange === "week") {
    resolution.value = queryRange;
  }
  const queryRefresh = route.query.refresh;
  if (typeof queryRefresh === "string") {
    const parsed = Number.parseInt(queryRefresh, 10);
    if (refreshOptions.includes(parsed)) {
      refreshInterval.value = parsed;
    }
  }
  if (target.value) {
    refreshQueues();
  }
  window.addEventListener("focus", queueViewRefreshOnFocus);
  document.addEventListener("visibilitychange", queueViewRefreshOnFocus);
  startAutoRefresh();
});

watch(
  () => resolution.value,
  () => {
    if (target.value) {
      refreshHistory();
    }
    router.replace({
      query: {
        ...route.query,
        range: resolution.value,
        refresh: refreshInterval.value,
      },
    });
  }
);

watch(
  () => refreshInterval.value,
  () => {
    if (autoRefresh.value) {
      startAutoRefresh();
    }
    router.replace({
      query: {
        ...route.query,
        range: resolution.value,
        refresh: refreshInterval.value,
      },
    });
  }
);

watch(
  () => autoRefresh.value,
  (enabled) => {
    if (enabled) {
      startAutoRefresh();
    } else {
      stopAutoRefresh();
    }
  }
);

onUnmounted(() => {
  if (focusRefreshTimer) {
    window.clearTimeout(focusRefreshTimer);
    focusRefreshTimer = null;
  }
  window.removeEventListener("focus", queueViewRefreshOnFocus);
  document.removeEventListener("visibilitychange", queueViewRefreshOnFocus);
  stopAutoRefresh();
});
</script>
