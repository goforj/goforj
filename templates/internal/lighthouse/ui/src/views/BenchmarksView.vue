<template>
  <div>
    <section class="grid gap-6">
      <Card v-if="!showResultsOnly" class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <Gauge class="h-4 w-4 text-muted-foreground" />
              Benchmarks
            </CardTitle>
          </template>
          <template #description>
            <CardDescription>
              Run baseline infrastructure benchmarks with sane defaults.
            </CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="rounded-xl border border-border/60 bg-muted/15 p-3">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <p class="text-xs text-muted-foreground">
                Agent: <span class="font-medium text-foreground">{{ target || "auto-selecting..." }}</span>
              </p>
              <div class="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                <span class="rounded-md border border-border/70 px-2 py-1">duration {{ durationMS }}ms</span>
                <span class="rounded-md border border-border/70 px-2 py-1">concurrency {{ concurrency }}</span>
                <span class="rounded-md border border-border/70 px-2 py-1">payload {{ payloadSize }} bytes</span>
              </div>
            </div>
            <div class="mt-3 flex flex-col gap-2">
              <label
                v-for="suite in suites"
                :key="suite.key"
                class="inline-flex items-center gap-2 rounded-lg border border-border/60 bg-card/40 px-3 py-2 text-xs"
                :class="suite.enabled ? 'text-foreground' : 'text-muted-foreground'"
              >
                <input
                  type="checkbox"
                  class="h-3.5 w-3.5 accent-primary"
                  :disabled="!suite.enabled || running"
                  :checked="selected[suite.key]"
                  @change="setSelected(suite.key, ($event.target as HTMLInputElement).checked)"
                />
                <span class="min-w-0 truncate whitespace-nowrap">
                  <span class="font-medium">{{ suite.label }}</span>
                  <span class="mx-1 text-muted">·</span>
                  <span class="text-muted">{{ suite.description }}</span>
                </span>
                <span v-if="!suite.enabled" class="text-[10px] uppercase tracking-[0.12em] text-muted">
                  soon
                </span>
              </label>
            </div>
          </div>

          <div class="mt-3 rounded-xl border border-border/60 bg-muted/15 p-3">
            <button
              type="button"
              class="inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"
              @click="advancedOpen = !advancedOpen"
            >
              <Settings2 class="h-3.5 w-3.5" />
              Advanced settings
              <ChevronDown class="h-3.5 w-3.5 transition-transform" :class="advancedOpen ? 'rotate-180' : ''" />
            </button>
            <div v-if="advancedOpen" class="mt-3 grid gap-3 text-xs sm:grid-cols-3">
              <FormField label="Target agent">
                <Select v-model="target">
                  <option value="">Auto-select</option>
                  <option v-for="agent in benchmarkAgents" :key="agent.source" :value="agent.source">
                    {{ agent.source }}
                  </option>
                </Select>
              </FormField>
              <FormField label="Duration (ms)">
                <Input v-model.number="durationMS" type="number" min="500" step="100" :disabled="running" />
              </FormField>
              <FormField label="Concurrency">
                <Input v-model.number="concurrency" type="number" min="1" step="1" :disabled="running" />
              </FormField>
              <FormField label="Payload bytes">
                <Input v-model.number="payloadSize" type="number" min="0" step="128" :disabled="running" />
              </FormField>
              <FormField label="Queue (queue suite)">
                <Input v-model="queueName" placeholder="default" :disabled="running" />
              </FormField>
              <FormField label="Concurrency sweep">
                <Select v-model="sweepEnabled">
                  <option :value="false">Off</option>
                  <option :value="true">On</option>
                </Select>
              </FormField>
              <FormField label="Sweep max concurrency">
                <Input v-model.number="sweepMax" type="number" min="1" step="1" :disabled="running || !sweepEnabled" />
              </FormField>
            </div>
          </div>

          <div class="mt-4 flex flex-wrap items-center gap-3">
            <Button variant="default" :disabled="running || selectedCount === 0" @click="runSelected">
              <LoaderCircle v-if="running" class="mr-1 h-3.5 w-3.5 animate-spin" />
              <Gauge v-else class="mr-1 h-3.5 w-3.5" />
              {{ running ? `Running (${runProgress}/${selectedCount})` : `Run Baseline (${selectedCount})` }}
            </Button>
            <Button variant="outline" size="sm" :disabled="running" @click="selectDefaults">
              Defaults
            </Button>
            <Button variant="outline" size="sm" :disabled="running" @click="selectAllEnabled">
              Select All Available
            </Button>
            <Button variant="outline" size="sm" :disabled="running" @click="clearResults">
              Clear Results
            </Button>
            <span v-if="error" class="text-xs text-red-300">{{ error }}</span>
          </div>
        </CardContent>
      </Card>

      <Card v-if="hasBenchmarkResults || showResultsOnly" class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle>
              <span class="inline-flex items-center gap-2">
                <Gauge class="h-4 w-4 text-muted-foreground" />
                Benchmark Results
              </span>
            </CardTitle>
          </template>
          <template #description>
            <CardDescription>Baseline summary and per-suite metrics.</CardDescription>
          </template>
          <template #action>
            <div class="flex items-center gap-2">
              <Button
                v-if="canRerunLast"
                variant="outline"
                size="sm"
                :disabled="running"
                @click="rerunLast"
              >
                <RotateCcw class="mr-1 h-3.5 w-3.5" />
                Re-run
              </Button>
            <Button
              v-if="showResultsOnly"
              variant="outline"
              size="sm"
              :disabled="running"
              @click="showResultsOnly = false"
            >
              <ArrowLeft class="mr-1 h-3.5 w-3.5" />
              Back to Settings
            </Button>
            </div>
          </template>
        </CardHeader>
        <CardContent>
          <div
            v-if="!hasBenchmarkResults"
            class="rounded-xl border border-border/60 bg-muted/15 px-4 py-8 text-center"
          >
            <p class="text-sm font-medium text-foreground">No benchmark results yet.</p>
            <p class="mt-1 text-xs text-muted-foreground">
              Run a benchmark from settings to populate this view.
            </p>
            <div class="mt-4">
              <Button variant="outline" size="sm" @click="showResultsOnly = false">
                <ArrowLeft class="mr-1 h-3.5 w-3.5" />
                Back to Settings
              </Button>
            </div>
          </div>

          <template v-else>
          <div v-if="systemInfo" class="mb-3 rounded-xl border border-border/60 bg-muted/20 p-3">
            <p class="text-[10px] uppercase tracking-[0.16em] text-muted">System Baseline</p>
            <div class="mt-2 flex flex-wrap gap-1.5 text-[11px]">
              <span class="inline-flex items-center gap-1 rounded-md border border-border/60 bg-card/40 px-2 py-1">
                <Server class="h-3 w-3 text-muted-foreground" /> host <strong class="font-semibold text-foreground">{{ systemInfo.hostname || "-" }}</strong>
              </span>
              <span class="inline-flex items-center gap-1 rounded-md border border-border/60 bg-card/40 px-2 py-1">
                <Monitor class="h-3 w-3 text-muted-foreground" /> platform <strong class="font-semibold text-foreground">{{ systemInfo.platform || "-" }} {{ systemInfo.platform_version || "" }}</strong>
              </span>
              <span class="inline-flex items-center gap-1 rounded-md border border-border/60 bg-card/40 px-2 py-1">
                <Cog class="h-3 w-3 text-muted-foreground" /> kernel <strong class="font-semibold text-foreground">{{ systemInfo.kernel_version || "-" }} ({{ systemInfo.kernel_arch || "-" }})</strong>
              </span>
              <span class="inline-flex items-center gap-1 rounded-md border border-border/60 bg-card/40 px-2 py-1">
                <Clock3 class="h-3 w-3 text-muted-foreground" /> uptime <strong class="font-semibold text-foreground">{{ systemInfo.uptime_human || "-" }}</strong>
              </span>
              <span class="inline-flex items-center gap-1 rounded-md border border-border/60 bg-card/40 px-2 py-1">
                <Cpu class="h-3 w-3 text-muted-foreground" /> cpu <strong class="font-semibold text-foreground">{{ systemInfo.cpu_model || "-" }}</strong>
              </span>
              <span class="inline-flex items-center gap-1 rounded-md border border-border/60 bg-card/40 px-2 py-1">
                <Microchip class="h-3 w-3 text-muted-foreground" /> cores <strong class="font-semibold text-foreground">{{ formatInt(systemInfo.cpu_cores_logical || 0) }}L / {{ formatInt(systemInfo.cpu_cores_physical || 0) }}P</strong>
              </span>
              <span class="inline-flex items-center gap-1 rounded-md border border-border/60 bg-card/40 px-2 py-1">
                <MemoryStick class="h-3 w-3 text-muted-foreground" /> memory <strong class="font-semibold text-foreground">{{ formatBytes(systemInfo.memory_total_bytes || 0) }}</strong>
              </span>
              <span class="inline-flex items-center gap-1 rounded-md border border-border/60 bg-card/40 px-2 py-1">
                <HardDrive class="h-3 w-3 text-muted-foreground" /> disk <strong class="font-semibold text-foreground">{{ formatBytes(systemInfo.disk_total_bytes || 0) }}</strong>
              </span>
            </div>
          </div>

          <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
            <div class="rounded-md border border-border/60 bg-muted/20 p-2">
              <p class="inline-flex items-center gap-1 text-[10px] uppercase tracking-[0.16em] text-muted">
                <ListChecks class="h-3 w-3" />Suites Completed
              </p>
              <p class="mt-0.5 text-base font-semibold text-foreground">{{ completedSuites }}/{{ selectedCount }}</p>
            </div>
            <div class="rounded-md border border-border/60 bg-muted/20 p-2">
              <p class="inline-flex items-center gap-1 text-[10px] uppercase tracking-[0.16em] text-muted">
                <BarChart3 class="h-3 w-3" />Total Ops
              </p>
              <p class="mt-0.5 text-base font-semibold text-foreground">{{ formatInt(summary.totalOps) }}</p>
            </div>
            <div class="rounded-md border border-border/60 bg-muted/20 p-2">
              <p class="inline-flex items-center gap-1 text-[10px] uppercase tracking-[0.16em] text-muted">
                <Gauge class="h-3 w-3" />Aggregate Ops/sec
              </p>
              <p class="mt-0.5 text-base font-semibold text-foreground">{{ formatNumber(summary.totalOpsPerSec) }}</p>
            </div>
            <div class="rounded-md border border-border/60 bg-muted/20 p-2">
              <p class="inline-flex items-center gap-1 text-[10px] uppercase tracking-[0.16em] text-muted">
                <AlertTriangle class="h-3 w-3" />Total Errors
              </p>
              <p
                class="mt-0.5 text-base font-semibold"
                :class="summary.totalErrors > 0 ? 'text-red-300' : 'text-emerald-300'"
              >
                {{ formatInt(summary.totalErrors) }}
              </p>
            </div>
          </div>

          <div class="mt-4 overflow-auto rounded-xl border border-border/60">
            <table class="w-full text-xs">
              <thead class="border-b border-border/60 text-muted">
                <tr>
                  <th class="px-3 py-2 text-left font-medium">Suite</th>
                  <th class="px-3 py-2 text-left font-medium">Status</th>
                  <th class="px-3 py-2 text-left font-medium">KPIs</th>
                  <th class="px-3 py-2 text-left font-medium">Metrics</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="suite in selectedSuitesOrdered"
                  :key="`detail-${suite}`"
                  class="border-t border-border/60 align-top"
                  :class="[
                    currentRunningSuite === suite ? 'benchmark-row-loading' : '',
                    runErrors[suite] ? 'bg-red-500/5' : results[suite] ? 'bg-emerald-500/[0.02]' : '',
                  ]"
                >
                  <td class="px-3 py-2">
                    <p class="inline-flex items-center gap-1.5 font-semibold text-foreground">
                      <component :is="suiteIcon(suite)" class="h-3.5 w-3.5 text-muted-foreground" />
                      {{ suiteTitle(suite) }}
                    </p>
                    <p class="mt-0.5 text-[11px] text-muted">driver {{ suiteDriverLabel(results[suite], suite) }}</p>
                  </td>
                  <td class="px-3 py-2">
                    <span
                      v-if="currentRunningSuite === suite"
                      class="inline-flex items-center gap-1 rounded-md border border-amber-400/40 bg-amber-400/10 px-1.5 py-0.5 text-amber-300"
                    >
                      <LoaderCircle class="h-3.5 w-3.5 animate-spin" /> running
                    </span>
                    <span
                      v-else-if="running && selected[suite]"
                      class="inline-flex items-center rounded-md border border-border/70 bg-card/40 px-1.5 py-0.5 text-muted"
                    >queued</span>
                    <span
                      v-else-if="results[suite]"
                      class="inline-flex items-center rounded-md border border-emerald-400/40 bg-emerald-400/10 px-1.5 py-0.5 text-emerald-300"
                    >ok</span>
                    <span
                      v-else-if="runErrors[suite]"
                      class="inline-flex items-center rounded-md border border-red-500/40 bg-red-500/10 px-1.5 py-0.5 text-red-300"
                    >{{ runErrors[suite] }}</span>
                    <span
                      v-else
                      class="inline-flex items-center rounded-md border border-border/70 bg-card/40 px-1.5 py-0.5 text-muted"
                    >not run</span>
                  </td>
                  <td class="px-3 py-2">
                    <div v-if="results[suite]" class="flex flex-wrap items-center gap-1.5 text-[11px]">
                      <span
                        class="inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5"
                        :class="opsPillClass(suite, results[suite])"
                      >
                        <Gauge class="h-3 w-3 text-muted-foreground" />
                        {{ formatNumber(results[suite]?.ops_per_sec || 0) }}/s
                      </span>
                      <span
                        class="inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5"
                        :class="latencyPillClass(results[suite]!.p95_ms || 0)"
                      >
                        <Activity class="h-3 w-3 text-muted-foreground" />
                        p95 {{ formatNumber(results[suite]?.p95_ms || 0) }}ms
                      </span>
                      <span
                        class="inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5"
                        :class="(results[suite]?.errors || 0) > 0 ? 'border-red-500/40 bg-red-500/10 text-red-300' : 'border-border/60 bg-card/40'"
                      >
                        <AlertTriangle class="h-3 w-3" />
                        err {{ formatInt(results[suite]?.errors || 0) }}
                      </span>
                      <span class="inline-flex items-center gap-1 rounded-md border border-border/60 bg-card/40 px-1.5 py-0.5">
                        <Clock3 class="h-3 w-3 text-muted-foreground" />
                        {{ formatInt(results[suite]?.elapsed_ms || 0) }}ms
                      </span>
                      <span
                        class="inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5"
                        :class="gradePillClass(String((results[suite]?.extra?.performance_grade as string) || '-'))"
                      >
                        perf {{ String((results[suite]?.extra?.performance_grade as string) || "-") }}
                      </span>
                      <span
                        class="inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5"
                        :class="gradePillClass(String((results[suite]?.extra?.stability_grade as string) || '-'))"
                      >
                        stable {{ String((results[suite]?.extra?.stability_grade as string) || "-") }}
                      </span>
                    </div>
                  </td>
                  <td class="px-3 py-2">
                    <div v-if="results[suite]" class="grid grid-cols-2 gap-x-3 gap-y-1 text-[11px] lg:grid-cols-3">
                      <div
                        v-for="metric in suiteMetricRows(suite, results[suite]!)"
                        :key="`${suite}-${metric.key}`"
                        class="truncate"
                      >
                        <span class="text-muted">{{ metric.label }}:</span>
                        <span class="ml-1 text-foreground" :title="metric.tooltip || ''">{{ metric.value }}</span>
                      </div>
                    </div>
                    <details v-if="results[suite]" class="mt-1">
                      <summary class="cursor-pointer text-[11px] text-muted-foreground hover:text-foreground">
                        Raw JSON
                      </summary>
                      <pre class="mt-1 max-h-44 overflow-auto text-[11px] text-muted-foreground">{{ toPrettyJSON(results[suite]) }}</pre>
                    </details>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          </template>
        </CardContent>
      </Card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import {
  Activity,
  AlertTriangle,
  ArrowLeft,
  BarChart3,
  ChevronDown,
  Clock3,
  Cog,
  Cpu,
  Database,
  Gauge,
  Globe,
  HardDrive,
  ListChecks,
  MemoryStick,
  Microchip,
  Monitor,
  LoaderCircle,
  RotateCcw,
  Settings2,
  Server,
  Workflow,
} from "lucide-vue-next";
import { onBeforeRouteLeave, useRoute, useRouter } from "vue-router";
import { useLighthouseStore } from "../stores/lighthouse";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import Select from "../components/ui/form/Select.vue";

