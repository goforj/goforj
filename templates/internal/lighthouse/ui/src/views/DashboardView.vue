<template>
  <div>
    <section class="mb-6">
      <Card class="card-texture dashboard-card">
        <CardHeader class="mb-3">
          <template #title>
            <div class="flex items-center gap-2">
              <Package class="h-4 w-4 text-sky-300" />
              <CardTitle>About</CardTitle>
            </div>
          </template>
        </CardHeader>
        <CardContent>
          <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
            <div class="rounded-md border border-border/60 bg-muted/20 px-2.5 py-2">
              <div class="flex items-start gap-2.5">
                <Package class="mt-0.5 h-4 w-4 text-sky-300" />
                <div>
                  <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Project</p>
                  <p class="mt-0.5 text-sm font-semibold text-foreground">{{ about.environment.app_name || "Unknown" }}</p>
                  <p class="mt-0.5 text-[11px] text-muted break-all">{{ about.environment.module || "No module configured" }}</p>
                </div>
              </div>
            </div>
            <div class="rounded-md border border-border/60 bg-muted/20 px-2.5 py-2">
              <div class="flex items-start gap-2.5">
                <Cpu class="mt-0.5 h-4 w-4 text-emerald-300" />
                <div>
                  <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Environment</p>
                  <p class="mt-0.5 text-sm font-semibold text-foreground">{{ about.environment.environment || "Unknown" }}</p>
                  <p class="mt-0.5 text-[11px] text-muted">Go {{ about.environment.go_version || "unknown" }}</p>
                </div>
              </div>
            </div>
            <div class="rounded-md border border-border/60 bg-muted/20 px-2.5 py-2">
              <div class="flex items-start gap-2.5">
                <Blocks class="mt-0.5 h-4 w-4 text-violet-300" />
                <div>
                  <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Framework</p>
                  <p class="mt-0.5 text-sm font-semibold text-foreground">{{ about.environment.goforj_version || "Unknown" }}</p>
                  <p class="mt-0.5 text-[11px] text-muted">Wire {{ about.build.wire_generated || "unknown" }}</p>
                </div>
              </div>
            </div>
            <div class="rounded-md border border-border/60 bg-muted/20 px-2.5 py-2">
              <div class="flex items-start gap-2.5">
                <Globe class="mt-0.5 h-4 w-4 text-amber-300" />
                <div>
                  <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Runtime</p>
                  <p class="mt-0.5 text-sm font-semibold text-foreground">{{ state.agents.length }} connected agent{{ state.agents.length === 1 ? "" : "s" }}</p>
                  <p class="mt-0.5 text-[11px] text-muted">{{ about.network.app_url || "No app URL configured" }}</p>
                </div>
              </div>
            </div>
          </div>
          <div class="mt-2.5">
            <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Enabled Components</p>
            <div class="mt-1 flex flex-wrap gap-1">
              <span
                v-for="component in about.build.components"
                :key="component"
                class="rounded-full border border-border/70 bg-muted/30 px-2 py-0.5 text-[10px] text-foreground"
              >
                {{ component }}
              </span>
              <span
                v-if="about.build.components.length === 0"
                class="text-xs text-muted"
              >
                No components reported.
              </span>
            </div>
          </div>
          <div class="mt-2.5 columns-1 [column-gap:0.5rem] lg:columns-2 xl:columns-3 2xl:columns-4">
            <section
              v-for="section in primitiveSections"
              :key="section.title"
              class="mb-2 inline-block w-full break-inside-avoid overflow-hidden rounded-md border border-border/60 bg-muted/10"
            >
              <div class="flex items-center justify-between gap-2.5 border-b border-border/60 px-2 py-1.5">
                <div class="flex items-center gap-2">
                  <component :is="sectionIcon(section.title)" class="h-3.5 w-3.5 text-sky-300" />
                  <h3 class="text-[12px] font-semibold text-foreground">{{ section.title }}</h3>
                </div>
                <p class="text-[10px] text-muted">
                  {{ section.connections.length }} instance{{ section.connections.length === 1 ? "" : "s" }}
                </p>
              </div>
              <div class="divide-y divide-border/50">
                <div
                  v-for="connection in section.connections"
                  :key="section.title + connection.name"
                  class="bg-background/35 px-2 py-1.5 text-xs"
                >
                  <div class="flex items-start gap-2">
                    <div class="min-w-0 shrink-0 basis-28">
                      <div class="flex flex-wrap items-center gap-1">
                        <span class="text-[12px] font-semibold text-foreground">{{ connection.name }}</span>
                        <span
                          v-if="connection.is_default"
                          class="rounded-full border border-sky-400/30 bg-sky-400/10 px-1 py-0.5 text-[8px] uppercase tracking-[0.15em] text-sky-200"
                        >
                          Default
                        </span>
                      </div>
                    </div>
                    <p class="min-w-0 flex-1 text-[10px] leading-4 text-muted">
                      {{ formatConnectionDetails(connection.details) }}
                    </p>
                  </div>
                </div>
              </div>
            </section>
          </div>
        </CardContent>
      </Card>
    </section>

    <section class="grid gap-4 lg:grid-cols-3">
      <Card class="card-texture dashboard-card dashboard-card-hero">
        <CardHeader class="mb-3">
          <template #title>
            <div class="flex items-center gap-3">
              <div class="dashboard-stat-icon dashboard-stat-icon-jobs">
                <Server class="h-4 w-4" />
              </div>
              <div>
                <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Jobs</p>
                <CardTitle>Queue totals</CardTitle>
              </div>
            </div>
          </template>
        </CardHeader>
        <CardContent class="flex h-full flex-col">
          <div class="flex flex-wrap gap-2">
            <span class="dashboard-metric-chip dashboard-metric-chip-pending">
              <span class="dashboard-metric-dot dashboard-metric-dot-pending" />
              Pending {{ jobTotals.pending }}
            </span>
            <span class="dashboard-metric-chip dashboard-metric-chip-active">
              <span class="dashboard-metric-dot dashboard-metric-dot-active" />
              Active {{ jobTotals.active }}
            </span>
            <span class="dashboard-metric-chip dashboard-metric-chip-scheduled">
              <span class="dashboard-metric-dot dashboard-metric-dot-scheduled" />
              Scheduled {{ jobTotals.scheduled }}
            </span>
            <span class="dashboard-metric-chip dashboard-metric-chip-retry">
              <span class="dashboard-metric-dot dashboard-metric-dot-retry" />
              Retry {{ jobTotals.retry }}
            </span>
          </div>
          <div class="mt-3 flex flex-wrap gap-2">
            <span class="dashboard-metric-chip dashboard-metric-chip-processed">
              <CheckCircle2 class="h-3.5 w-3.5 text-emerald-300" />
              Processed {{ jobTotals.processed }}
            </span>
            <span class="dashboard-metric-chip dashboard-metric-chip-failed">
              <CircleX class="h-3.5 w-3.5 text-rose-300" />
              Failed {{ jobTotals.failed }}
            </span>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture dashboard-card dashboard-card-hero">
        <CardHeader class="mb-3">
          <template #title>
            <div class="flex items-center justify-between gap-3">
              <div class="flex items-center gap-3">
                <div class="dashboard-stat-icon dashboard-stat-icon-routes">
                  <Route class="h-4 w-4" />
                </div>
                <div>
                  <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Routes</p>
                  <CardTitle>API surface</CardTitle>
                </div>
              </div>
              <span class="dashboard-count-pill">{{ totalRoutes }} endpoints</span>
            </div>
          </template>
        </CardHeader>
        <CardContent class="flex h-full flex-col">
          <div class="flex flex-wrap gap-2">
            <span class="dashboard-inline-chip">Handlers {{ routeSummary.handlers }}</span>
            <span class="dashboard-inline-chip">Dynamic {{ routeSummary.dynamic }}</span>
          </div>
          <div class="mt-3 flex flex-wrap gap-2">
            <span class="dashboard-inline-chip" v-for="item in routeSummary.methodBreakdown" :key="item.label">
              <span :class="methodDotClass(item.label)" class="dashboard-metric-dot" />
              {{ item.label }} {{ item.count }}
            </span>
          </div>
          <div v-if="routeSummary.prefixBreakdown.length > 0" class="mt-3 flex flex-wrap gap-2">
            <span class="dashboard-inline-chip" v-for="item in routeSummary.prefixBreakdown" :key="item.label">
              {{ item.label }} {{ item.count }}
            </span>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture dashboard-card dashboard-card-hero">
        <CardHeader class="mb-3">
          <template #title>
            <div class="flex items-center justify-between gap-3">
              <div class="flex items-center gap-3">
                <div class="dashboard-stat-icon dashboard-stat-icon-schedules">
                  <CalendarClock class="h-4 w-4" />
                </div>
                <div>
                  <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Schedules</p>
                  <CardTitle>Upcoming jobs</CardTitle>
                </div>
              </div>
              <span class="dashboard-count-pill">{{ totalSchedules }} jobs</span>
            </div>
          </template>
        </CardHeader>
        <CardContent class="flex h-full flex-col">
          <div class="mt-3 rounded-xl border border-border/65 bg-background/55 px-3 py-2.5">
            <div class="flex items-center justify-between gap-2">
              <p class="dashboard-stat-label">Next 5</p>
              <span class="text-[10px] text-muted">{{ scheduleSummary.upcoming.length }} shown</span>
            </div>
            <div v-if="scheduleSummary.upcoming.length > 0" class="mt-2 space-y-1">
              <div
                v-for="schedule in scheduleSummary.upcoming"
                :key="schedule.id"
                class="flex items-start justify-between gap-3 text-xs"
              >
                <p
                  class="min-w-0 flex-1 truncate text-[11px] font-medium text-foreground"
                  :title="schedule.name"
                >
                  {{ schedule.name }}
                </p>
                <span
                  class="shrink-0 truncate text-[10px] text-muted"
                  :title="schedule.next"
                >
                  {{ schedule.next }}
                </span>
              </div>
            </div>
            <p v-else class="mt-2 text-xs text-muted">No schedules reported.</p>
          </div>
        </CardContent>
      </Card>
    </section>

    <section class="mt-6 grid gap-6">
      <Card class="card-texture dashboard-card">
        <CardHeader class="mb-3">
          <template #title>
            <div class="flex items-center gap-2">
              <Route class="h-4 w-4 text-sky-300" />
              <CardTitle>Routes</CardTitle>
            </div>
          </template>
          <template #description>
            <CardDescription>Active API routes across connected agents.</CardDescription>
          </template>
          <template #action>
            <RefreshButton :on-click="requestRoutesAll" />
          </template>
        </CardHeader>
        <CardContent>
          <div class="overflow-hidden rounded-xl border border-border/60">
            <table class="w-full text-xs">
              <thead class="bg-muted/40 text-muted">
                <tr>
                  <th class="px-4 py-3 text-left">Path</th>
                  <th class="px-4 py-3 text-left">Methods</th>
                  <th class="px-4 py-3 text-left">Handler</th>
                  <th class="px-4 py-3 text-left">Middleware</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="state.routes.length === 0" class="border-t border-border/60">
                  <td colspan="4" class="px-4 py-3 text-muted">No route data yet.</td>
                </tr>
                <tr
                  v-for="route in state.routes"
                  :key="route.path + route.handler"
                  class="border-t border-border/60"
                >
                  <td class="px-4 py-3 text-foreground">{{ route.path }}</td>
                  <td class="px-4 py-3 text-muted">{{ (route.methods || []).join(", ") }}</td>
                  <td class="px-4 py-3 text-muted">{{ route.handler }}</td>
                  <td class="px-4 py-3 text-muted">{{ (route.middlewares || []).join(", ") }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture dashboard-card">
        <CardHeader class="mb-3">
          <template #title>
            <div class="flex items-center gap-2">
              <CalendarClock class="h-4 w-4 text-amber-300" />
              <CardTitle>Schedules</CardTitle>
            </div>
          </template>
          <template #description>
            <CardDescription>Upcoming scheduler jobs from connected agents.</CardDescription>
          </template>
          <template #action>
            <RefreshButton :on-click="requestSchedulesAll" />
          </template>
        </CardHeader>
        <CardContent>
          <div class="overflow-hidden rounded-xl border border-border/60">
            <table class="w-full text-xs">
              <thead class="bg-muted/40 text-muted">
                <tr>
                  <th class="px-4 py-3 text-left">Name</th>
                  <th class="px-4 py-3 text-left">Next Run</th>
                  <th class="px-4 py-3 text-left">Tags</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="state.schedules.length === 0" class="border-t border-border/60">
                  <td colspan="3" class="px-4 py-3 text-muted">No schedule data yet.</td>
                </tr>
                <tr
                  v-for="schedule in state.schedules"
                  :key="schedule.id"
                  class="border-t border-border/60"
                >
                  <td class="px-4 py-3 text-foreground">{{ schedule.name }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.next || schedule.next_run }}</td>
                  <td class="px-4 py-3 text-muted">{{ (schedule.tags || []).join(", ") }}</td>
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
import { computed, onMounted, ref, watch } from "vue";
import { useLighthouseStore } from "../stores/lighthouse";
import { lighthousePath } from "../lib/base-path";
import { Blocks, Boxes, CalendarClock, CheckCircle2, CircleX, Cpu, Database, Globe, HardDrive, Mail, Package, Route, Server, Workflow } from "lucide-vue-next";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import RefreshButton from "../components/ui/button/RefreshButton.vue";

