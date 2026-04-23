<template>
  <div><section class="grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <CalendarClock class="h-4 w-4 text-muted-foreground" />
              Schedules
            </CardTitle>
          </template>
          <template #description>
            <div class="flex flex-wrap items-center gap-2 text-xs text-muted">
              <span>
                Schedules are defined in
                <code class="font-mono text-[11px] text-muted-foreground"
                  >internal/scheduler/scheduler_registry.go</code
                >.
              </span>
              <EditorDropdown label="Open in Editor" symbol="scheduler.Scheduler.Register" />
              <a
                class="flex items-center gap-1 rounded-md border border-border/60 bg-muted/40 px-2 py-1 text-[10px] text-muted-foreground transition active:scale-95 active:bg-muted"
                href="https://goforj.dev/scheduler#api-index"
                target="_blank"
                rel="noreferrer"
              >
                <BookOpen class="h-3.5 w-3.5" />
                <span>API Docs</span>
                <ExternalLink class="h-3.5 w-3.5" />
              </a>
            </div>
          </template>
          <template #action>
            <div class="flex items-center gap-2">
              <Button
                v-if="canFreezeAll && !pausedAll"
                variant="outline"
                class="border-destructive/60 text-destructive hover:bg-destructive/10 hover:text-destructive"
                size="sm"
                @click="pauseAll"
              >
                <Pause class="mr-1 h-3.5 w-3.5" />
                Stop All
              </Button>
              <Button
                v-if="canFreezeAll && pausedAll"
                variant="default"
                size="sm"
                @click="resumeAll"
              >
                <Play class="mr-1 h-3.5 w-3.5" />
                Start All
              </Button>
              <RefreshButton :on-click="refresh" />
            </div>
          </template>
        </CardHeader>
        <CardContent class="space-y-4">
          <div class="grid gap-4 text-xs sm:grid-cols-2 lg:grid-cols-3">
            <FormField label="Search">
              <Input v-model="query" placeholder="Search schedules..." />
            </FormField>
            <FormField v-if="showAgentFilter" label="Agent">
              <Select v-model="agentFilterModel">
                <SelectTrigger class="w-full">
                  <SelectValue placeholder="All agents" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem :value="allSelectValue">All agents</SelectItem>
                  <SelectItem v-for="agent in scheduleAgents" :key="agent.source" :value="agent.source">
                  {{ agent.source }}
                  </SelectItem>
                </SelectContent>
              </Select>
            </FormField>
            <FormField label="Tag">
              <Input v-model="tagFilter" placeholder="Tag filter" />
            </FormField>
          </div>
          <div class="schedules-overview-strip">
            <span class="schedules-overview-chip">
              <span class="schedules-overview-chip-label">Schedules</span>
              <span class="schedules-overview-chip-value">{{ filteredSchedules.length }}</span>
            </span>
            <span class="schedules-overview-chip">
              <span class="schedules-overview-chip-label">Active</span>
              <span class="schedules-overview-chip-value">{{ scheduleOverview.active }}</span>
            </span>
            <span class="schedules-overview-chip">
              <span class="schedules-overview-chip-label">Paused</span>
              <span class="schedules-overview-chip-value">{{ scheduleOverview.paused }}</span>
            </span>
            <span class="schedules-overview-chip">
              <span class="schedules-overview-chip-label">Tagged</span>
              <span class="schedules-overview-chip-value">{{ scheduleOverview.tagged }}</span>
            </span>
          </div>
          <div
            class="max-h-[calc(100vh-21rem)] overflow-auto rounded-xl border border-border/60"
            :class="filteredSchedules.length > 8 ? 'min-h-[22rem]' : ''"
          >
            <table class="w-full min-w-[980px] text-xs">
              <colgroup>
                <col v-if="showAgentColumn" style="width: 8rem" />
                <col style="width: 19rem" />
                <col style="width: 10rem" />
                <col style="width: 14rem" />
                <col style="width: 10rem" />
                <col style="width: 8rem" />
                <col style="width: 8rem" />
              </colgroup>
              <thead class="sticky top-0 z-10 bg-muted/85 text-muted backdrop-blur supports-[backdrop-filter]:bg-muted/70">
                <tr>
                  <th v-if="showAgentColumn" class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Server class="h-3.5 w-3.5" />
                      Agent
                    </span>
                  </th>
                  <th class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Tag class="h-3.5 w-3.5" />
                      Name
                    </span>
                  </th>
                  <th class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Clock class="h-3.5 w-3.5" />
                      Schedule
                    </span>
                  </th>
                  <th class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Code2 class="h-3.5 w-3.5" />
                      Handler
                    </span>
                  </th>
                  <th class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Timer class="h-3.5 w-3.5" />
                      Next
                    </span>
                  </th>
                  <th class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <CircleDot class="h-3.5 w-3.5" />
                      State
                    </span>
                  </th>
                  <th class="px-2 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <SlidersHorizontal class="h-3.5 w-3.5" />
                      Actions
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredSchedules.length === 0" class="border-t border-border/60">
                  <td
                    :colspan="showAgentColumn ? 7 : 6"
                    class="px-4 py-2.5 text-muted"
                  >
                    No schedules found.
                  </td>
                </tr>
                <tr
                  v-for="schedule in filteredSchedules"
                  :key="schedule.id + schedule.source"
                  class="schedules-table-row group border-t border-border/60"
                  :class="schedule.paused ? 'opacity-70' : ''"
                >
                  <td v-if="showAgentColumn" class="px-4 py-2 text-foreground">
                    <span class="schedules-agent-pill">{{ schedule.source }}</span>
                  </td>
                  <td class="px-4 py-2 text-foreground align-top">
                    <div class="space-y-1">
                      <span class="schedules-name-chip">{{ displayScheduleName(schedule) }}</span>
                      <div v-if="(schedule.tags || []).length > 0" class="flex flex-wrap gap-1.5">
                        <span
                          v-for="tag in (schedule.tags || []).slice(0, 2)"
                          :key="schedule.id + tag"
                          class="schedules-tag-chip"
                        >
                          {{ tag }}
                        </span>
                        <span
                          v-if="(schedule.tags || []).length > 2"
                          class="schedules-tag-chip schedules-tag-chip-overflow"
                        >
                          +{{ (schedule.tags || []).length - 2 }} more
                        </span>
                      </div>
                    </div>
                  </td>
                  <td class="px-4 py-2 text-muted align-top">
                    <span class="schedules-plan-chip">{{ schedule.schedule || "-" }}</span>
                  </td>
                  <td class="px-4 py-2 text-muted align-top">
                    <div class="flex min-w-0 items-center gap-2">
                      <span class="schedules-handler-chip min-w-0 truncate">{{ displayHandlerForSchedule(schedule) }}</span>
                      <EditorDropdown
                        v-if="showEditorColumn && canOpenEditorSymbol(editorSymbolForSchedule(schedule))"
                        :symbol="editorSymbolForSchedule(schedule)"
                        label="Open in Editor"
                        compact
                      />
                    </div>
                  </td>
                  <td class="px-4 py-2 text-muted align-top">
                    <span class="schedules-next-chip">{{ schedule.next || schedule.next_run }}</span>
                  </td>
                  <td class="px-4 py-2 align-top">
                    <Badge
                      variant="secondary"
                      class="border-border/60 bg-muted/40 text-muted-foreground"
                      :class="schedule.paused ? 'schedules-state-badge-paused' : 'schedules-state-badge-active'"
                    >
                      {{ schedule.paused ? "paused" : "active" }}
                    </Badge>
                  </td>
                  <td class="px-2 py-2 text-left align-top">
                    <div class="flex items-center gap-2">
                      <Button
                        variant="outline"
                        size="icon-xs"
                        class="rounded-full border-destructive/60 text-destructive hover:bg-destructive/10 hover:text-destructive"
                        :title="schedule.paused ? 'Start schedule' : 'Stop schedule'"
                        :aria-label="schedule.paused ? 'Start schedule' : 'Stop schedule'"
                        @click="toggleSchedule(schedule)"
                      >
                        <Play v-if="schedule.paused" class="h-3.5 w-3.5" />
                        <Pause v-else class="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon-xs"
                        class="rounded-full border-destructive/60 text-destructive hover:bg-destructive/10 hover:text-destructive"
                        title="Restart schedule"
                        aria-label="Restart schedule"
                        @click="restartSchedule(schedule)"
                      >
                        <RotateCw class="h-3.5 w-3.5" />
                      </Button>
                      <button
                        class="flex h-7 w-7 items-center justify-center rounded-md border border-border/60 bg-muted/40 text-muted-foreground transition active:scale-95 active:bg-muted"
                        title="Copy schedule"
                        aria-label="Copy schedule"
                        @click="copySchedule(schedule)"
                      >
                        <Copy class="h-3.5 w-3.5" />
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
import { computed, ref, watch } from "vue";
import { toast } from "vue-sonner";
import { useLighthouseStore } from "../stores/lighthouse";
import {
  CircleDot,
  CalendarClock,
  Clock,
  Code2,
  Copy,
  BookOpen,
  ExternalLink,
  Pause,
  Play,
  RotateCw,
  Server,
  SlidersHorizontal,
  Tag,
  Timer,
} from "lucide-vue-next";
import EditorDropdown from "../components/EditorDropdown.vue";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/input/Input.vue";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../components/ui/select";
import RefreshButton from "../components/ui/button/RefreshButton.vue";
import { Badge } from "../components/ui/badge";

