<template>
  <div class="h-[calc(100vh-6rem)] overflow-hidden">
    <section class="grid h-full min-h-0 gap-5 xl:grid-cols-[19rem_minmax(0,1fr)]">
      <Card class="card-texture flex min-h-0 flex-col">
        <CardHeader class="pb-2">
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <Workflow class="h-4 w-4 text-muted-foreground" />
              {{ inspectTitle }}
            </CardTitle>
          </template>
          <template #action>
            <RefreshButton :refreshing="refreshing" :on-click="refresh" />
          </template>
        </CardHeader>
        <CardContent class="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden pb-3">
          <div class="grid gap-3">
            <FormField label="Search">
              <Input v-model="query" :placeholder="searchPlaceholder" />
            </FormField>
            <FormField v-if="showSourceFilter" label="Source">
              <Select v-model="sourceFilterModel">
                <SelectTrigger class="w-full">
                  <SelectValue placeholder="All sources" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem :value="allSelectValue">All sources</SelectItem>
                  <SelectItem v-for="source in sourceOptions" :key="source" :value="source">
                    {{ source }}
                  </SelectItem>
                </SelectContent>
              </Select>
            </FormField>
            <div class="flex items-center justify-between rounded-xl border border-border/60 bg-muted/10 px-3 py-2">
              <div>
                <p class="text-[11px] font-medium text-foreground">Show internal inspects</p>
                <p class="text-[10px] text-muted">Include Lighthouse API and websocket requests.</p>
              </div>
              <Switch v-model="showInternal" aria-label="Show internal inspects" />
            </div>
          </div>

          <div class="flex items-center justify-between text-[11px] text-muted">
            <span>{{ filteredTraces.length }} shown</span>
            <span v-if="!showInternal">{{ internalTraceCount }} internal hidden</span>
          </div>

          <div class="min-h-0 flex-1 space-y-1 overflow-y-auto px-1 pb-1 pt-1">
            <button
              v-for="trace in filteredTraces"
              :key="trace.trace_id"
              type="button"
              class="relative isolate w-full overflow-hidden rounded-xl border px-3 py-2 text-left transition outline-none focus:outline-none focus-visible:outline-none focus-visible:ring-0 focus-visible:ring-offset-0"
              :class="traceRowClass(trace)"
              @click="selectTrace(trace.trace_id)"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <p class="truncate text-sm font-semibold text-foreground">{{ traceDisplayName(trace) }}</p>
                  <p class="mt-0.5 truncate text-[10px] text-muted">{{ shortTraceID(trace.trace_id) }}</p>
                </div>
                <Badge :variant="statusBadgeVariant(trace.status)">
                  {{ trace.status || "unknown" }}
                </Badge>
              </div>
              <div class="mt-1.5 flex flex-wrap items-center gap-1.5 text-[11px] text-muted">
                <span class="rounded-full border border-border/60 bg-background/60 px-2 py-0.5">{{ trace.source || "app" }}</span>
                <span :class="durationClass(trace.duration_ms)">{{ formatTime(trace.started_at) }}</span>
                <span :class="durationClass(trace.duration_ms)">{{ formatDuration(trace.duration_ms) }}</span>
                <span>{{ trace.event_count }} events</span>
              </div>
            </button>
            <div v-if="filteredTraces.length === 0" class="rounded-xl border border-dashed border-border/60 px-4 py-8 text-sm text-muted">
              No inspects matched the current filters.
            </div>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture flex min-h-0 flex-col">
        <CardHeader class="pb-2">
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <Binary class="h-4 w-4 text-muted-foreground" />
              {{ selectedRecord ? traceDisplayName(selectedRecord.summary) : `${inspectTitle} detail` }}
            </CardTitle>
          </template>
          <template #description>
            <CardDescription v-if="selectedRecord">
              {{ shortTraceID(selectedRecord.summary.trace_id) }} · {{ selectedRecord.summary.source }} · {{ selectedRecord.events.length }} events
            </CardDescription>
            <CardDescription v-else>Select an inspect to view its event timeline.</CardDescription>
          </template>
        </CardHeader>
        <CardContent class="min-h-0 flex-1 overflow-hidden">
          <div v-if="selectedRecord" class="flex h-full min-h-0 flex-col gap-3">
            <div class="flex flex-wrap items-center gap-2 text-[11px]">
              <span class="rounded-full border border-border/60 bg-muted/10 px-2.5 py-1 text-[11px] text-foreground">
                source={{ selectedRecord.summary.source }}
              </span>
              <span class="rounded-full border border-border/60 bg-muted/10 px-2.5 py-1 text-[11px] text-foreground">
                status={{ selectedRecord.summary.status || "running" }}
              </span>
              <span class="rounded-full border border-border/60 bg-muted/10 px-2.5 py-1 text-[11px] text-foreground">
                started={{ formatDateTime(selectedRecord.summary.started_at) }}
              </span>
              <span class="rounded-full border border-border/60 bg-muted/10 px-2.5 py-1 text-[11px] text-foreground">
                duration={{ formatDuration(selectedRecord.summary.duration_ms) }}
              </span>
              <span class="rounded-full border border-border/60 bg-muted/10 px-2.5 py-1 text-[11px] text-foreground">
                events={{ selectedRecord.events.length }}
              </span>
            </div>

            <div v-if="labelEntries(selectedRecord.summary.labels).length > 0" class="flex flex-wrap gap-2 rounded-xl border border-border/60 bg-muted/10 px-3 py-2">
              <span class="text-[10px] uppercase tracking-[0.2em] text-muted">Labels</span>
              <div class="flex flex-wrap gap-2">
                <span
                  v-for="[key, value] in labelEntries(selectedRecord.summary.labels)"
                  :key="key"
                  class="rounded-full border border-border/60 bg-background/60 px-2 py-0.5 text-[11px] text-foreground"
                >
                  {{ key }}={{ value }}
                </span>
              </div>
            </div>

            <Tabs v-if="requestExchange" v-model="activeInspectTab" class="min-h-0 flex-1 gap-3">
              <TabsList class="w-fit">
                <TabsTrigger value="timeline">Timeline</TabsTrigger>
                <TabsTrigger value="request">Request</TabsTrigger>
                <TabsTrigger value="headers">Headers</TabsTrigger>
                <TabsTrigger value="response">Response</TabsTrigger>
              </TabsList>
              <TabsContent value="timeline" class="min-h-0 flex-1 mt-0">
                <div class="h-full overflow-y-auto rounded-xl border border-border/60 bg-muted/10">
                  <div class="border-b border-border/60 px-4 py-2.5">
                    <div class="flex items-center justify-between gap-3">
                      <p class="text-xs font-medium text-foreground">Event timeline</p>
                      <p class="text-[11px] text-muted">ordered capture</p>
                    </div>
                  </div>
                  <div class="divide-y divide-border/60">
                    <div
                      v-for="event in timelineEvents"
                      :key="`${event.seq}-${event.at}`"
                      class="grid items-center gap-2 px-4 py-1 lg:grid-cols-[7rem_minmax(0,1fr)]"
                    >
                      <div class="text-[11px] text-muted">
                        <p class="flex h-8 items-center whitespace-nowrap font-medium">
                          <span :class="traceOffsetClass(selectedRecord.summary.started_at, event.at)">
                            {{ formatTraceOffset(selectedRecord.summary.started_at, event.at) }}
                          </span>
                          <span class="ml-1 tabular-nums text-muted">{{ formatTime(event.at) }}</span>
                        </p>
                      </div>
                      <div class="space-y-1">
                        <div class="overflow-x-auto pb-0.5">
                          <div class="flex min-w-max items-center gap-1.5 whitespace-nowrap text-[11px] leading-none">
                            <span
                              class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full border px-2.5 text-[11px] font-medium capitalize"
                              :class="eventKindPillClass(event.kind)"
                            >
                              <component :is="eventKindIcon(event.kind)" class="h-3.5 w-3.5" />
                              {{ event.kind }}
                            </span>
                            <Badge v-if="event.level" class="shrink-0 self-center" variant="secondary">{{ event.level }}</Badge>
                            <Badge v-if="event.status" class="shrink-0 self-center" :variant="statusBadgeVariant(event.status)">{{ event.status }}</Badge>
                            <p class="flex h-8 shrink-0 items-center self-center text-sm font-semibold leading-none text-foreground">{{ eventHeadline(event) }}</p>
                            <template v-for="(field, index) in eventInlineFields(event)" :key="`${event.seq}-${field.key}`">
                              <span v-if="index > 0" class="flex h-8 shrink-0 items-center self-center text-muted">•</span>
                              <span class="flex h-8 shrink-0 items-center self-center gap-1 text-[11px] leading-none">
                                <span
                                  v-if="field.label"
                                  class="text-muted"
                                  :class="field.labelClassName"
                                >
                                  {{ field.label }}
                                </span>
                                <span
                                  class="text-foreground"
                                  :class="field.valueClassName"
                                >
                                  {{ field.value }}
                                </span>
                              </span>
                            </template>
                          </div>
                        </div>
                        <div
                          v-if="eventSummaryLine(event)"
                          class="text-[11px] text-muted"
                          :title="eventSummaryLine(event)"
                        >
                          {{ eventSummaryLine(event) }}
                        </div>
                        <div
                          v-if="eventShapePreview(event)"
                          class="rounded-md border border-border/50 bg-background/80 px-2.5 py-1.5"
                        >
                          <pre class="whitespace-pre-wrap break-words text-[11px] leading-5 text-muted"><code v-html="eventShapePreviewHTML(event)"></code></pre>
                        </div>
                        <div v-if="eventExtraFields(event).length > 0" class="rounded-lg border border-border/50 bg-background/60 px-2.5 py-1.5">
                          <dl class="grid gap-x-4 gap-y-1 text-[11px] sm:grid-cols-2 xl:grid-cols-3">
                            <template v-for="[key, value] in eventExtraFields(event)" :key="`${event.seq}-extra-${key}`">
                              <dt class="text-muted">{{ key }}</dt>
                              <dd class="truncate" :class="genericValueClass(value)" :title="value">{{ value }}</dd>
                            </template>
                          </dl>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </TabsContent>
              <TabsContent value="request" class="min-h-0 flex-1 mt-0">
                <div class="h-full overflow-y-auto rounded-xl border border-border/60 bg-muted/10 p-4 space-y-4">
                  <div class="flex items-start justify-between gap-3">
                    <div class="space-y-1">
                      <p class="text-xs uppercase tracking-[0.16em] text-muted">Request</p>
                      <p class="text-sm font-semibold text-foreground">{{ requestLine }}</p>
                      <p class="text-xs text-muted break-all">{{ requestURL }}</p>
                    </div>
                    <RefreshButton
                      variant="outline"
                      :on-click="copyCurl"
                      :refreshing="copyingCurl"
                      label="Copy to Curl"
                      refreshing-label="Copying"
                    />
                  </div>
                  <div class="rounded-lg border border-border/50 bg-background/80 p-3">
                    <p class="mb-2 text-xs font-medium text-foreground">Body</p>
                    <pre
                      class="whitespace-pre-wrap break-words text-[11px] leading-5 text-muted"
                    ><code v-html="requestBodyDisplayHTML"></code></pre>
                  </div>
                </div>
              </TabsContent>
              <TabsContent value="headers" class="min-h-0 flex-1 mt-0">
                <div class="h-full overflow-y-auto rounded-xl border border-border/60 bg-muted/10 p-4 space-y-4">
                  <div class="rounded-lg border border-border/50 bg-background/60 p-3">
                    <p class="mb-2 text-xs font-medium text-foreground">Request headers</p>
                    <dl
                      v-if="requestHeaderEntries.length > 0"
                      class="grid grid-cols-[minmax(0,10rem)_minmax(0,1fr)] gap-x-4 gap-y-1 text-[11px]"
                    >
                      <template v-for="[key, value] in requestHeaderEntries" :key="`request-${key}`">
                        <dt class="break-words text-muted">{{ key }}</dt>
                        <dd class="break-words whitespace-pre-wrap" :class="genericValueClass(value)" :title="value">{{ value }}</dd>
                      </template>
                    </dl>
                    <p v-else class="text-[11px] text-muted">(none)</p>
                  </div>
                  <div class="rounded-lg border border-border/50 bg-background/60 p-3">
                    <p class="mb-2 text-xs font-medium text-foreground">Response headers</p>
                    <dl
                      v-if="responseHeaderEntries.length > 0"
                      class="grid grid-cols-[minmax(0,10rem)_minmax(0,1fr)] gap-x-4 gap-y-1 text-[11px]"
                    >
                      <template v-for="[key, value] in responseHeaderEntries" :key="`response-${key}`">
                        <dt class="break-words text-muted">{{ key }}</dt>
                        <dd class="break-words whitespace-pre-wrap" :class="genericValueClass(value)" :title="value">{{ value }}</dd>
                      </template>
                    </dl>
                    <p v-else class="text-[11px] text-muted">(none)</p>
                  </div>
                </div>
              </TabsContent>
              <TabsContent value="response" class="min-h-0 flex-1 mt-0">
                <div class="h-full overflow-y-auto rounded-xl border border-border/60 bg-muted/10 p-4 space-y-4">
                  <div class="space-y-1">
                    <p class="text-xs uppercase tracking-[0.16em] text-muted">Response</p>
                    <p class="text-sm font-semibold text-foreground">{{ responseStatusLine }}</p>
                  </div>
                  <div class="rounded-lg border border-border/50 bg-background/80 p-3">
                    <p class="mb-2 text-xs font-medium text-foreground">Body</p>
                    <pre
                      class="whitespace-pre-wrap break-words text-[11px] leading-5 text-muted"
                    ><code v-html="responseBodyDisplayHTML"></code></pre>
                  </div>
                </div>
              </TabsContent>
            </Tabs>
            <div v-else class="min-h-0 flex-1 overflow-y-auto rounded-xl border border-border/60 bg-muted/10">
              <div class="border-b border-border/60 px-4 py-2.5">
                <div class="flex items-center justify-between gap-3">
                  <p class="text-xs font-medium text-foreground">Event timeline</p>
                  <p class="text-[11px] text-muted">ordered capture</p>
                </div>
              </div>
              <div class="divide-y divide-border/60">
                <div
                  v-for="event in timelineEvents"
                  :key="`${event.seq}-${event.at}`"
                  class="grid items-center gap-2 px-4 py-1 lg:grid-cols-[7rem_minmax(0,1fr)]"
                >
                  <div class="text-[11px] text-muted">
                    <p class="flex h-8 items-center whitespace-nowrap font-medium">
                      <span :class="traceOffsetClass(selectedRecord.summary.started_at, event.at)">
                        {{ formatTraceOffset(selectedRecord.summary.started_at, event.at) }}
                      </span>
                      <span class="ml-1 tabular-nums text-muted">{{ formatTime(event.at) }}</span>
                    </p>
                  </div>
                  <div class="space-y-1">
                    <div class="overflow-x-auto pb-0.5">
                      <div class="flex min-w-max items-center gap-1.5 whitespace-nowrap text-[11px] leading-none">
                        <span
                          class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full border px-2.5 text-[11px] font-medium capitalize"
                          :class="eventKindPillClass(event.kind)"
                        >
                          <component :is="eventKindIcon(event.kind)" class="h-3.5 w-3.5" />
                          {{ event.kind }}
                        </span>
                        <Badge v-if="event.level" class="shrink-0 self-center" variant="secondary">{{ event.level }}</Badge>
                        <Badge v-if="event.status" class="shrink-0 self-center" :variant="statusBadgeVariant(event.status)">{{ event.status }}</Badge>
                        <p class="flex h-8 shrink-0 items-center self-center text-sm font-semibold leading-none text-foreground">{{ eventHeadline(event) }}</p>
                        <template v-for="(field, index) in eventInlineFields(event)" :key="`${event.seq}-${field.key}`">
                          <span v-if="index > 0" class="flex h-8 shrink-0 items-center self-center text-muted">•</span>
                          <span class="flex h-8 shrink-0 items-center self-center gap-1 text-[11px] leading-none">
                            <span
                              v-if="field.label"
                              class="text-muted"
                              :class="field.labelClassName"
                            >
                              {{ field.label }}
                            </span>
                            <span
                              class="text-foreground"
                              :class="field.valueClassName"
                            >
                              {{ field.value }}
                            </span>
                          </span>
                        </template>
                      </div>
                    </div>
                    <div
                      v-if="eventSummaryLine(event)"
                      class="text-[11px] text-muted"
                      :title="eventSummaryLine(event)"
                    >
                      {{ eventSummaryLine(event) }}
                    </div>
                    <div
                      v-if="eventShapePreview(event)"
                      class="rounded-md border border-border/50 bg-background/80 px-2.5 py-1.5"
                    >
                      <pre class="whitespace-pre-wrap break-words text-[11px] leading-5 text-muted"><code v-html="eventShapePreviewHTML(event)"></code></pre>
                    </div>
                    <div v-if="eventExtraFields(event).length > 0" class="rounded-lg border border-border/50 bg-background/60 px-2.5 py-1.5">
                      <dl class="grid gap-x-4 gap-y-1 text-[11px] sm:grid-cols-2 xl:grid-cols-3">
                        <template v-for="[key, value] in eventExtraFields(event)" :key="`${event.seq}-extra-${key}`">
                          <dt class="text-muted">{{ key }}</dt>
                          <dd class="truncate" :class="genericValueClass(value)" :title="value">{{ value }}</dd>
                        </template>
                      </dl>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="rounded-xl border border-dashed border-border/60 px-4 py-12 text-sm text-muted">
            Select an inspect from the list to view its event timeline.
          </div>
        </CardContent>
      </Card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { Binary, Database, HardDrive, Package, ScrollText, Tag, TriangleAlert, Workflow } from "lucide-vue-next";
