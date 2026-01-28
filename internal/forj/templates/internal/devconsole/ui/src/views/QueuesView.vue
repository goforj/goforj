<template>
  <div>
    <PageHeader label="Platform" title="Job Queues (Asynq)">
      <template #right>
        <AgentPills />
        <LivePill />
      </template>
    </PageHeader>

    <section class="mt-8 grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Queues</p>
            <CardTitle>Queue health and counts.</CardTitle>
          </template>
          <template #description>
            <CardDescription>Inspect Asynq queues and pause or clear as needed.</CardDescription>
          </template>
          <template #action>
            <RefreshButton :on-click="refreshQueues" />
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 flex flex-wrap items-center gap-3 text-xs">
            <div v-if="showAgentFilter" class="min-w-[160px]">
              <FormField label="Agent">
                <Select v-model="target" class="min-w-[160px]">
                  <option value="">Select agent</option>
                  <option v-for="agent in queueAgents" :key="agent.source" :value="agent.source">
                    {{ agent.source }}
                  </option>
                </Select>
              </FormField>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-[10px] uppercase tracking-[0.2em] text-muted">Refresh</span>
              <Select v-model.number="refreshInterval">
                <option v-for="option in refreshOptions" :key="option" :value="option">
                  {{ option }}s
                </option>
              </Select>
              <Button variant="outline" @click="toggleRefresh">
                <component :is="autoRefresh ? Pause : Play" class="mr-1 h-3.5 w-3.5" />
                {{ autoRefresh ? "Pause refresh" : "Start refresh" }}
              </Button>
            </div>
            <span v-if="status" class="text-xs text-muted">{{ status }}</span>
          </div>
          <div class="max-h-[60vh] overflow-auto rounded-xl border border-border/70">
            <table class="w-full text-xs">
              <thead class="bg-white/5 text-muted">
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
                <tr v-if="queues.length === 0" class="border-t border-border/70">
                  <td colspan="10" class="px-4 py-2 text-muted">No queues found.</td>
                </tr>
                <tr
                  v-for="queue in queues"
                  :key="queue.name"
                  class="group border-t border-border/70 cursor-pointer"
                  :class="[
                    queue.name === selectedQueue ? 'bg-white/5' : '',
                    queue.paused ? 'opacity-60' : '',
                  ]"
                  @click="selectQueue(queue.name)"
                >
                  <td class="px-4 py-2 text-white">{{ queue.name }}</td>
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
                      <button
                        class="inline-flex items-center gap-1 rounded-md border border-border/70 bg-white/5 px-2.5 py-1 text-[10px] text-muted whitespace-nowrap leading-none transition active:scale-95 active:translate-y-[0.5px]"
                        @click.stop="togglePause(queue)"
                      >
                        <component :is="queue.paused ? Play : Pause" class="h-3 w-3" />
                        {{ queue.paused ? "Resume" : "Pause" }}
                      </button>
                      <button
                        class="inline-flex items-center gap-1 rounded-md border border-border/70 bg-white/5 px-2.5 py-1 text-[10px] text-muted whitespace-nowrap leading-none transition active:scale-95 active:translate-y-[0.5px]"
                        @click.stop="clearQueue(queue)"
                      >
                        <Trash2 class="h-3 w-3" />
                        Clear
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="mt-4 rounded-xl border border-border/70 bg-white/5 p-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p class="text-[10px] uppercase tracking-[0.3em] text-muted">Throughput ({{ resolutionLabel }})</p>
                <p class="mt-1 text-xs text-white/80">Processed vs failed runs.</p>
              </div>
              <div class="flex items-center gap-2 text-[10px] uppercase tracking-[0.2em] text-muted">
                <button
                  v-for="option in resolutions"
                  :key="option.value"
                  class="rounded-full border border-border/70 px-3 py-1 transition"
                  :class="option.value === resolution ? 'bg-white/10 text-white' : 'hover:border-white/30'"
                  @click="setResolution(option.value)"
                >
                  {{ option.label }}
                </button>
              </div>
            </div>
            <div class="mt-4">
              <div v-if="history.length === 0" class="text-xs text-muted">No history yet.</div>
              <div v-else class="space-y-3">
                <div
                  ref="chartRef"
                  class="relative pl-8"
                  @mousemove="onChartMove"
                  @mouseleave="clearHover"
                >
                  <svg viewBox="0 0 100 40" preserveAspectRatio="none" class="block h-20 w-full">
                    <polyline
                      :points="processedLine"
                      fill="none"
                      stroke="rgba(115, 214, 170, 0.9)"
                      stroke-width="1.5"
                      stroke-linecap="butt"
                      stroke-linejoin="bevel"
                      vector-effect="non-scaling-stroke"
                      shape-rendering="geometricPrecision"
                    />
                    <polyline
                      :points="failedLine"
                      fill="none"
                      stroke="rgba(239, 118, 122, 0.85)"
                      stroke-width="1.5"
                      stroke-linecap="butt"
                      stroke-linejoin="bevel"
                      vector-effect="non-scaling-stroke"
                      shape-rendering="geometricPrecision"
                    />
                  </svg>
                  <div
                    v-if="hoverPoint"
                    class="pointer-events-none absolute inset-y-0"
                    :style="{ left: `${hoverLeft}%` }"
                  >
                    <div class="h-full w-px bg-white/20"></div>
                  </div>
                  <div
                    v-if="hoverPoint"
                    class="pointer-events-none absolute top-0 min-w-[140px] -translate-y-[110%] rounded-lg border border-border/70 bg-black/80 px-3 py-2 text-[10px] text-white shadow-lg"
                    :style="{ left: `calc(${tooltipLeft}% - 24px)` }"
                  >
                    <div class="text-[9px] uppercase tracking-[0.2em] text-muted">{{ hoverLabel }}</div>
                    <div class="mt-1 flex items-center justify-between gap-2">
                      <span class="text-emerald-200/90">Processed</span>
                      <span>{{ hoverPoint.processed }}</span>
                    </div>
                    <div class="flex items-center justify-between gap-2">
                      <span class="text-red-200/90">Failed</span>
                      <span>{{ hoverPoint.failed }}</span>
                    </div>
                  </div>
                  <div class="absolute left-0 top-0 flex h-full flex-col justify-between text-[10px] text-muted">
                    <span>{{ chartMax }}</span>
                    <span>0</span>
                  </div>
                  <div class="absolute -left-1 top-1/2 -translate-y-1/2 -rotate-90 text-[10px] uppercase tracking-[0.3em] text-muted">
                    Jobs
                  </div>
                </div>
                <div class="flex items-center gap-4 text-[10px] text-muted">
                  <span class="flex items-center gap-2">
                    <span class="h-2 w-2 rounded-full bg-emerald-300/80"></span> Processed
                  </span>
                  <span class="flex items-center gap-2">
                    <span class="h-2 w-2 rounded-full bg-red-300/80"></span> Failed
                  </span>
                  <span class="text-[10px] uppercase tracking-[0.2em] text-muted">per bucket</span>
                </div>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Jobs</p>
            <CardTitle>Jobs in selected queue.</CardTitle>
          </template>
          <template #description>
            <CardDescription>Filter by state and manage individual jobs.</CardDescription>
          </template>
          <template #action>
            <RefreshButton :disabled="!selectedQueue" :on-click="refreshJobs" />
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 grid gap-4 text-xs sm:grid-cols-2">
            <FormField label="State">
              <Select v-model="selectedState" @change="refreshJobs">
                <option value="pending">Pending</option>
                <option value="active">Active</option>
                <option value="scheduled">Scheduled</option>
                <option value="retry">Retry</option>
                <option value="archived">Archived</option>
                <option value="completed">Completed</option>
              </Select>
            </FormField>
            <FormField label="Search">
              <Input v-model="jobQuery" placeholder="Search jobs..." />
            </FormField>
          </div>
          <div class="max-h-[60vh] overflow-auto rounded-xl border border-border/70">
            <table class="w-full text-xs">
              <thead class="bg-white/5 text-muted">
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
                <tr v-if="filteredJobs.length === 0" class="border-t border-border/70">
                  <td colspan="5" class="px-4 py-2 text-muted">No jobs found.</td>
                </tr>
                <tr v-for="job in filteredJobs" :key="job.id" class="group border-t border-border/70">
                  <td class="px-4 py-2 text-muted">{{ job.id }}</td>
                  <td class="px-4 py-2 text-white">{{ job.type }}</td>
                  <td class="px-4 py-2 text-muted">{{ job.payload }}</td>
                  <td class="px-4 py-2 text-muted">{{ job.next_process_at || job.completed_at || "-" }}</td>
                  <td class="px-2 py-2 text-left">
                    <div class="flex flex-wrap items-center gap-3">
                      <button
                        class="inline-flex items-center gap-1 rounded-md border border-border/70 bg-white/5 px-2.5 py-1 text-[10px] text-muted whitespace-nowrap leading-none transition active:scale-95 active:translate-y-[0.5px]"
                        @click="retryJob(job)"
                      >
                        <RotateCw class="h-3 w-3" />
                        Retry
                      </button>
                      <button
                        class="inline-flex items-center gap-1 rounded-md border border-border/70 bg-white/5 px-2.5 py-1 text-[10px] text-muted whitespace-nowrap leading-none transition active:scale-95 active:translate-y-[0.5px]"
                        @click="cancelJob(job)"
                      >
                        <XCircle class="h-3 w-3" />
                        Cancel
                      </button>
                      <button
                        class="inline-flex items-center gap-1 rounded-md border border-border/70 bg-white/5 px-2.5 py-1 text-[10px] text-muted whitespace-nowrap leading-none transition active:scale-95 active:translate-y-[0.5px]"
                        @click="deleteJob(job)"
                      >
                        <Trash2 class="h-3 w-3" />
                        Delete
                      </button>
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
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useDevconsoleStore } from "../stores/devconsole";
import {
  Activity,
  Archive,
  CalendarClock,
  CheckCircle2,
  CircleDot,
  Clock,
  FileText,
  Fingerprint,
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
import AgentPills from "../components/AgentPills.vue";
import LivePill from "../components/LivePill.vue";
import PageHeader from "../components/PageHeader.vue";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import Select from "../components/ui/form/Select.vue";
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

const store = useDevconsoleStore();
const { state, sendCommand } = store;
const route = useRoute();
const router = useRouter();
const target = ref("");
const queues = ref<QueueSnapshot[]>([]);
const jobs = ref<JobSnapshot[]>([]);
const history = ref<HistoryPoint[]>([]);
const selectedQueue = ref("");
const selectedState = ref("pending");
const jobQuery = ref("");
const status = ref("");
const resolution = ref<"hour" | "day" | "week">("day");
let refreshTimer: number | null = null;
const autoRefresh = ref(true);
const refreshInterval = ref(5);
const refreshOptions = [5, 10, 30, 60];
const chartRef = ref<HTMLElement | null>(null);
const hoverIndex = ref<number | null>(null);

const resolutions = [
  { value: "hour" as const, label: "Hour" },
  { value: "day" as const, label: "Day" },
  { value: "week" as const, label: "Week" },
];

const queueAgents = computed(() =>
  state.agents.filter((agent) => agent.capabilities.includes("asynq"))
);
const showAgentFilter = computed(() => queueAgents.value.length > 1);

const resolutionLabel = computed(() => {
  switch (resolution.value) {
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
    case "hour":
      return 60;
    case "week":
      return 56;
    default:
      return 48;
  }
});

const chartPointsRaw = computed(() => {
  const now = Date.now();
  const start = now - resolutionWindowMs.value;
  return [...history.value]
    .map((point) => ({
      ...point,
      ts: (() => {
        const raw = point.ts ?? Date.parse(point.date);
        return raw < 1_000_000_000_000 ? raw * 1000 : raw;
      })(),
    }))
    .filter((point) => (point.ts || 0) >= start)
    .sort((a, b) => (a.ts || 0) - (b.ts || 0));
});

const chartWindowStart = computed(() => Date.now() - resolutionWindowMs.value);
const chartWindowEnd = computed(() => Date.now());

const chartSeries = computed(() => {
  const raw = chartPointsRaw.value;
  if (raw.length === 0) return [];
  const deltas: HistoryPoint[] = [];
  for (let i = 0; i < raw.length; i += 1) {
    const curr = raw[i];
    const prev = i > 0 ? raw[i - 1] : curr;
    deltas.push({
      ...curr,
      processed: Math.max(0, curr.processed - prev.processed),
      failed: Math.max(0, curr.failed - prev.failed),
    });
  }
  return deltas;
});

const bucketedSeries = computed(() => {
  const start = chartWindowStart.value;
  const end = chartWindowEnd.value;
  const count = Math.max(bucketCount.value, 1);
  const span = Math.max(end - start, 1);
  const bucketSize = span / count;
  const buckets = Array.from({ length: count }, (_, index) => ({
    ts: start + index * bucketSize,
    processed: 0,
    failed: 0,
  }));
  for (const point of chartSeries.value) {
    const ts = point.ts ?? start;
    if (ts < start || ts > end) continue;
    const idx = Math.min(count - 1, Math.max(0, Math.floor((ts - start) / bucketSize)));
    buckets[idx].processed += point.processed;
    buckets[idx].failed += point.failed;
  }
  return buckets;
});

const chartMax = computed(() => {
  const values = bucketedSeries.value.flatMap((point) => [point.processed, point.failed]);
  return Math.max(1, ...values);
});

const buildLine = (key: "processed" | "failed") => {
  const points = bucketedSeries.value;
  if (points.length === 0) return "";
  const lastIndex = Math.max(points.length - 1, 1);
  return points
    .map((point, index) => {
      const x = (index / lastIndex) * 100;
      const y = 38 - (point[key] / chartMax.value) * 34;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");
};

const processedLine = computed(() => buildLine("processed"));
const failedLine = computed(() => buildLine("failed"));
const hoverPoint = computed(() => {
  const idx = hoverIndex.value;
  if (idx === null) return null;
  return bucketedSeries.value[idx] ?? null;
});
const hoverLeft = computed(() => {
  const count = bucketedSeries.value.length;
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

const onChartMove = (event: MouseEvent) => {
  const el = chartRef.value;
  if (!el) return;
  const count = bucketedSeries.value.length;
  if (count === 0) return;
  const rect = el.getBoundingClientRect();
  const x = Math.min(Math.max(event.clientX - rect.left, 0), rect.width);
  const idx = count === 1 ? 0 : Math.round((x / rect.width) * (count - 1));
  hoverIndex.value = idx;
};

const clearHover = () => {
  hoverIndex.value = null;
};

const setResolution = (value: "hour" | "day" | "week") => {
  resolution.value = value;
  refreshHistory();
};

const refreshQueues = async () => {
  status.value = "";
  if (!target.value) {
    status.value = "Select an agent.";
    return;
  }
  const result = await sendCommand(target.value, "asynq:queues", {});
  if (result?.data) {
    const payload = typeof result.data === "string" ? JSON.parse(result.data) : result.data;
    queues.value = payload.queues || [];
    if (!selectedQueue.value && queues.value.length > 0) {
      selectedQueue.value = queues.value[0].name;
      refreshJobs();
    }
    if (selectedQueue.value) {
      refreshHistory();
    }
  }
};

const refreshJobs = async () => {
  if (!target.value || !selectedQueue.value) return;
  const result = await sendCommand(target.value, "asynq:jobs", {
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
};

const refreshHistory = async () => {
  if (!target.value || !selectedQueue.value) return;
  const result = await sendCommand(target.value, "queue:metrics", {
    queue: selectedQueue.value,
    window: resolution.value,
  });
  if (result?.data) {
    const payload = typeof result.data === "string" ? JSON.parse(result.data) : result.data;
    history.value = (payload.points || []).map((point: HistoryPoint) => ({
      ...point,
      ts: point.ts ?? Date.parse(point.date),
    }));
  }
};

const ensurePolling = () => {
  if (resolution.value === "week") {
    if (pollTimer) {
      window.clearInterval(pollTimer);
      pollTimer = null;
    }
    return;
  }
  if (pollTimer) return;
  pollTimer = window.setInterval(() => {
    if (target.value) {
      refreshQueues();
    }
  }, 15000);
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
  await sendCommand(target.value, "asynq:queue:clear", {
    queue: queue.name,
  });
  refreshQueues();
  refreshJobs();
};

const togglePause = async (queue: QueueSnapshot) => {
  if (!target.value || !queue?.name) return;
  if (
    !queue.paused &&
    !window.confirm(`Pause "${queue.name}"? Jobs in this queue will stop processing.`)
  ) {
    return;
  }
  const cmd = queue.paused ? "asynq:queue:resume" : "asynq:queue:pause";
  await sendCommand(target.value, cmd, { queue: queue.name });
  refreshQueues();
};

const cancelJob = async (job: JobSnapshot) => {
  if (!target.value) return;
  await sendCommand(target.value, "asynq:job:cancel", { id: job.id });
  refreshJobs();
};

const retryJob = async (job: JobSnapshot) => {
  if (!target.value || !selectedQueue.value) return;
  await sendCommand(target.value, "asynq:job:retry", {
    queue: selectedQueue.value,
    id: job.id,
  });
  refreshJobs();
};

const deleteJob = async (job: JobSnapshot) => {
  if (!target.value || !selectedQueue.value) return;
  await sendCommand(target.value, "asynq:job:delete", {
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

const runAutoRefresh = () => {
  if (!target.value) return;
  refreshQueues();
  if (selectedQueue.value) {
    refreshJobs();
    refreshHistory();
  }
};

const startAutoRefresh = () => {
  if (!autoRefresh.value) return;
  if (refreshTimer) window.clearInterval(refreshTimer);
  refreshTimer = window.setInterval(runAutoRefresh, Math.max(1, refreshInterval.value) * 1000);
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

onMounted(() => {
  const queryRange = route.query.range;
  if (queryRange === "hour" || queryRange === "day" || queryRange === "week") {
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
  startAutoRefresh();
});

watch(
  () => resolution.value,
  () => {
    history.value = [];
    if (target.value && selectedQueue.value) {
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
  stopAutoRefresh();
});
</script>