type SuiteKey = "cache" | "queue" | "db" | "http";
type RunConfig = {
  target: string;
  durationMS: number;
  concurrency: number;
  payloadSize: number;
  queueName: string;
  sweepEnabled: boolean;
  sweepMax: number;
  suites: SuiteKey[];
};

type BenchmarkReport = {
  suite: string;
  driver: string;
  duration_ms: number;
  concurrency: number;
  payload_size: number;
  queue?: string;
  started_at: string;
  completed_at: string;
  elapsed_ms: number;
  ops: number;
  ops_per_sec: number;
  errors: number;
  p50_ms: number;
  p95_ms: number;
  p99_ms: number;
  extra?: Record<string, any>;
};

type BenchmarkSystemInfo = {
  hostname: string;
  os: string;
  platform: string;
  platform_version: string;
  kernel_version: string;
  kernel_arch: string;
  uptime_seconds: number;
  uptime_human: string;
  cpu_cores_logical: number;
  cpu_cores_physical: number;
  cpu_model: string;
  cpu_mhz: string;
  memory_total_bytes: number;
  memory_available_bytes: number;
  swap_total_bytes: number;
  disk_total_bytes: number;
  disk_free_bytes: number;
  vm_system: string;
  vm_role: string;
  go_version: string;
};

const suites: Array<{ key: SuiteKey; label: string; description: string; enabled: boolean }> = [
  { key: "cache", label: "Cache", description: "set/get throughput and latency", enabled: true },
  { key: "queue", label: "Queue", description: "enqueue+process throughput and latency", enabled: true },
  { key: "db", label: "Database", description: "query throughput and latency", enabled: true },
  { key: "http", label: "Web/API", description: "request throughput and latency", enabled: true },
];