import { useRoute, useRouter } from "vue-router";
import { lighthousePath } from "../lib/base-path";
import { escapeHTML, highlightJSON, maybePrettyJSON, renderBodyHTML } from "../lib/json-preview";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/input/Input.vue";
import RefreshButton from "../components/ui/button/RefreshButton.vue";
import { Badge } from "../components/ui/badge";
import Switch from "../components/ui/switch/Switch.vue";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../components/ui/tabs";

type TraceSummary = {
  trace_id: string;
  source: string;
  name: string;
  status: string;
  started_at: string;
  updated_at: string;
  ended_at?: string;
  duration_ms?: number;
  event_count: number;
  labels?: Record<string, string>;
};

type TraceEvent = {
  seq: number;
  at: string;
  kind: string;
  level?: string;
  name?: string;
  message?: string;
  status?: string;
  attributes?: Record<string, unknown>;
};

type TraceRecord = {
  summary: TraceSummary;
  events: TraceEvent[];
};

type HTTPExchange = {
  method: string;
  scheme: string;
  host: string;
  uri: string;
  requestHeaders: Record<string, string>;
  requestBody: string;
  responseStatus: number;
  responseHeaders: Record<string, string>;
  responseBody: string;
};

type InlineField = {
  key: string;
  label?: string;
  value: string;
  labelClassName?: string;
  valueClassName?: string;
};