const { state, requestRoutesAll, requestSchedulesAll, sendCommand } = useLighthouseStore();
const totalRoutes = computed(() => state.routes.length);
const totalSchedules = computed(() => state.schedules.length);
const routeSummary = computed(() => {
  const methodCounts = new Map<string, number>();
  const prefixCounts = new Map<string, number>();
  const handlers = new Set<string>();
  let dynamic = 0;

  for (const route of state.routes) {
    if (route.handler) {
      handlers.add(route.handler);
    }
    if (route.path.includes(":") || route.path.includes("*") || route.path.includes("{")) {
      dynamic += 1;
    }
    const segments = route.path.split("/").filter(Boolean);
    const prefix = segments[0] ? `/${segments[0]}` : "/";
    prefixCounts.set(prefix, (prefixCounts.get(prefix) || 0) + 1);
    for (const method of route.methods || []) {
      methodCounts.set(method, (methodCounts.get(method) || 0) + 1);
    }
  }

  const ranked = Array.from(methodCounts.entries())
    .map(([label, count]) => ({ label, count }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label));
  const prefixes = Array.from(prefixCounts.entries())
    .map(([label, count]) => ({ label, count }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label));

  return {
    handlers: handlers.size,
    dynamic,
    methodBreakdown: ranked.slice(0, 4),
    prefixBreakdown: prefixes.slice(0, 3),
  };
});