const { state, sendCommand } = useLighthouseStore();
const route = useRoute();
const router = useRouter();

const benchmarkAgents = computed(() => {
  const capable = state.agents.filter((agent) => agent.capabilities.includes("benchmark"));
  if (capable.length > 0) return capable;
  const likely = state.agents.filter(
    (agent) =>
      agent.source === "jobs" ||
      agent.capabilities.includes("jobs") ||
      agent.capabilities.includes("queue")
  );
  return likely.length > 0 ? likely : state.agents;
});

const target = ref(state.selectedAgent || "");
const durationMS = ref(15000);
const concurrency = ref(8);
const payloadSize = ref(512);
const queueName = ref("default");
const sweepEnabled = ref(false);
const sweepMax = ref(16);
const advancedOpen = ref(false);
const showResultsOnly = ref(false);
const running = ref(false);
const currentRunningSuite = ref<SuiteKey | null>(null);
const runProgress = ref(0);
const error = ref("");
const selected = ref<Record<SuiteKey, boolean>>({
  cache: true,
  queue: true,
  db: true,
  http: true,
});
const results = ref<Partial<Record<SuiteKey, BenchmarkReport>>>({});
const runErrors = ref<Partial<Record<SuiteKey, string>>>({});
const systemInfo = ref<BenchmarkSystemInfo | null>(null);
const lastRunConfig = ref<RunConfig | null>(null);

