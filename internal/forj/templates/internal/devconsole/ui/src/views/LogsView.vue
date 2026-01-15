<template>
  <div>
    <PageHeader label="Platform" title="Logs">
      <template #right>
        <AgentPills />
        <LivePill />
      </template>
    </PageHeader>

    <section class="mt-8 grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Logs</p>
            <CardTitle>Streaming logs from all connected agents.</CardTitle>
          </template>
          <template #description>
            <CardDescription>Filter and scan high-volume output here.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 flex flex-wrap items-center gap-3 text-xs">
            <input
              v-model="query"
              type="text"
              placeholder="Search logs..."
              class="h-9 w-full max-w-xs rounded-lg border border-border/70 bg-white/5 px-3 text-xs text-white placeholder:text-muted focus:border-white/30 focus:outline-none"
            />
            <select
              v-model="sourceFilter"
              class="h-9 rounded-lg border border-border/70 bg-white/5 px-3 text-xs text-white focus:border-white/30 focus:outline-none"
            >
              <option value="">All sources</option>
              <option v-for="source in sources" :key="source" :value="source">
                {{ source }}
              </option>
            </select>
            <select
              v-model="levelFilter"
              class="h-9 rounded-lg border border-border/70 bg-white/5 px-3 text-xs text-white focus:border-white/30 focus:outline-none"
            >
              <option value="">All levels</option>
              <option v-for="level in levels" :key="level" :value="level">
                {{ level }}
              </option>
            </select>
            <div class="flex items-center gap-2">
              <span class="text-[10px] uppercase tracking-[0.2em] text-muted">Buffer</span>
              <input
                v-model.number="logLimit"
                type="number"
                min="100"
                max="10000"
                class="h-9 w-24 rounded-lg border border-border/70 bg-white/5 px-3 text-xs text-white focus:border-white/30 focus:outline-none"
                @change="applyLogLimit"
              />
            </div>
          </div>
          <div class="max-h-[65vh] overflow-auto rounded-xl border border-border/70">
            <table class="w-full text-xs">
              <thead class="bg-white/5 text-muted">
                <tr>
                  <th class="px-4 py-3 text-left">Time</th>
                  <th class="px-4 py-3 text-left">Source</th>
                  <th class="px-4 py-3 text-left">Level</th>
                  <th class="px-4 py-3 text-left">Message</th>
                  <th class="px-4 py-3 text-left">Fields</th>
                  <th class="px-2 py-3 text-right"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredLogs.length === 0" class="border-t border-border/70">
                  <td colspan="6" class="px-4 py-3 text-muted">No log data yet.</td>
                </tr>
                <tr
                  v-for="log in filteredLogs"
                  :key="log.time + log.message"
                  class="group border-t border-border/70"
                >
                  <td class="px-4 py-3 text-muted">{{ formatTime(log.time) }}</td>
                  <td class="px-4 py-3 text-white">{{ log.source }}</td>
                  <td class="px-4 py-3">
                    <span :class="levelClass(log.level)">{{ log.level }}</span>
                  </td>
                  <td class="px-4 py-3 text-muted">{{ log.message }}</td>
                  <td class="px-4 py-3 text-muted">{{ formatFields(log.fields) }}</td>
                  <td class="px-2 py-3 text-right">
                    <button
                      class="rounded-md border border-border/70 bg-white/5 px-2 py-1 text-[10px] text-muted opacity-0 transition group-hover:opacity-100"
                      @click="copyLog(log)"
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
import { computed, ref } from "vue";
import { useDevconsoleStore } from "../stores/devconsole";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import AgentPills from "../components/AgentPills.vue";
import PageHeader from "../components/PageHeader.vue";
import LivePill from "../components/LivePill.vue";

const store = useDevconsoleStore();
const { state } = store;
const query = ref("");
const sourceFilter = ref("");
const levelFilter = ref("");
const logLimit = ref(state.logLimit);

const sources = computed(() =>
  Array.from(new Set(state.logs.map((log) => log.source))).filter(Boolean)
);
const levels = computed(() =>
  Array.from(new Set(state.logs.map((log) => log.level))).filter(Boolean)
);


const filteredLogs = computed(() => {
  const needle = query.value.trim().toLowerCase();
  return state.logs.filter((log) => {
    if (sourceFilter.value && log.source !== sourceFilter.value) return false;
    if (levelFilter.value && log.level !== levelFilter.value) return false;
    if (!needle) return true;
    const fieldsText = formatFields(log.fields).toLowerCase();
    return (
      log.message.toLowerCase().includes(needle) ||
      log.source.toLowerCase().includes(needle) ||
      log.level.toLowerCase().includes(needle) ||
      fieldsText.includes(needle)
    );
  });
});

const formatTime = (value: string) => {
  if (!value) return "--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString();
};

const formatFields = (fields: Record<string, any>) => {
  if (!fields) return "";
  return Object.entries(fields)
    .map(([key, value]) => `${key}=${formatFieldValue(value)}`)
    .join(" · ");
};

const formatFieldValue = (value: any) => {
  if (value === null || value === undefined) return "";
  if (typeof value === "object") {
    try {
      return JSON.stringify(value);
    } catch {
      return String(value);
    }
  }
  return String(value);
};

const levelClass = (level: string) => {
  const normalized = level?.toLowerCase?.() || "";
  if (normalized === "error") {
    return "rounded-full bg-red-500/15 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-red-200";
  }
  if (normalized === "warn" || normalized === "warning") {
    return "rounded-full bg-amber-500/15 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-amber-200";
  }
  if (normalized === "debug") {
    return "rounded-full bg-white/10 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-white/70";
  }
  return "rounded-full bg-emerald-500/10 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-emerald-200";
};

const copyLog = async (log: any) => {
  const payload = [
    `time=${formatTime(log.time)}`,
    `source=${log.source}`,
    `level=${log.level}`,
    `message=${log.message}`,
    formatFields(log.fields),
  ]
    .filter(Boolean)
    .join(" · ");
  try {
    await navigator.clipboard.writeText(payload);
  } catch {
    return;
  }
};

const applyLogLimit = () => {
  store.setLogLimit(logLimit.value);
};
</script>