const methodDotClass = (method: string) => {
  switch (method.toUpperCase()) {
    case "GET":
      return "dashboard-metric-dot-get";
    case "POST":
      return "dashboard-metric-dot-post";
    case "PUT":
      return "dashboard-metric-dot-put";
    case "PATCH":
      return "dashboard-metric-dot-patch";
    case "DELETE":
      return "dashboard-metric-dot-delete";
    default:
      return "dashboard-metric-dot-default";
  }
};

const parseRelativeDurationMs = (value: string) => {
  const normalized = value.trim().toLowerCase().replace(/^in\s+/, "");
  if (!normalized) {
    return null;
  }

  let total = 0;
  let matched = false;
  const pattern = /(\d+)\s*(d|h|m|s)/g;

  for (const match of normalized.matchAll(pattern)) {
    matched = true;
    const amount = Number(match[1]);
    const unit = match[2];
    switch (unit) {
      case "d":
        total += amount * 24 * 60 * 60 * 1000;
        break;
      case "h":
        total += amount * 60 * 60 * 1000;
        break;
      case "m":
        total += amount * 60 * 1000;
        break;
      case "s":
        total += amount * 1000;
        break;
    }
  }

  return matched ? total : null;
};

const parseScheduleTime = (schedule: { next?: string; next_run?: string }) => {
  const value = schedule.next || schedule.next_run || "";
  const relativeMs = parseRelativeDurationMs(value);
  if (relativeMs !== null) {
    return Date.now() + relativeMs;
  }

  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) {
    return null;
  }
  return parsed;
};