const allSelectValue = "__all__";
const refreshing = ref(false);
const traces = ref<TraceSummary[]>([]);
const selectedTraceId = ref("");
const selectedRecord = ref<TraceRecord | null>(null);
const query = ref("");
const sourceFilter = ref("");
const showInternal = ref(false);
const route = useRoute();
const router = useRouter();
const activeInspectTab = ref("timeline");
const copyingCurl = ref(false);

const inspectTitle = computed(() => String(route.meta.inspectTitle || route.meta.title || "Inspect"));
const inspectSource = computed(() => String(route.meta.inspectSource || "").trim());
const showSourceFilter = computed(() => inspectSource.value === "");
const searchPlaceholder = computed(() => {
  switch (inspectSource.value) {
    case "http":
      return "Request path or id";
    case "cli":
      return "Command name or id";
    case "jobs":
      return "Job name or id";
    case "scheduler":
      return "Schedule name or id";
    default:
      return "Inspect record or id";
  }
});
const inspectRecordLabel = computed(() => {
  switch (inspectSource.value) {
    case "http":
      return "request";
    case "cli":
      return "command";
    case "jobs":
      return "job";
    case "scheduler":
      return "schedule";
    default:
      return "record";
  }
});

const sourceFilterModel = computed({
  get: () => (showSourceFilter.value ? sourceFilter.value || allSelectValue : inspectSource.value || allSelectValue),
  set: (value: string) => {
    if (!showSourceFilter.value) return;
    sourceFilter.value = value === allSelectValue ? "" : value;
  },
});