const store = useLighthouseStore();
const { state } = store;
const allSelectValue = "__all__";
const query = ref("");
const agentFilter = ref(store.state.selectedAgent || "");
const tagFilter = ref("");

const agentFilterModel = computed({
  get: () => agentFilter.value || allSelectValue,
  set: (value: string) => {
    agentFilter.value = value === allSelectValue ? "" : value;
  },
});

const scheduleAgents = computed(() =>
  state.agents.filter((agent) => agent.capabilities.includes("schedule"))
);

const showAgentColumn = computed(() => scheduleAgents.value.length > 1);
const showAgentFilter = computed(() => scheduleAgents.value.length > 1);
const showEditorColumn = computed(() => state.localClient);

const activeAgent = computed(() => {
  if (agentFilter.value) return agentFilter.value;
  if (scheduleAgents.value.length === 1) {
    return scheduleAgents.value[0].source;
  }
  return "";
});

const pausedAll = computed(() => {
  if (agentFilter.value) {
    const flagged = Boolean(state.schedulesPausedAllByAgent[agentFilter.value]);
    if (flagged) return true;
    const schedules = state.schedulesByAgent[agentFilter.value] || [];
    if (schedules.length === 0) return false;
    return schedules.every((schedule) => schedule.paused);
  }
  if (scheduleAgents.value.length === 0) return false;
  return scheduleAgents.value.every((agent) => {
    const flagged = Boolean(state.schedulesPausedAllByAgent[agent.source]);
    if (flagged) return true;
    const schedules = state.schedulesByAgent[agent.source] || [];
    if (schedules.length === 0) return false;
    return schedules.every((schedule) => schedule.paused);
  });
});