const selectedSuitesOrdered = computed(() => suites.filter((s) => selected.value[s.key]).map((s) => s.key));
const selectedCount = computed(() => selectedSuitesOrdered.value.length);
const completedSuites = computed(() => Object.keys(results.value).length);
const hasBenchmarkResults = computed(
  () => running.value || completedSuites.value > 0 || Object.keys(runErrors.value).length > 0,
);
const canRerunLast = computed(() => !!lastRunConfig.value && !running.value);
const summary = computed(() => {
  const reports = Object.values(results.value).filter(Boolean) as BenchmarkReport[];
  return reports.reduce(
    (acc, report) => {
      acc.totalOps += report.ops || 0;
      acc.totalOpsPerSec += report.ops_per_sec || 0;
      acc.totalErrors += report.errors || 0;
      return acc;
    },
    { totalOps: 0, totalOpsPerSec: 0, totalErrors: 0 }
  );
});

const syncStateFromRoute = () => {
  const raw = Array.isArray(route.query.view) ? route.query.view[0] : route.query.view;
  const view = String(raw || "").trim().toLowerCase();
  if (view === "results") {
    showResultsOnly.value = true;
    return;
  }
  if (view === "settings") {
    showResultsOnly.value = false;
  }
};

const syncRouteFromState = async () => {
  const next = showResultsOnly.value ? "results" : "settings";
  const currentRaw = Array.isArray(route.query.view) ? route.query.view[0] : route.query.view;
  const current = String(currentRaw || "").trim().toLowerCase();
  if (current === next) return;
  await router.replace({
    query: {
      ...route.query,
      view: next,
    },
  });
};

