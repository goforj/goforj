<template>
  <div><section class="grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle>Schedules</CardTitle>
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
                variant="destructive"
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
        <CardContent>
          <div class="mb-4 grid gap-4 text-xs sm:grid-cols-2 lg:grid-cols-3">
            <FormField label="Search">
              <Input v-model="query" placeholder="Search schedules..." />
            </FormField>
            <FormField v-if="showAgentFilter" label="Agent">
              <Select v-model="agentFilter">
                <option value="">All agents</option>
                <option v-for="agent in scheduleAgents" :key="agent.source" :value="agent.source">
                  {{ agent.source }}
                </option>
              </Select>
            </FormField>
            <FormField label="Tag">
              <Input v-model="tagFilter" placeholder="Tag filter" />
            </FormField>
          </div>
          <div class="max-h-[70vh] overflow-auto rounded-xl border border-border/60">
            <table class="w-full text-xs">
              <thead class="bg-muted/40 text-muted">
                <tr>
                  <th v-if="showAgentColumn" class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Server class="h-3.5 w-3.5" />
                      Agent
                    </span>
                  </th>
                  <th class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Tag class="h-3.5 w-3.5" />
                      Name
                    </span>
                  </th>
                  <th class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Clock class="h-3.5 w-3.5" />
                      Schedule
                    </span>
                  </th>
                  <th class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Code2 class="h-3.5 w-3.5" />
                      Handler
                    </span>
                  </th>
                  <th v-if="showEditorColumn" class="px-2 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Laptop class="h-3.5 w-3.5" />
                      Editor
                    </span>
                  </th>
                  <th class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Timer class="h-3.5 w-3.5" />
                      Next
                    </span>
                  </th>
                  <th class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <CircleDot class="h-3.5 w-3.5" />
                      State
                    </span>
                  </th>
                  <th class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Hash class="h-3.5 w-3.5" />
                      Tags
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
                    :colspan="showAgentColumn ? (showEditorColumn ? 10 : 9) : showEditorColumn ? 9 : 8"
                    class="px-4 py-3 text-muted"
                  >
                    No schedules found.
                  </td>
                </tr>
                <tr
                  v-for="schedule in filteredSchedules"
                  :key="schedule.id + schedule.source"
                  class="group border-t border-border/60"
                  :class="schedule.paused ? 'opacity-70' : ''"
                >
                  <td v-if="showAgentColumn" class="px-4 py-3 text-foreground">{{ schedule.source }}</td>
                  <td class="px-4 py-3 text-foreground">{{ schedule.name }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.schedule || "-" }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.handler || "-" }}</td>
                  <td v-if="showEditorColumn" class="px-2 py-3 text-left">
                    <EditorDropdown :symbol="editorSymbolForSchedule(schedule)" label="Open in Editor" />
                  </td>
                  <td class="px-4 py-3 text-muted">{{ schedule.next || schedule.next_run }}</td>
                  <td class="px-4 py-3">
                    <Badge variant="secondary" class="border-border/60 bg-muted/40 text-muted-foreground">
                      {{ schedule.paused ? "paused" : "active" }}
                    </Badge>
                  </td>
                  <td class="px-4 py-3 text-muted">{{ (schedule.tags || []).join(", ") }}</td>
                  <td class="px-2 py-3 text-left">
                    <div class="flex items-center gap-2">
                      <Button
                        :variant="schedule.paused ? 'outline' : 'destructive'"
                        size="icon-xs"
                        class="rounded-full"
                        :title="schedule.paused ? 'Start schedule' : 'Stop schedule'"
                        :aria-label="schedule.paused ? 'Start schedule' : 'Stop schedule'"
                        @click="toggleSchedule(schedule)"
                      >
                        <Play v-if="schedule.paused" class="h-3.5 w-3.5" />
                        <Pause v-else class="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="destructive"
                        size="icon-xs"
                        class="rounded-full"
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
import { useDevconsoleStore } from "../stores/devconsole";
import {
  CircleDot,
  Clock,
  Code2,
  Copy,
  Hash,
  BookOpen,
  ExternalLink,
  Laptop,
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
import Input from "../components/ui/form/Input.vue";
import Select from "../components/ui/form/Select.vue";
import RefreshButton from "../components/ui/button/RefreshButton.vue";
import { Badge } from "../components/ui/badge";

const store = useDevconsoleStore();
const { state } = store;
const query = ref("");
const agentFilter = ref(store.state.selectedAgent || "");
const tagFilter = ref("");

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
        schedule.name.toLowerCase().includes(needle) ||
        (schedule.type || "").toLowerCase().includes(needle) ||
        (schedule.schedule || "").toLowerCase().includes(needle) ||
        (schedule.handler || "").toLowerCase().includes(needle) ||
        (schedule.next || schedule.next_run).toLowerCase().includes(needle)
      );
    });
});

const editorSymbolForSchedule = (schedule: any) => {
  const raw = schedule.handler_raw || "";
  if (!raw || raw === "-") return "";
  if (raw.includes(":")) return "";
  if (raw.includes("anon func")) return "";
  if (raw.includes(" ")) return "";
  if (!raw.includes(".")) return "";
  return raw;
};

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
    ? `Start schedule "${schedule.name}"?`
    : `Stop schedule "${schedule.name}"?`;
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
  if (!window.confirm(`Restart schedule "${schedule.name}"?`)) {
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
    `name=${schedule.name}`,
    `type=${schedule.type || ""}`,
    `schedule=${schedule.schedule || ""}`,
    `handler=${schedule.handler || ""}`,
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
</script>