const sourceOptions = computed(() =>
  Array.from(new Set(traces.value.map((trace) => trace.source).filter(Boolean))).sort()
);

const internalTraceCount = computed(() => traces.value.filter((trace) => isInternalTrace(trace)).length);

const filteredTraces = computed(() => {
  const needle = query.value.trim().toLowerCase();
  return traces.value.filter((trace) => {
    if (!showInternal.value && isInternalTrace(trace)) return false;
    if (inspectSource.value && trace.source !== inspectSource.value) return false;
    if (!inspectSource.value && sourceFilter.value && trace.source !== sourceFilter.value) return false;
    if (!needle) return true;
    return (
      trace.trace_id.toLowerCase().includes(needle) ||
      traceDisplayName(trace).toLowerCase().includes(needle) ||
      (trace.source || "").toLowerCase().includes(needle)
    );
  });
});

const normalizeHeaderMap = (value: unknown): Record<string, string> => {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const out: Record<string, string> = {};
  for (const [key, raw] of Object.entries(value as Record<string, unknown>)) {
    const name = String(key || "").trim();
    if (!name) continue;
    if (raw === undefined || raw === null) continue;
    out[name] = typeof raw === "string" ? raw : JSON.stringify(raw);
  }
  return out;
};

const requestExchange = computed<HTTPExchange | null>(() => {
  if (!selectedRecord.value) return null;
  const event = selectedRecord.value.events.find((candidate) => candidate.kind === "http" && candidate.name === "http_exchange");
  if (!event) return null;
  return {
    method: readAttr(event, "method"),
    scheme: readAttr(event, "scheme"),
    host: readAttr(event, "host"),
    uri: readAttr(event, "uri"),
    requestHeaders: normalizeHeaderMap(event.attributes?.request_headers),
    requestBody: readAttr(event, "request_body"),
    responseStatus: Number(event.attributes?.response_status) || 0,
    responseHeaders: normalizeHeaderMap(event.attributes?.response_headers),
    responseBody: readAttr(event, "response_body"),
  };
});