watch(
  benchmarkAgents,
  (agents) => {
    if (agents.length === 0) {
      return;
    }
    const preferred = agents.find((agent) => agent.source === "jobs")?.source || agents[0].source;
    const currentValid = agents.some((agent) => agent.source === target.value);
    if (!target.value || !currentValid) {
      target.value = preferred;
    }
  },
  { immediate: true }
);

watch(
  () => route.query.view,
  () => {
    syncStateFromRoute();
  },
);

watch(
  showResultsOnly,
  async () => {
    await syncRouteFromState();
  },
);

const setSelected = (suite: SuiteKey, value: boolean) => {
  selected.value = { ...selected.value, [suite]: value };
};

const selectDefaults = () => {
  selected.value = suites.reduce((acc, suite) => {
    acc[suite.key] = suite.enabled;
    return acc;
  }, {} as Record<SuiteKey, boolean>);
  durationMS.value = 15000;
  concurrency.value = 8;
  payloadSize.value = 512;
  queueName.value = "default";
  sweepEnabled.value = false;
  sweepMax.value = 16;
};

const selectAllEnabled = () => {
  selected.value = suites.reduce((acc, suite) => {
    acc[suite.key] = suite.enabled;
    return acc;
  }, {} as Record<SuiteKey, boolean>);
};

const clearResults = () => {
  results.value = {};
  runErrors.value = {};
  systemInfo.value = null;
  error.value = "";
  showResultsOnly.value = false;
};

const suiteTitle = (suite: SuiteKey) => {
  const found = suites.find((item) => item.key === suite);
  return found?.label || suite;
};

const suiteIcon = (suite: SuiteKey) => {
  switch (suite) {
    case "cache":
      return HardDrive;
    case "queue":
      return Workflow;
    case "db":
      return Database;
    case "http":
      return Globe;
    default:
      return Gauge;
  }
};

