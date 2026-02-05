<template>
  <div><section class="grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
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
          <div
            ref="scrollRef"
            class="max-h-[65vh] overflow-auto rounded-xl border border-border/60"
            @scroll="handleScroll"
          >
            <table class="w-full text-xs">
              <thead class="bg-muted text-muted sticky top-0 z-10">
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
                <tr v-else class="border-0">
                  <td :style="{ height: `${topSpacerHeight}px` }" class="p-0" colspan="6"></td>
                </tr>
                <tr
                  v-for="entry in visibleEntries"
                  :key="entry.idx"
                  class="group border-t border-border/60 h-9"
                >
                  <td class="px-3 py-2 text-muted tabular-nums whitespace-nowrap">
                    {{ entry.log._timeLabel ?? formatTime(entry.log.time) }}
                  </td>
                  <td class="px-4 py-2 text-foreground whitespace-nowrap">{{ entry.log.source }}</td>
                  <td class="px-4 py-2">
                    <Badge variant="secondary" class="border-border/60 bg-muted/40 text-muted-foreground">
                      {{ entry.log.level }}
                    </Badge>
                  </td>
                  <td class="px-4 py-2 text-muted truncate max-w-[520px]" :title="entry.log.message">
                    {{ entry.log.message }}
                  </td>
                  <td
                    class="px-4 py-2 text-muted truncate max-w-[520px]"
                    :title="entry.log._fieldsText ?? formatFields(entry.log.fields)"
                  >
                    {{ entry.log._fieldsText ?? formatFields(entry.log.fields) }}
                  </td>
                  <td class="px-2 py-2 text-right">
                    <button
                      class="rounded-md border border-border/60 bg-muted/40 px-2 py-1 text-[10px] text-muted opacity-0 transition group-hover:opacity-100"
                      @click="copyLog(entry.log)"
                    >
                      Copy
                    </button>
                  </td>
                </tr>
                <tr v-if="filteredLogs.length > 0" class="border-0">
                  <td :style="{ height: `${bottomSpacerHeight}px` }" class="p-0" colspan="6"></td>
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
import { useDevconsoleStore } from "../stores/devconsole";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import Select from "../components/ui/form/Select.vue";
import { Badge } from "../components/ui/badge";

const store = useDevconsoleStore();
const { state } = store;
const query = ref("");
const debouncedQuery = ref("");
const sourceFilter = ref("");
const levelFilter = ref("");
const logLimit = ref(state.logLimit);
const scrollRef = ref<HTMLDivElement | null>(null);
const scrollTop = ref(0);
const containerHeight = ref(0);
const rowHeight = 36;
const overscan = 12;

const sources = computed(() =>
  Array.from(new Set(state.logs.map((log) => log.source))).filter(Boolean)
);
const levels = computed(() =>
  Array.from(new Set(state.logs.map((log) => log.level))).filter(Boolean)
);


const filteredLogs = computed(() => {
  const needle = debouncedQuery.value.trim().toLowerCase();
  return state.logs.filter((log) => {
    if (sourceFilter.value && log.source !== sourceFilter.value) return false;
    if (levelFilter.value && log.level !== levelFilter.value) return false;
    if (!needle) return true;
    const fieldsText = (log._fieldsText ?? formatFields(log.fields)).toLowerCase();
    return (
      (log._messageLower ?? log.message.toLowerCase()).includes(needle) ||
      (log._sourceLower ?? log.source.toLowerCase()).includes(needle) ||
      (log._levelLower ?? log.level.toLowerCase()).includes(needle) ||
      fieldsText.includes(needle)
    );
  });
});

const totalLogs = computed(() => filteredLogs.value.length);
const startIndex = computed(() => {
  if (!containerHeight.value) return 0;
  return Math.max(0, Math.floor(scrollTop.value / rowHeight) - overscan);
});
const endIndex = computed(() => {
  if (!containerHeight.value) return totalLogs.value;
  const visible = Math.ceil(containerHeight.value / rowHeight) + overscan * 2;
  return Math.min(totalLogs.value, startIndex.value + visible);
});
const visibleLogs = computed(() => filteredLogs.value.slice(startIndex.value, endIndex.value));
const visibleEntries = computed(() =>
  visibleLogs.value.map((log, index) => ({
    log,
    idx: startIndex.value + index,
  }))
);
const topSpacerHeight = computed(() => startIndex.value * rowHeight);
const bottomSpacerHeight = computed(() => (totalLogs.value - endIndex.value) * rowHeight);

const handleScroll = () => {
  if (!scrollRef.value) return;
  scrollTop.value = scrollRef.value.scrollTop;
};

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

let debounceHandle: number | null = null;
watch(query, (value) => {
  if (debounceHandle !== null) {
    window.clearTimeout(debounceHandle);
  }
  debounceHandle = window.setTimeout(() => {
    debouncedQuery.value = value;
  }, 200);
});

const syncContainerHeight = () => {
  if (!scrollRef.value) return;
  containerHeight.value = scrollRef.value.clientHeight;
};

onMounted(() => {
  debouncedQuery.value = query.value;
  syncContainerHeight();
  window.addEventListener("resize", syncContainerHeight);
});

onUnmounted(() => {
  window.removeEventListener("resize", syncContainerHeight);
  if (debounceHandle !== null) {
    window.clearTimeout(debounceHandle);
  }
});
</script>