const timelineEvents = computed(() => {
  if (!selectedRecord.value) return [];
  return selectedRecord.value.events.filter((event) => !(event.kind === "http" && event.name === "http_exchange"));
});

const inspectURL = (exchange: HTTPExchange) => {
  const uri = String(exchange.uri || "").trim();
  if (uri.startsWith("http://") || uri.startsWith("https://")) return uri;
  const scheme = String(exchange.scheme || "http").trim() || "http";
  const host = String(exchange.host || "").trim();
  if (!host) return uri || "/";
  const path = uri.startsWith("/") ? uri : `/${uri}`;
  return `${scheme}://${host}${path}`;
};

const requestLine = computed(() => {
  if (!requestExchange.value) return "";
  const method = requestExchange.value.method || "GET";
  const uri = requestExchange.value.uri || "/";
  return `${method} ${uri}`;
});

const requestURL = computed(() => (requestExchange.value ? inspectURL(requestExchange.value) : ""));

const sortedEntries = (record: Record<string, string>) =>
  Object.entries(record).sort(([left], [right]) => left.localeCompare(right));

const requestHeaderEntries = computed(() => (requestExchange.value ? sortedEntries(requestExchange.value.requestHeaders) : []));
const responseHeaderEntries = computed(() => (requestExchange.value ? sortedEntries(requestExchange.value.responseHeaders) : []));

const requestBodyDisplay = computed(() => {
  if (!requestExchange.value) return "";
  return requestExchange.value.requestBody || "(empty)";
});

const responseBodyDisplay = computed(() => {
  if (!requestExchange.value) return "";
  return requestExchange.value.responseBody || "(empty)";
});

const requestBodyDisplayHTML = computed(() => renderBodyHTML(requestBodyDisplay.value));
const responseBodyDisplayHTML = computed(() => renderBodyHTML(responseBodyDisplay.value));

const responseStatusLine = computed(() => {
  if (!requestExchange.value) return "";
  const code = requestExchange.value.responseStatus;
  return code > 0 ? `Status ${code}` : "Status unknown";
});

const shellEscape = (value: string) => `'${String(value).replaceAll("'", `'\"'\"'`)}'`;

const copyCurl = async () => {
  if (!requestExchange.value || copyingCurl.value) return;
  copyingCurl.value = true;
  try {
    const exchange = requestExchange.value;
    const url = inspectURL(exchange);
    const command: string[] = ["curl"];
    if (exchange.method && exchange.method.toUpperCase() !== "GET") {
      command.push("-X", shellEscape(exchange.method.toUpperCase()));
    }
    for (const [key, value] of sortedEntries(exchange.requestHeaders)) {
      const lowerKey = key.toLowerCase();
      if (lowerKey === "host" || lowerKey === "content-length") continue;
      command.push("-H", shellEscape(`${key}: ${value}`));
    }
    if (exchange.requestBody) {
      command.push("--data-raw", shellEscape(exchange.requestBody));
    }
    command.push(shellEscape(url));
    await navigator.clipboard.writeText(command.join(" "));
  } finally {
    window.setTimeout(() => {
      copyingCurl.value = false;
    }, 500);
  }
};

const refresh = async () => {
  refreshing.value = true;
  try {
    const requestedSource = inspectSource.value || sourceFilter.value;
    const sourceQuery = requestedSource ? `&source=${encodeURIComponent(requestedSource)}` : "";
    const res = await fetch(lighthousePath(`/api/inspect?limit=100${sourceQuery}`));
    if (!res.ok) return;
    const payload = (await res.json()) as { traces?: TraceSummary[] };
    traces.value = payload.traces || [];
    const routeSelected = readRouteTraceID();
    const defaultSelected = routeSelected || filteredTraces.value[0]?.trace_id || traces.value[0]?.trace_id || "";
    if (!selectedTraceId.value) {
      selectedTraceId.value = defaultSelected;
    }
    const stillSelected = filteredTraces.value.some((trace) => trace.trace_id === selectedTraceId.value);
    if (!stillSelected) {
      selectedTraceId.value = defaultSelected;
    }
    await loadSelectedTrace();
  } finally {
    refreshing.value = false;
  }
};