const scheduleSummary = computed(() => {
  let tagged = 0;

  for (const schedule of state.schedules) {
    const tags = schedule.tags || [];
    if (tags.length > 0) {
      tagged += 1;
    }
  }

  const parsedUpcoming = [...state.schedules]
    .map((schedule) => ({ schedule, parsed: parseScheduleTime(schedule) }))
    .filter((entry) => entry.parsed !== null)
    .sort((a, b) => (a.parsed as number) - (b.parsed as number))
    .slice(0, 5)
    .map((entry) => ({
      id: entry.schedule.id,
      name: entry.schedule.name || "Unnamed schedule",
      next: entry.schedule.next || entry.schedule.next_run,
    }));
  const fallbackUpcoming = state.schedules.slice(0, 5).map((schedule) => ({
    id: schedule.id,
    name: schedule.name || "Unnamed schedule",
    next: schedule.next || schedule.next_run || schedule.schedule || "Run time unavailable",
  }));
  const uniqueTags = new Set(
    state.schedules.flatMap((schedule) => schedule.tags || [])
  ).size;

  return {
    tagged,
    uniqueTags,
    upcoming: parsedUpcoming.length > 0 ? parsedUpcoming : fallbackUpcoming,
    usingFallbackOrder: parsedUpcoming.length === 0 && fallbackUpcoming.length > 0,
  };
});
const about = ref({
  environment: {
    app_name: "",
    module: "",
    environment: "",
    go_version: "",
    goforj_version: "",
  },
  build: {
    components: [] as string[],
    wire_generated: "",
  },
  network: {
    app_url: "",
  },
  sections: [] as Array<{
    title: string;
    rows?: Array<{ key: string; value: string }>;
    connections?: Array<{
      name: string;
      is_default: boolean;
      details: Array<{ key: string; value: string }>;
    }>;
  }>,
});
const primitiveSections = computed(() =>
  about.value.sections.filter((section) => (section.connections || []).length > 0)
);
const jobTotals = ref({
  pending: 0,
  active: 0,
  scheduled: 0,
  retry: 0,
  processed: 0,
  failed: 0,
});

