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
            <Button @click="refresh">Refresh</Button>
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 flex flex-wrap items-center gap-3 text-xs">
            <input
              v-model="query"
              type="text"
              placeholder="Search schedules..."
              class="h-9 w-full max-w-xs rounded-lg border border-border/70 bg-white/5 px-3 text-xs text-white placeholder:text-muted focus:border-white/30 focus:outline-none"
            />
            <select
              v-model="agentFilter"
              class="h-9 rounded-lg border border-border/70 bg-white/5 px-3 text-xs text-white focus:border-white/30 focus:outline-none"
            >
              <option value="">All agents</option>
              <option v-for="agent in scheduleAgents" :key="agent.source" :value="agent.source">
                {{ agent.source }}
              </option>
            </select>
            <input
              v-model="tagFilter"
              type="text"
              placeholder="Tag filter"
              class="h-9 w-40 rounded-lg border border-border/70 bg-white/5 px-3 text-xs text-white placeholder:text-muted focus:border-white/30 focus:outline-none"
            />
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
                  <th class="px-4 py-3 text-left">Tags</th>
                  <th class="px-2 py-3 text-right"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredSchedules.length === 0" class="border-t border-border/70">
                  <td colspan="9" class="px-4 py-3 text-muted">No schedules found.</td>
                </tr>
                <tr
                  v-for="schedule in filteredSchedules"
                  :key="schedule.id + schedule.source"
                  class="group border-t border-border/70"
                >
                  <td class="px-4 py-3 text-white">{{ schedule.source }}</td>
                  <td class="px-4 py-3 text-white">{{ schedule.name }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.type || "-" }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.schedule || "-" }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.handler || "-" }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.next || schedule.next_run }}</td>
                  <td class="px-4 py-3 text-muted">{{ (schedule.tags || []).join(", ") }}</td>
                  <td class="px-2 py-3 text-right">
                    <button
                      class="rounded-md border border-border/70 bg-white/5 px-2 py-1 text-[10px] text-muted opacity-0 transition group-hover:opacity-100"
                      @click="copySchedule(schedule)"
                    >
                      Copy
                    </button>
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
import { useDevconsoleStore } from "../stores/devconsole";
import AgentPills from "../components/AgentPills.vue";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
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
  } catch {
    return;
  }
};
</script>