const loadSelectedTrace = async () => {
  if (!selectedTraceId.value) {
    selectedRecord.value = null;
    return;
  }
  const res = await fetch(lighthousePath(`/api/inspect/${encodeURIComponent(selectedTraceId.value)}`));
  if (!res.ok) {
    selectedRecord.value = null;
    return;
  }
  selectedRecord.value = (await res.json()) as TraceRecord;
};

const selectTrace = async (traceID: string) => {
  selectedTraceId.value = traceID;
  syncTraceToRoute(traceID);
  await loadSelectedTrace();
};

const readRouteTraceID = () => {
  const trace = route.query.trace;
  return typeof trace === "string" ? trace.trim() : "";
};

const syncTraceToRoute = (traceID: string) => {
  const current = readRouteTraceID();
  if (current === traceID) return;
  router.replace({
    query: {
      ...route.query,
      trace: traceID || undefined,
    },
  });
};

const isInternalTrace = (trace: TraceSummary) => {
  const path = String(trace.labels?.path || "").trim().toLowerCase();
  const name = String(trace.name || "").trim().toLowerCase();
  return path.startsWith("/lighthouse/") || name.includes("/lighthouse/");
};

const shortTraceID = (traceID: string) => {
  if (!traceID) return "";
  if (traceID.length <= 24) return traceID;
  return `${traceID.slice(0, 12)}...${traceID.slice(-8)}`;
};

const traceDisplayName = (trace: TraceSummary) => {
  const path = trace.labels?.path ? String(trace.labels.path) : "";
  if (path && path !== trace.name) {
    const method = trace.labels?.method ? String(trace.labels.method) : "";
    return method ? `${method} ${path}` : path;
  }
  return trace.name || "Inspect";
};

const formatDateTime = (value?: string) => {
  if (!value) return "--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString([], {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
};

const formatTime = (value?: string) => {
  if (!value) return "--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
};

const formatRounded = (value: number, digits: number) => {
  const rounded = value.toFixed(digits);
  return rounded.replace(/\.0+$|(\.\d*[1-9])0+$/, "$1");
};

const formatDuration = (durationMs?: number, durationNs?: number) => {
  const ns = Number(durationNs) || 0;
  if (ns > 0 && ns < 1_000_000) {
    if (ns < 1_000) return `${Math.max(1, Math.round(ns))} ns`;
    const micros = ns / 1_000;
    return `${formatRounded(micros, micros < 10 ? 1 : 0)} µs`;
  }
  const ms = Number(durationMs) || (ns > 0 ? ns / 1_000_000 : 0);
  if (ms <= 0) return "0 ms";
  if (ms < 10) {
    return `${formatRounded(ms, 2)} ms`;
  }
  if (ms < 1000) {
    return `${formatRounded(ms, ms < 100 ? 1 : 0)} ms`;
  }
  const seconds = ms / 1000;
  return `${formatRounded(seconds, seconds < 10 ? 2 : 1)} s`;
};

const durationClass = (durationMs?: number) => {
  const ms = Number(durationMs) || 0;
  if (ms < 10) return "text-emerald-400";
  if (ms < 50) return "text-sky-400";
  if (ms < 150) return "text-amber-400";
  if (ms < 500) return "text-orange-400";
  return "text-rose-400";
};

const formatTraceOffset = (startedAt?: string, eventAt?: string) => {
  if (!startedAt || !eventAt) return "+0 ms";
  const start = new Date(startedAt).getTime();
  const at = new Date(eventAt).getTime();
  if (Number.isNaN(start) || Number.isNaN(at)) return "+0 ms";
  return `+${formatDuration(Math.max(0, at - start))}`;
};

const traceOffsetClass = (startedAt?: string, eventAt?: string) => {
  if (!startedAt || !eventAt) return "text-muted";
  const start = new Date(startedAt).getTime();
  const at = new Date(eventAt).getTime();
  if (Number.isNaN(start) || Number.isNaN(at)) return "text-muted";
  const delta = Math.max(0, at - start);
  if (delta < 10) return "text-emerald-400";
  if (delta < 50) return "text-sky-400";
  if (delta < 150) return "text-amber-400";
  if (delta < 500) return "text-orange-400";
  return "text-rose-400";
};

const labelEntries = (labels?: Record<string, string>) =>
  Object.entries(labels || {}).filter(([, value]) => String(value || "").trim() !== "");

const readAttr = (event: TraceEvent, key: string) => {
  const value = event.attributes?.[key];
  if (value === undefined || value === null) return "";
  if (typeof value === "boolean") return value ? "true" : "false";
  if (typeof value === "number") return Number.isFinite(value) ? `${value}` : "";
  if (typeof value === "string") return value.trim();
  return JSON.stringify(value);
};