const suiteExtraMetrics = (suite: SuiteKey, report: BenchmarkReport) => {
  const extra = report.extra || {};
  const sweepRows = Array.isArray(extra.sweep) ? (extra.sweep as Array<Record<string, any>>) : [];
  if (suite === "cache") {
    const out = [
      { key: "duration", label: "Configured Duration", value: `${formatInt(report.duration_ms)}ms` },
      { key: "concurrency", label: "Concurrency", value: formatInt(report.concurrency) },
      { key: "payload", label: "Payload", value: `${formatInt(report.payload_size)} bytes` },
    ];
    if (sweepRows.length > 0) {
      out.push({ key: "sweep", label: "Sweep Points", value: formatInt(sweepRows.length) });
    }
    return out;
  }
  if (suite === "queue") {
    const out = [
      { key: "queue", label: "Queue", value: String(report.queue || "default") },
      { key: "dispatched", label: "Dispatched", value: formatInt(Number(extra.dispatched || 0)) },
      { key: "processed", label: "Processed Delta", value: formatInt(Number(extra.processed_delta || 0)) },
      { key: "failed", label: "Failed Delta", value: formatInt(Number(extra.failed_delta || 0)) },
      { key: "pending", label: "End Pending", value: formatInt(Number(extra.end_pending || 0)) },
      { key: "active", label: "End Active", value: formatInt(Number(extra.end_active || 0)) },
    ];
    if (sweepRows.length > 0) {
      out.push({ key: "sweep", label: "Sweep Points", value: formatInt(sweepRows.length) });
    }
    return out;
  }
  if (suite === "db") {
    const connectionRows = Array.isArray(extra.connections) ? (extra.connections as Array<Record<string, any>>) : [];
    const connectionDriverSummary = connectionRows
      .map((row) => `${String(row.name || "default")}(${String(row.driver || "-")})`)
      .join(" · ");
    const connectionSummary = connectionRows
      .map((row) => `${String(row.name || "default")}:${formatInt(Number(row.ops || 0))}`)
      .join(" · ");
    const out = [
      { key: "profile", label: "Profile", value: String(extra.profile || "mixed") },
      { key: "connection_count", label: "Connections", value: formatInt(Number(extra.connection_count || connectionRows.length || 0)) },
      { key: "read_ops", label: "Read Ops", value: formatInt(Number(extra.read_ops || 0)) },
      { key: "update_ops", label: "Update Ops", value: formatInt(Number(extra.update_ops || 0)) },
      { key: "write_ops", label: "Write Ops", value: formatInt(Number(extra.write_ops || 0)) },
      { key: "seed_rows", label: "Seed Rows", value: formatInt(Number(extra.seed_rows || 0)) },
    ];
    if (connectionSummary) {
      out.push({ key: "per_connection_ops", label: "Per-Connection Ops", value: connectionSummary });
    }
    if (connectionDriverSummary) {
      out.push({ key: "per_connection_driver", label: "Per-Connection Driver", value: connectionDriverSummary });
    }
    if (extra.error_classes && typeof extra.error_classes === "object") {
      out.push({ key: "error_classes", label: "Error Classes", value: formatStatusCounts(extra.error_classes) });
    }
    if (sweepRows.length > 0) {
      out.push({ key: "sweep", label: "Sweep Points", value: formatInt(sweepRows.length) });
    }
    return out;
  }
  if (suite === "http") {
    let topErrors = Array.isArray(extra.top_errors) ? (extra.top_errors as string[]) : [];
    if (topErrors.length === 0 && Number(report.errors || 0) > 0) {
      const derived: string[] = [];
      const statusMap =
        extra.status_counts && typeof extra.status_counts === "object"
          ? (extra.status_counts as Record<string, number>)
          : {};
      let non2xx = 0;
      for (const [code, rawCount] of Object.entries(statusMap)) {
        const count = Number(rawCount || 0);
        if (count <= 0) continue;
        if (!code.startsWith("2")) {
          non2xx += count;
          derived.push(`${formatInt(count)}× status_${code}`);
        }
      }
      const unknown = Math.max(0, Number(report.errors || 0) - non2xx);
      if (unknown > 0) {
        derived.push(`${formatInt(unknown)}× transport_or_unknown`);
      }
      if (derived.length === 0) {
        derived.push(`${formatInt(Number(report.errors || 0))}× unclassified_error`);
      }
      topErrors = derived;
    }
    const visibleTopErrors = topErrors.length > 0 ? topErrors.slice(0, 2).join(" · ") : "-";
    const tooltipTopErrors = topErrors.length > 0 ? topErrors.join("\n") : "";
    const out = [
      { key: "url", label: "Target URL", value: String(extra.url || "-") },
      { key: "status_counts", label: "Status Counts", value: formatStatusCounts(extra.status_counts) },
      {
        key: "error_counts",
        label: "Top Errors",
        value: visibleTopErrors,
        tooltip: tooltipTopErrors,
      },
    ];
    if (sweepRows.length > 0) {
      out.push({ key: "sweep", label: "Sweep Points", value: formatInt(sweepRows.length) });
    }
    return out;
  }
  return Object.entries(extra).map(([key, value]) => ({
    key,
    label: key.replace(/_/g, " "),
    value: typeof value === "number" ? formatNumber(value) : String(value),
  }));
};

