<template>
  <div>
    <PageHeader label="Platform" title="Schedules">
      <template #right>
        <AgentPills />
        <LivePill />
      </template>
    </PageHeader>

    <section class="mt-8 grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Schedules</p>
            <CardTitle>Upcoming jobs across agents.</CardTitle>
          </template>
          <template #description>
            <CardDescription>Filter schedules by agent or tag.</CardDescription>
          </template>
          <template #action>
            <div class="flex items-center gap-2">
              <Button
                v-if="canFreezeAll && !pausedAll"
                variant="outline"
                size="sm"
                @click="pauseAll"
              >
                <Pause class="mr-1 h-3.5 w-3.5" />
                Stop All
              </Button>
              <Button
                v-if="canFreezeAll && pausedAll"
                variant="outline"
                size="sm"
                @click="resumeAll"
              >
                <Play class="mr-1 h-3.5 w-3.5" />
                Start All
              </Button>
              <Button variant="outline" size="sm" @click="refresh">
                <RefreshCw class="mr-1 h-3.5 w-3.5" />
                Refresh
              </Button>
            </div>
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 grid gap-4 text-xs sm:grid-cols-2 lg:grid-cols-3">
            <FormField label="Search">
              <Input v-model="query" placeholder="Search schedules..." />
            </FormField>
            <FormField label="Agent">
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
          <div class="max-h-[70vh] overflow-auto rounded-xl border border-border/70">
            <table class="w-full text-xs">
              <thead class="bg-white/5 text-muted">
                <tr>
                  <th class="px-4 py-3 text-left">Agent</th>
                  <th class="px-4 py-3 text-left">Name</th>
                  <th class="px-4 py-3 text-left">Type</th>
                  <th class="px-4 py-3 text-left">Schedule</th>
                  <th class="px-4 py-3 text-left">Handler</th>
                  <th class="px-4 py-3 text-left">Next</th>
                  <th class="px-4 py-3 text-left">State</th>
                  <th class="px-4 py-3 text-left">Tags</th>
                  <th class="px-2 py-3 text-left">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredSchedules.length === 0" class="border-t border-border/70">
                  <td colspan="10" class="px-4 py-3 text-muted">No schedules found.</td>
                </tr>
                <tr
                  v-for="schedule in filteredSchedules"
                  :key="schedule.id + schedule.source"
                  class="group border-t border-border/70"
                  :class="schedule.paused ? 'opacity-70' : ''"
                >
                  <td class="px-4 py-3 text-white">{{ schedule.source }}</td>
                  <td class="px-4 py-3 text-white">{{ schedule.name }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.type || "-" }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.schedule || "-" }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.handler || "-" }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.next || schedule.next_run }}</td>
                  <td class="px-4 py-3">
                    <span
                      class="rounded-full border px-2 py-1 text-[10px] uppercase tracking-wide"
                      :class="
                        schedule.paused
                          ? 'border-amber-500/40 bg-amber-500/10 text-amber-200'
                          : 'border-emerald-400/40 bg-emerald-400/10 text-emerald-200'
                      "
                    >
                      {{ schedule.paused ? "paused" : "active" }}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-muted">{{ (schedule.tags || []).join(", ") }}</td>
                  <td class="px-2 py-3 text-left">
                    <div class="flex items-center gap-2">
                      <button
                        class="flex items-center gap-1 rounded-md border border-border/70 bg-white/5 px-2 py-1 text-[10px] text-muted transition active:scale-95 active:border-accent active:bg-accent/30"
                        :title="schedule.paused ? 'Start schedule' : 'Stop schedule'"
                        :aria-label="schedule.paused ? 'Start schedule' : 'Stop schedule'"
                        @click="toggleSchedule(schedule)"
                      >
                        <Play v-if="schedule.paused" class="h-3.5 w-3.5" />
                        <Pause v-else class="h-3.5 w-3.5" />
                        <span>{{ schedule.paused ? "Start" : "Stop" }}</span>
                      </button>
                      <button
                        class="flex items-center gap-1 rounded-md border border-border/70 bg-white/5 px-2 py-1 text-[10px] text-muted transition active:scale-95 active:border-accent active:bg-accent/30"
                        title="Restart schedule"
                        aria-label="Restart schedule"
                        @click="restartSchedule(schedule)"
                      >
                        <RotateCw class="h-3.5 w-3.5" />
                        <span>Restart</span>
                      </button>
                      <button
                        class="flex h-7 w-7 items-center justify-center rounded-md border border-border/70 bg-white/5 text-muted transition active:scale-95 active:border-accent active:bg-accent/30"
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
import { Copy, Pause, Play, RefreshCw, RotateCw } from "lucide-vue-next";
import AgentPills from "../components/AgentPills.vue";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import Select from "../components/ui/form/Select.vue";
import PageHeader from "../components/PageHeader.vue";
import LivePill from "../components/LivePill.vue";

const store = useDevconsoleStore();
const { state } = store;
const query = ref("");
const agentFilter = ref(store.state.selectedAgent || "");
const tagFilter = ref("");

const scheduleAgents = computed(() =>
  state.agents.filter((agent) => agent.capabilities.includes("schedule"))
);

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

const refresh = () => {
  if (agentFilter.value) {
    store.requestSchedules(agentFilter.value);
    return;
  }
  store.requestSchedulesAll();
};

const pauseAll = async () => {
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