const eventHeadline = (event: TraceEvent) => {
  switch (event.kind) {
    case "cache": {
      const operation = readAttr(event, "operation") || event.name || "operation";
      const cacheName = readAttr(event, "cache") || "cache";
      return `${operation} ${cacheName}`;
    }
    case "queue": {
      const kind = readAttr(event, "kind") || event.name || "event";
      const jobName = readAttr(event, "job_name") || "job";
      return `${kind} ${jobName}`;
    }
    case "query": {
      const operation = readAttr(event, "operation") || event.name || "query";
      const target = readAttr(event, "target");
      return target ? `${operation} ${target}` : operation;
    }
    case "log":
      return event.message || "log entry";
    case "error":
      return event.message || "error";
    default:
      return event.message || event.name || event.kind;
  }
};

const eventSubline = (event: TraceEvent) => {
  switch (event.kind) {
    case "log":
      return readAttr(event, "source");
    default:
      return "";
  }
};

const eventDurationClass = (durationMs: number) => {
  return durationClass(durationMs);
};

const eventDurationParts = (event: TraceEvent) => {
  const durationNs = Number(readAttr(event, "duration_ns")) || 0;
  const durationMs = Number(readAttr(event, "duration_ms")) || (durationNs > 0 ? durationNs / 1_000_000 : 0);
  return { durationMs, durationNs };
};

const boolValueClass = (value: string) => {
  const trimmedValue = String(value || "").trim().toLowerCase();
  if (trimmedValue === "true") return "text-emerald-300";
  if (trimmedValue === "false") return "text-amber-300";
  return "";
};

const durationValueClass = (value: string) => {
  const text = String(value || "");
  if (text.includes("ns")) return "text-fuchsia-300";
  if (text.includes("µs")) return "text-violet-300";
  const raw = Number.parseFloat(text.replace(/[^\d.-]/g, ""));
  return eventDurationClass(Number.isFinite(raw) ? raw : 0);
};

const genericValueClass = (value: string) => {
  const boolClass = boolValueClass(value);
  if (boolClass) return boolClass;
  const trimmedValue = String(value || "").trim().toLowerCase();
  if (trimmedValue.startsWith("http")) return "text-sky-300";
  if (/^\d+(\.\d+)?$/.test(trimmedValue)) return "text-amber-300";
  if (/^(mysql|postgres|sqlite|redis|memory|s3|local|nats|rabbitmq|sqs)$/i.test(trimmedValue)) return "text-violet-300";
  return "text-foreground";
};

const eventInlineFields = (event: TraceEvent): InlineField[] => {
  const { durationMs, durationNs } = eventDurationParts(event);
  const durationField = (key = "duration"): InlineField => ({
    key,
    label: "duration",
    value: formatDuration(durationMs, durationNs),
    valueClassName: durationValueClass(formatDuration(durationMs, durationNs)),
  });
  switch (event.kind) {
    case "cache":
      return [
        pair("driver", readAttr(event, "driver"), "text-violet-300"),
        pair("hit", readAttr(event, "hit"), boolValueClass(readAttr(event, "hit"))),
        durationField(),
      ].filter(Boolean) as InlineField[];
    case "queue":
      return [
        pair("queue", readAttr(event, "queue"), "text-cyan-300"),
        pair("attempt", readAttr(event, "attempt"), "text-amber-300"),
        pair("scheduled", readAttr(event, "scheduled"), boolValueClass(readAttr(event, "scheduled"))),
        durationField(),
      ].filter(Boolean) as InlineField[];
    case "query":
      return [
        pair("connection", readAttr(event, "connection"), "text-cyan-300"),
        pair("driver", readAttr(event, "driver"), "text-violet-300"),
        pair("fingerprint", readAttr(event, "fingerprint"), "text-muted"),
        durationField(),
      ].filter(Boolean) as InlineField[];
    case "log":
      return [
        durationField(),
      ].filter(Boolean) as InlineField[];
    default:
      return [];
  }
};

const eventKindIcon = (kind?: string) => {
  switch ((kind || "").toLowerCase()) {
    case "query":
      return Database;
    case "cache":
      return HardDrive;
    case "storage":
      return HardDrive;
    case "queue":
      return Package;
    case "log":
      return ScrollText;
    case "error":
      return TriangleAlert;
    case "annotation":
      return Tag;
    default:
      return Binary;
  }
};

const eventKindPillClass = (kind?: string) => {
  switch ((kind || "").toLowerCase()) {
    case "query":
      return "border-sky-500/30 bg-sky-500/10 text-sky-200";
    case "cache":
      return "border-amber-500/30 bg-amber-500/10 text-amber-200";
    case "storage":
      return "border-violet-500/30 bg-violet-500/10 text-violet-200";
    case "queue":
      return "border-emerald-500/30 bg-emerald-500/10 text-emerald-200";
    case "log":
      return "border-border/60 bg-muted/40 text-foreground";
    case "error":
      return "border-rose-500/30 bg-rose-500/10 text-rose-200";
    case "annotation":
      return "border-fuchsia-500/30 bg-fuchsia-500/10 text-fuchsia-200";
    default:
      return "border-border/60 bg-muted/20 text-muted";
  }
};

const eventSummaryLine = (event: TraceEvent) => {
  switch (event.kind) {
    case "queue": {
      const queueName = readAttr(event, "queue");
      const attempt = readAttr(event, "attempt");
      const scheduled = readAttr(event, "scheduled");
      const parts = [queueName && `queue ${queueName}`, attempt && `attempt ${attempt}`, scheduled && `scheduled ${scheduled}`].filter(Boolean);
      return parts.join(" · ");
    }
    case "log": {
      const attrs = eventExtraFields(event)
        .slice(0, 4)
        .map(([key, value]) => `${key}=${value}`);
      return attrs.join(" · ");
    }
    default:
      return "";
  }
};