const suiteMetricRows = (suite: SuiteKey, report: BenchmarkReport): Array<{ key: string; label: string; value: string; tooltip?: string }> => [
  { key: "ops", label: "Ops", value: formatInt(report.ops || 0) },
  { key: "latency", label: "P50/P95/P99", value: `${formatNumber(report.p50_ms || 0)} / ${formatNumber(report.p95_ms || 0)} / ${formatNumber(report.p99_ms || 0)}` },
  ...suiteExtraMetrics(suite, report),
];

const suiteDriverLabel = (report: BenchmarkReport | undefined, suite: SuiteKey) => {
  if (!report) return "-";
  const extra = report.extra || {};
  let explicit = String(
    extra.configured_driver || extra.driver || report.driver || "",
  ).trim();
  explicit = explicit.toLowerCase();
  if (suite === "db") {
    const rows = Array.isArray(extra.connections) ? (extra.connections as Array<Record<string, any>>) : [];
    const perConnection = Array.from(
      new Set(
        rows
          .map((row) => String(row.driver || "").trim())
          .filter((v) => v.length > 0 && v !== "-" && v !== "db"),
      ),
    ).sort();
    if (perConnection.length > 0) return perConnection.join(",");
  }
  if (["cache", "db", "http", "queue", "-"].includes(explicit)) return "-";
  if (explicit) return explicit;
  return "-";
};

const toPrettyJSON = (value: unknown) => JSON.stringify(value, null, 2);

const runSuite = async (suite: SuiteKey) => {
  const result = await sendCommand(target.value, "benchmark:run", {
    suite,
    duration_ms: durationMS.value,
    concurrency: concurrency.value,
    payload_size: payloadSize.value,
    queue: queueName.value,
    sweep: sweepEnabled.value,
    sweep_max: sweepMax.value,
  });
  if (!result?.ok) {
    throw new Error(result?.error || `Benchmark command failed for suite '${suite}'.`);
  }
  const payload = result?.data ? (typeof result.data === "string" ? JSON.parse(result.data) : result.data) : {};
  const report = payload.report as BenchmarkReport | undefined;
  if (!report) {
    throw new Error(`Suite '${suite}' returned no report.`);
  }
  if (payload.system && typeof payload.system === "object") {
    systemInfo.value = payload.system as BenchmarkSystemInfo;
  }
  results.value = { ...results.value, [suite]: report };
};

const preloadSystemBaseline = async () => {
  if (!target.value) return;
  const result = await sendCommand(target.value, "benchmark:system", {});
  if (!result?.ok) return;
  const payload = result?.data ? (typeof result.data === "string" ? JSON.parse(result.data) : result.data) : {};
  if (payload?.system && typeof payload.system === "object") {
    systemInfo.value = payload.system as BenchmarkSystemInfo;
  }
};

const runFromConfig = async (cfg: RunConfig) => {
  error.value = "";
  target.value = cfg.target;
  durationMS.value = cfg.durationMS;
  concurrency.value = cfg.concurrency;
  payloadSize.value = cfg.payloadSize;
  queueName.value = cfg.queueName;
  sweepEnabled.value = cfg.sweepEnabled;
  sweepMax.value = cfg.sweepMax;
  selected.value = suites.reduce((acc, suite) => {
    acc[suite.key] = cfg.suites.includes(suite.key);
    return acc;
  }, {} as Record<SuiteKey, boolean>);

  if (benchmarkAgents.value.length > 0) {
    const selectedIsCapable = benchmarkAgents.value.some((agent) => agent.source === cfg.target);
    if (!selectedIsCapable) {
      target.value = benchmarkAgents.value.find((agent) => agent.source === "jobs")?.source || benchmarkAgents.value[0].source;
    }
  }
  if (!target.value && state.agents.length > 0) {
    target.value = state.agents[0].source;
  }
  if (!target.value) {
    error.value = "Select an agent.";
    return;
  }
  if (selectedCount.value === 0) {
    error.value = "Select at least one suite.";
    return;
  }
  lastRunConfig.value = {
    target: target.value,
    durationMS: durationMS.value,
    concurrency: concurrency.value,
    payloadSize: payloadSize.value,
    queueName: queueName.value || "default",
    sweepEnabled: sweepEnabled.value,
    sweepMax: sweepMax.value,
    suites: [...selectedSuitesOrdered.value],
  };

  showResultsOnly.value = true;
  runProgress.value = 0;
  runErrors.value = {};
  results.value = {};
  systemInfo.value = null;
  running.value = true;
  currentRunningSuite.value = null;
  try {
    await preloadSystemBaseline();
    for (const suite of selectedSuitesOrdered.value) {
      currentRunningSuite.value = suite;
      runProgress.value += 1;
      try {
        await runSuite(suite);
      } catch (err: any) {
        runErrors.value = { ...runErrors.value, [suite]: err?.message || "Benchmark failed." };
      }
    }
  } finally {
    currentRunningSuite.value = null;
    running.value = false;
  }
};

