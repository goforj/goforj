<template>
  <div><section class="grid gap-6">
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
          <div class="mb-4 grid gap-4 text-xs sm:grid-cols-2 lg:grid-cols-4">
            <FormField label="Search">
              <Input v-model="query" placeholder="Search logs..." />
            </FormField>
            <FormField label="Source">
              <Select v-model="sourceFilter">
                <option value="">All sources</option>
                <option v-for="source in sources" :key="source" :value="source">
                  {{ source }}
                </option>
              </Select>
            </FormField>
            <FormField label="Level">
              <Select v-model="levelFilter">
                <option value="">All levels</option>
                <option v-for="level in levels" :key="level" :value="level">
                  {{ level }}
                </option>
              </Select>
            </FormField>
            <FormField label="Buffer">
              <Input
                v-model.number="logLimit"
                type="number"
                min="100"
                max="10000"
                class="max-w-[140px]"
                @change="applyLogLimit"
              />
            </FormField>
          </div>
          <div class="max-h-[65vh] overflow-auto rounded-xl border border-border/60">
            <table class="w-full text-xs">
              <thead class="bg-muted/40 text-muted">
                <tr>
                  <th class="px-3 py-2 text-left whitespace-nowrap">Time</th>
                  <th class="px-4 py-2 text-left">Source</th>
                  <th class="px-4 py-2 text-left">Level</th>
                  <th class="px-4 py-2 text-left">Message</th>
                  <th class="px-4 py-2 text-left">Fields</th>
                  <th class="px-2 py-3 text-right"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredLogs.length === 0" class="border-t border-border/60">
                  <td colspan="6" class="px-4 py-2 text-muted">No log data yet.</td>
                </tr>
                <tr
                  v-for="log in filteredLogs"
                  :key="log.time + log.message"
                  class="group border-t border-border/60"
                >
                  <td class="px-3 py-2 text-muted tabular-nums whitespace-nowrap">{{ formatTime(log.time) }}</td>
                  <td class="px-4 py-2 text-white">{{ log.source }}</td>
                  <td class="px-4 py-2">
                    <span :class="levelClass(log.level)">{{ log.level }}</span>
                  </td>
                  <td class="px-4 py-2 text-muted">{{ log.message }}</td>
                  <td class="px-4 py-2 text-muted">{{ formatFields(log.fields) }}</td>
                  <td class="px-2 py-2 text-right">
                    <button
                      class="rounded-md border border-border/60 bg-muted/40 px-2 py-1 text-[10px] text-muted opacity-0 transition group-hover:opacity-100"
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
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import Select from "../components/ui/form/Select.vue";

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
  return date.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
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
    return "rounded-full bg-muted/70 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-muted-foreground";
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