const canFreezeAll = computed(() => scheduleAgents.value.length > 0);


watch(
  () => scheduleAgents.value,
  (agents) => {
    if (agents.length === 0) {
      agentFilter.value = "";
      return;
    }
    if (agents.length === 1) {
      agentFilter.value = agents[0].source;
      return;
    }
    const selected = store.state.selectedAgent;
    if (selected && agents.some((agent) => agent.source === selected)) {
      agentFilter.value = selected;
      return;
    }
    agentFilter.value = "";
  },
  { immediate: true }
);

watch(
  () => store.state.selectedAgent,
  (value) => {
    if (!value) return;
    if (scheduleAgents.value.some((agent) => agent.source === value)) {
      agentFilter.value = value;
    }
  }
);

const filteredSchedules = computed(() => {
  const needle = query.value.trim().toLowerCase();
  const tagNeedle = tagFilter.value.trim().toLowerCase();
  const schedules = agentFilter.value
    ? state.schedulesByAgent[agentFilter.value] || []
    : state.schedules;
  return schedules
    .map((schedule) => ({ ...schedule, source: schedule.source || agentFilter.value || "scheduler" }))
    .filter((schedule) => {
      if (tagNeedle && !(schedule.tags || []).some((tag) => tag.toLowerCase().includes(tagNeedle))) {
        return false;
      }
      if (!needle) return true;
        return (
          displayScheduleName(schedule).toLowerCase().includes(needle) ||
          (schedule.type || "").toLowerCase().includes(needle) ||
          (schedule.schedule || "").toLowerCase().includes(needle) ||
          displayHandlerForSchedule(schedule).toLowerCase().includes(needle) ||
          (schedule.next || schedule.next_run).toLowerCase().includes(needle)
        );
    });
});