const runSelected = async () => {
  const cfg: RunConfig = {
    target: target.value,
    durationMS: durationMS.value,
    concurrency: concurrency.value,
    payloadSize: payloadSize.value,
    queueName: queueName.value || "default",
    sweepEnabled: sweepEnabled.value,
    sweepMax: sweepMax.value,
    suites: [...selectedSuitesOrdered.value],
  };
  await runFromConfig(cfg);
};

const rerunLast = async () => {
  if (!lastRunConfig.value || running.value) return;
  await runFromConfig(lastRunConfig.value);
};

const beforeUnload = (event: BeforeUnloadEvent) => {
  if (!running.value) return;
  event.preventDefault();
  event.returnValue = "";
};

onMounted(() => {
  syncStateFromRoute();
  window.addEventListener("beforeunload", beforeUnload);
});

onUnmounted(() => {
  window.removeEventListener("beforeunload", beforeUnload);
});

onBeforeRouteLeave(() => {
  if (running.value) {
    return false;
  }
  return true;
});

const formatNumber = (value: number) =>
  Number(value || 0).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
const gradePillClass = (grade: string) => {
  const g = String(grade || "-").trim().toUpperCase();
  if (g === "A") return "border-emerald-400/40 bg-emerald-400/10 text-emerald-300";
  if (g === "B") return "border-sky-400/40 bg-sky-400/10 text-sky-300";
  if (g === "C") return "border-amber-400/40 bg-amber-400/10 text-amber-300";
  if (g === "D" || g === "F") return "border-red-500/40 bg-red-500/10 text-red-300";
  return "border-border/60 bg-card/40";
};
const latencyPillClass = (p95ms: number) => {
  const p95 = Number(p95ms || 0);
  if (p95 <= 1.0) return "border-emerald-400/40 bg-emerald-400/10 text-emerald-300";
  if (p95 <= 5.0) return "border-sky-400/40 bg-sky-400/10 text-sky-300";
  if (p95 <= 20.0) return "border-amber-400/40 bg-amber-400/10 text-amber-300";
  return "border-red-500/40 bg-red-500/10 text-red-300";
};
const opsPillClass = (suite: SuiteKey, report?: BenchmarkReport) => {
  const ops = Number(report?.ops_per_sec || 0);
  const goodBySuite: Record<SuiteKey, number> = {
    cache: 200000,
    queue: 3000,
    db: 8000,
    http: 4000,
  };
  const warnBySuite: Record<SuiteKey, number> = {
    cache: 80000,
    queue: 1200,
    db: 3000,
    http: 1500,
  };

  if (suite === "queue") {
    const driver = String(report?.driver || report?.extra?.driver || "").trim().toLowerCase();
    if (driver === "redis" || driver === "asynq") {
      goodBySuite.queue = 6000;
      warnBySuite.queue = 2500;
    } else if (driver === "workerpool") {
      goodBySuite.queue = 30000;
      warnBySuite.queue = 10000;
    }
  }

  if (ops >= goodBySuite[suite]) return "border-emerald-400/40 bg-emerald-400/10 text-emerald-300";
  if (ops >= warnBySuite[suite]) return "border-sky-400/40 bg-sky-400/10 text-sky-300";
  if (ops > 0) return "border-amber-400/40 bg-amber-400/10 text-amber-300";
  return "border-red-500/40 bg-red-500/10 text-red-300";
};
const formatInt = (value: number) => Math.round(Number(value || 0)).toLocaleString();
const formatStatusCounts = (raw: unknown) => {
  if (!raw || typeof raw !== "object") return "-";
  const entries = Object.entries(raw as Record<string, unknown>);
  if (entries.length === 0) return "-";
  return entries
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([code, count]) => `${code}:${formatInt(Number(count || 0))}`)
    .join(" · ");
};
const formatBytes = (value: number) => {
  const size = Number(value || 0);
  if (size <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let idx = 0;
  let n = size;
  while (n >= 1024 && idx < units.length - 1) {
    n /= 1024;
    idx += 1;
  }
  return `${n.toFixed(1)} ${units[idx]}`;
};
</script>

<style scoped>
.benchmark-row-loading {
  position: relative;
  box-shadow: inset 0 -1px 0 rgba(245, 158, 11, 0.35);
  background-image: linear-gradient(
    90deg,
    rgba(0, 0, 0, 0) 0%,
    rgba(245, 158, 11, 0.12) 35%,
    rgba(245, 158, 11, 0.45) 50%,
    rgba(245, 158, 11, 0.12) 65%,
    rgba(0, 0, 0, 0) 100%
  );
  background-repeat: no-repeat;
  background-size: 220% 2px;
  background-position: 120% 100%;
  animation: benchmark-row-progress 1.1s linear infinite;
}

@keyframes benchmark-row-progress {
  0% {
    background-position: 120% 100%;
  }
  100% {
    background-position: -120% 100%;
  }
}
</style>