const refreshJobTotals = async () => {
  const agent = state.agents.find(
    (entry) =>
      entry.capabilities.includes("queue") ||
      entry.capabilities.includes("jobs") ||
      entry.source === "jobs"
  );
  if (!agent) {
    jobTotals.value = { pending: 0, active: 0, scheduled: 0, retry: 0, processed: 0, failed: 0 };
    return;
  }
  const result = await sendCommand(agent.source, "queue:queues", {});
  if (!result?.data) return;
  const payload = typeof result.data === "string" ? JSON.parse(result.data) : result.data;
  const queues = payload.queues || [];
  jobTotals.value = queues.reduce(
    (acc: typeof jobTotals.value, queue: any) => ({
      pending: acc.pending + (queue.pending || 0),
      active: acc.active + (queue.active || 0),
      scheduled: acc.scheduled + (queue.scheduled || 0),
      retry: acc.retry + (queue.retry || 0),
      processed: acc.processed + (queue.processed || 0),
      failed: acc.failed + (queue.failed || 0),
    }),
    { pending: 0, active: 0, scheduled: 0, retry: 0, processed: 0, failed: 0 }
  );
};

const refreshAbout = async () => {
  const res = await fetch(lighthousePath("/api/about"));
  if (!res.ok) {
    return;
  }
  about.value = await res.json();
};

const sectionIcon = (title: string) => {
  switch (title) {
    case "Databases":
      return Database;
    case "Mail":
      return Mail;
    case "Storages":
      return HardDrive;
    case "Caches":
      return Boxes;
    case "Queues":
      return Server;
    case "Events":
      return Workflow;
    default:
      return Package;
  }
};

const formatConnectionDetails = (details: Array<{ key: string; value: string }>) =>
  details.map((detail) => `${detail.key} ${detail.value}`).join(" · ");

onMounted(() => {
  refreshAbout();
  refreshJobTotals();
});

watch(
  () => state.agents,
  () => {
    refreshJobTotals();
  }
);

</script>