const scheduleOverview = computed(() => {
  let paused = 0;
  let tagged = 0;

  for (const schedule of filteredSchedules.value) {
    if (schedule.paused) {
      paused += 1;
    }
    if ((schedule.tags || []).length > 0) {
      tagged += 1;
    }
  }

  return {
    active: Math.max(filteredSchedules.value.length - paused, 0),
    paused,
    tagged,
  };
});

const editorCandidateSymbols = computed(() =>
  Array.from(
    new Set(
      filteredSchedules.value
        .map((schedule) => editorSymbolForSchedule(schedule))
        .filter(Boolean)
    )
  )
);

const displayHandlerForSchedule = (schedule: any) => {
  const handler = String(schedule?.handler || "").trim();
  if (!handler) return "-";
  if (isAnonymousSchedule(schedule)) {
    return "anonymous callback";
  }
  const parts = handler.split(".");
  if (parts.length === 2 && /^[A-Z]/.test(parts[0]) && parts[1]) {
    return parts[1];
  }
  return handler;
};

const isAnonymousSchedule = (schedule: any) => {
  const handler = String(schedule?.handler || "").trim().toLowerCase();
  const rawHandler = String(schedule?.handler_raw || "").trim().toLowerCase();
  return handler.includes("anon func") || rawHandler.includes("anon func");
};

const hasGeneratedScheduleName = (schedule: any) => {
  const name = String(schedule?.name || "").trim();
  if (!name || name === "-") return true;
  const lower = name.toLowerCase();
  return (
    name.includes(".func") ||
    lower.includes("dowithrunner") ||
    lower.includes("jobbuilder") ||
    lower.includes("scheduler.register") ||
    /^v\d+\./i.test(name) ||
    name.includes("JobBuilder") ||
    name.includes("github.com/")
  );
};

const displayScheduleName = (schedule: any) => {
  if (isAnonymousSchedule(schedule) && hasGeneratedScheduleName(schedule)) {
    return "Unnamed callback";
  }
  if (hasGeneratedScheduleName(schedule)) {
    const handler = displayHandlerForSchedule(schedule);
    if (handler && handler !== "-") {
      return handler;
    }
  }
  return String(schedule?.name || "-").trim() || "-";
};

const editorSymbolForSchedule = (schedule: any) => {
  return String(schedule?.editor_symbol || "").trim();
};

const canOpenEditorSymbol = (symbol?: string) => store.canOpenEditorSymbol(symbol);

const refresh = async () => {
  if (agentFilter.value) {
    store.requestSchedules(agentFilter.value);
    return;
  }
  store.requestSchedulesAll();
};

const pauseAll = async () => {
  if (
    !window.confirm(
      "Stop all schedules? Running schedules will be paused until resumed."
    )
  ) {
    return;
  }
  if (agentFilter.value) {
    await handleScheduleAction(agentFilter.value, "pause-all", "Stopped all schedules");
    return;
  }
  const targets = scheduleAgents.value.map((agent) => agent.source);
  for (const target of targets) {
    await handleScheduleAction(target, "pause-all", "Stopped all schedules");
  }
};

const resumeAll = async () => {
  if (
    !window.confirm(
      "Start all schedules? This will resume any paused schedules."
    )
  ) {
    return;
  }
  if (agentFilter.value) {
    await handleScheduleAction(agentFilter.value, "resume-all", "Started all schedules");
    return;
  }
  const targets = scheduleAgents.value.map((agent) => agent.source);
  for (const target of targets) {
    await handleScheduleAction(target, "resume-all", "Started all schedules");
  }
};

const toggleSchedule = async (schedule: any) => {
  const target = schedule.source || activeAgent.value || "scheduler";
  if (!target) return;
  const confirmMessage = schedule.paused
    ? `Start schedule "${displayScheduleName(schedule)}"?`
    : `Stop schedule "${displayScheduleName(schedule)}"?`;
  if (!window.confirm(confirmMessage)) {
    return;
  }
  const label = schedule.paused ? "Started schedule" : "Stopped schedule";
  await handleScheduleAction(
    target,
    schedule.paused ? "resume" : "pause",
    label,
    schedule.id
  );
};


const restartSchedule = async (schedule: any) => {
  const target = schedule.source || activeAgent.value || "scheduler";
  if (!target) return;
  if (!window.confirm(`Restart schedule "${displayScheduleName(schedule)}"?`)) {
    return;
  }
  await handleScheduleAction(target, "restart", "Restarted schedule", schedule.id);
};

