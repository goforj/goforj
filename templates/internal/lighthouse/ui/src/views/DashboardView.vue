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

      <RoutesSummaryCard :routes="state.routes" detail-to="/routes" />
      <SchedulesSummaryCard :schedules="state.schedules" detail-to="/schedules" />
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useLighthouseStore } from "../stores/lighthouse";
import { lighthousePath } from "../lib/base-path";
import { Blocks, Boxes, CheckCircle2, CircleX, Cpu, Database, Globe, HardDrive, Mail, Package, Server, Workflow } from "lucide-vue-next";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import RoutesSummaryCard from "../components/RoutesSummaryCard.vue";
import SchedulesSummaryCard from "../components/SchedulesSummaryCard.vue";

const { state, sendCommand } = useLighthouseStore();
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

<style scoped>
.dashboard-card {
  position: relative;
  overflow: hidden;
  animation: dashFade 220ms ease-out both;
}

.dashboard-card-hero {
  min-height: 190px;
}

.dashboard-card:nth-of-type(1) { animation-delay: 0ms; }
.dashboard-card:nth-of-type(2) { animation-delay: 60ms; }
.dashboard-card:nth-of-type(3) { animation-delay: 120ms; }

.dashboard-stat-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border-radius: 0.75rem;
  border: 1px solid color-mix(in oklab, var(--border) 70%, transparent);
  background: color-mix(in oklab, var(--muted) 35%, transparent);
  color: var(--foreground);
  box-shadow: inset 0 1px 0 color-mix(in oklab, white 8%, transparent);
}

.dashboard-stat-icon-jobs {
  color: color-mix(in oklab, #7dd3fc 78%, var(--foreground));
  background: color-mix(in oklab, #38bdf8 16%, transparent);
}

.dashboard-metric-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  border-radius: 999px;
  border: 1px solid color-mix(in oklab, var(--border) 68%, transparent);
  background: color-mix(in oklab, var(--background) 72%, transparent);
  padding: 0.38rem 0.7rem;
  font-size: 11px;
  line-height: 1;
  color: var(--foreground);
  white-space: nowrap;
}

.dashboard-metric-dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  flex: 0 0 auto;
}

.dashboard-metric-dot-pending { background: #7dd3fc; }
.dashboard-metric-dot-active { background: #86efac; }
.dashboard-metric-dot-scheduled { background: #fcd34d; }
.dashboard-metric-dot-retry { background: #f9a8d4; }

.dashboard-metric-chip-processed {
  border-color: color-mix(in oklab, #34d399 35%, var(--border));
}

.dashboard-metric-chip-failed {
  border-color: color-mix(in oklab, #fb7185 35%, var(--border));
}

@media (prefers-reduced-motion: reduce) {
  .dashboard-card {
    animation: none;
  }
}

@keyframes dashFade {
  from { opacity: 0; transform: translateY(6px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