const eventShapePreview = (event: TraceEvent) => {
  if (event.kind !== "query") return "";
  return readAttr(event, "shape");
};

const escapeHTML = (value: string) =>
  value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");

const sqlKeywords = new Set([
  "select",
  "from",
  "where",
  "order",
  "by",
  "group",
  "having",
  "limit",
  "offset",
  "insert",
  "into",
  "values",
  "update",
  "set",
  "delete",
  "join",
  "left",
  "right",
  "inner",
  "outer",
  "on",
  "and",
  "or",
  "not",
  "in",
  "is",
  "null",
  "like",
  "ilike",
  "as",
  "distinct",
  "with",
  "union",
  "all",
  "case",
  "when",
  "then",
  "else",
  "end",
  "true",
  "false",
]);

const wrapSQLToken = (className: string, value: string) => `<span class="${className}">${escapeHTML(value)}</span>`;

const highlightSQL = (sql: string) => {
  let out = "";
  let i = 0;
  while (i < sql.length) {
    const ch = sql[i];
    if (ch === "`") {
      let j = i + 1;
      while (j < sql.length && sql[j] !== "`") j += 1;
      if (j < sql.length) j += 1;
      out += wrapSQLToken("text-violet-300", sql.slice(i, j));
      i = j;
      continue;
    }
    if (ch === "'") {
      let j = i + 1;
      while (j < sql.length) {
        if (sql[j] === "'" && sql[j - 1] !== "\\") {
          j += 1;
          break;
        }
        j += 1;
      }
      out += wrapSQLToken("text-emerald-300", sql.slice(i, j));
      i = j;
      continue;
    }
    if (ch === "?") {
      out += wrapSQLToken("text-rose-300", ch);
      i += 1;
      continue;
    }
    if (/\d/.test(ch)) {
      let j = i + 1;
      while (j < sql.length && /[\d.]/.test(sql[j])) j += 1;
      out += wrapSQLToken("text-amber-300", sql.slice(i, j));
      i = j;
      continue;
    }
    if (/[A-Za-z_]/.test(ch)) {
      let j = i + 1;
      while (j < sql.length && /[A-Za-z_]/.test(sql[j])) j += 1;
      const word = sql.slice(i, j);
      if (sqlKeywords.has(word.toLowerCase())) {
        out += wrapSQLToken("font-semibold text-sky-300", word);
      } else {
        out += escapeHTML(word);
      }
      i = j;
      continue;
    }
    out += escapeHTML(ch);
    i += 1;
  }
  return out;
};

const eventShapePreviewHTML = (event: TraceEvent) => highlightSQL(eventShapePreview(event));

const eventExtraFields = (event: TraceEvent): Array<[string, string]> => {
  const omit = new Set(["cache", "operation", "driver", "hit", "duration_ms", "duration_ns", "queue", "job_name", "kind", "attempt", "scheduled", "connection", "target", "fingerprint", "shape", "source"]);
  return Object.entries(event.attributes || {})
    .filter(([key, value]) => !omit.has(key) && value !== undefined && value !== null && `${value}` !== "")
    .map(([key, value]) => [key, typeof value === "string" ? value : JSON.stringify(value)]);
};

const pair = (key: string, value: string, className?: string): InlineField | null => {
  const trimmed = String(value || "").trim();
  if (!trimmed) return null;
  return {
    key,
    label: key,
    value: trimmed,
    valueClassName: className || genericValueClass(trimmed),
  };
};

const statusBadgeVariant = (status?: string) => {
  switch ((status || "").toLowerCase()) {
    case "ok":
      return "secondary";
    case "error":
      return "destructive";
    case "warning":
      return "outline";
    default:
      return "outline";
  }
};

const traceRowClass = (trace: TraceSummary) => {
  const selected = trace.trace_id === selectedTraceId.value;
  const base = selected
    ? "bg-accent/70 ring-2 ring-primary/70 ring-offset-0 shadow-[inset_0_0_0_1px_rgba(255,255,255,0.08)]"
    : "bg-muted/10 hover:bg-accent/20";
  switch ((trace.status || "").toLowerCase()) {
    case "ok":
      return `${base} border-emerald-400/60 ring-1 ring-emerald-500/30`;
    case "error":
      return `${base} border-destructive/60 ring-1 ring-destructive/30`;
    case "warning":
      return `${base} border-amber-400/60 ring-1 ring-amber-500/30`;
    default:
      return `${base} border-border/80 ring-1 ring-white/5`;
  }
};

watch(requestExchange, (value) => {
  if (!value && activeInspectTab.value !== "timeline") {
    activeInspectTab.value = "timeline";
  }
});

watch([sourceFilter, showInternal, inspectSource], async () => {
  await refresh();
});

watch(
  () => [route.path, route.query.trace],
  async () => {
    const routeTraceID = readRouteTraceID();
    if (routeTraceID && routeTraceID !== selectedTraceId.value) {
      selectedTraceId.value = routeTraceID;
      await loadSelectedTrace();
      return;
    }
    if (!routeTraceID) {
      await refresh();
    }
  }
);

onMounted(async () => {
  selectedTraceId.value = readRouteTraceID();
  await refresh();
});
</script>