const handleScheduleAction = async (
  target: string,
  action: string,
  successMessage: string,
  id?: string
) => {
  try {
    const response = await store.runScheduleAction(target, action, id);
    if (!response?.ok) {
      toast(response?.error || "Schedule action failed");
      return;
    }
    toast(successMessage);
    refresh();
  } catch (err: any) {
    toast(err?.message || "Schedule action failed");
  }
};

const copySchedule = async (schedule: any) => {
  const payload = [
    `agent=${schedule.source}`,
    `name=${displayScheduleName(schedule)}`,
    `type=${schedule.type || ""}`,
    `schedule=${schedule.schedule || ""}`,
    `handler=${displayHandlerForSchedule(schedule)}`,
    `next=${schedule.next || schedule.next_run}`,
    `tags=${(schedule.tags || []).join(",")}`,
  ]
    .filter(Boolean)
    .join(" · ");
  try {
    await navigator.clipboard.writeText(payload);
    toast("Schedule copied to clipboard");
  } catch (error: any) {
    const message = error?.message || "Unable to copy schedule.";
    toast(message);
  }
};

watch(
  () => [showEditorColumn.value, ...editorCandidateSymbols.value],
  async ([enabled]) => {
    if (!enabled || editorCandidateSymbols.value.length === 0) {
      return;
    }
    try {
      await store.validateEditorSymbols(editorCandidateSymbols.value);
    } catch {
      //
    }
  },
  { immediate: true }
);
</script>

<style scoped>
.schedules-overview-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
}

.schedules-overview-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  border-radius: 999px;
  border: 1px solid color-mix(in oklab, var(--border) 68%, transparent);
  background: color-mix(in oklab, var(--card) 88%, transparent);
  padding: 0.38rem 0.62rem;
  box-shadow: inset 0 1px 0 color-mix(in oklab, white 4%, transparent);
}

.schedules-overview-chip-label {
  font-size: 10px;
  line-height: 1;
  text-transform: uppercase;
  color: var(--muted-foreground);
}

.schedules-overview-chip-value {
  font-size: 12px;
  line-height: 1;
  font-weight: 700;
  color: var(--foreground);
}

.schedules-table-row {
  transition: background-color 140ms ease, box-shadow 140ms ease;
}

.schedules-table-row:hover {
  background: color-mix(in oklab, var(--muted) 18%, transparent);
  box-shadow: inset 3px 0 0 color-mix(in oklab, #f59e0b 34%, transparent);
}

.schedules-agent-pill,
.schedules-name-chip,
.schedules-plan-chip,
.schedules-handler-chip,
.schedules-next-chip,
.schedules-tag-chip {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  border: 1px solid color-mix(in oklab, var(--border) 68%, transparent);
}

.schedules-agent-pill {
  background: color-mix(in oklab, var(--background) 76%, transparent);
  padding: 0.24rem 0.5rem;
  font-size: 11px;
  line-height: 1;
}

.schedules-name-chip {
  background: color-mix(in oklab, #f59e0b 9%, transparent);
  border-color: color-mix(in oklab, #f59e0b 24%, var(--border));
  padding: 0.24rem 0.5rem;
  font-size: 11px;
  line-height: 1.1;
  color: var(--foreground);
  max-width: 100%;
}

.schedules-plan-chip,
.schedules-next-chip {
  background: color-mix(in oklab, var(--background) 78%, transparent);
  padding: 0.2rem 0.42rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 10px;
  line-height: 1.2;
  color: var(--muted-foreground);
}

.schedules-handler-chip {
  background: color-mix(in oklab, var(--muted) 18%, transparent);
  padding: 0.26rem 0.48rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  line-height: 1.2;
  color: var(--foreground);
}

.schedules-state-badge-active {
  border-color: color-mix(in oklab, #22c55e 26%, var(--border));
  color: #86efac;
}

.schedules-state-badge-paused {
  border-color: color-mix(in oklab, #f59e0b 26%, var(--border));
  color: #fcd34d;
}

.schedules-tag-chip {
  background: color-mix(in oklab, var(--muted) 16%, transparent);
  padding: 0.18rem 0.4rem;
  font-size: 10px;
  line-height: 1;
  color: var(--muted-foreground);
}

.schedules-tag-chip-overflow {
  color: var(--foreground);
}
</style>
