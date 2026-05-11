<template>
  <div class="h-[calc(100vh-6rem)] overflow-hidden">
    <section class="grid h-full min-h-0 gap-4 xl:grid-cols-[31rem_minmax(0,1fr)]">
      <Card class="card-texture flex min-h-0 flex-col overflow-hidden border-border/70 bg-card/95">
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
        <CardContent class="flex min-h-0 flex-1 flex-col gap-2.5 overflow-hidden pb-3">
          <div class="grid gap-2.5">
            <div class="relative">
              <Input v-model="query" :placeholder="searchPlaceholder" class="h-8 rounded-lg border-border/70 pr-10 text-[12px]" />
              <span class="pointer-events-none absolute inset-y-0 right-3 inline-flex items-center text-muted">/</span>
            </div>
            <FormField v-if="showSourceFilter" label="Source">
              <Select v-model="sourceFilterModel">
                <SelectTrigger class="w-full border-border/70">
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
            <div class="grid gap-2.5 sm:grid-cols-2">
              <FormField label="Status">
                <Select v-model="statusFilterModel">
                  <SelectTrigger class="w-full border-border/70">
                    <SelectValue placeholder="All statuses" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem :value="allStatusValue">All statuses</SelectItem>
                    <SelectItem v-for="status in statusOptions" :key="status" :value="status">
                      {{ status }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
              <FormField label="Window">
                <Select v-model="timeWindowModel">
                  <SelectTrigger class="w-full border-border/70">
                    <SelectValue placeholder="Any time" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem :value="allTimeValue">Any time</SelectItem>
                    <SelectItem value="5m">Last 5m</SelectItem>
                    <SelectItem value="15m">Last 15m</SelectItem>
                    <SelectItem value="1h">Last 1h</SelectItem>
                    <SelectItem value="6h">Last 6h</SelectItem>
                    <SelectItem value="24h">Last 24h</SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
            </div>
            <div class="flex items-center justify-between rounded-xl border border-border/60 bg-muted/10 px-4 py-2.5">
              <div>
                <p class="text-[11px] font-medium text-foreground">Show internal inspects</p>
                <p class="text-[10px] text-muted">Include Lighthouse API and websocket requests.</p>
              </div>
              <Switch v-model="showInternal" aria-label="Show internal inspects" />
            </div>
          </div>

          <div class="flex items-center justify-between text-[10px] text-muted">
            <span>{{ filteredInspects.length }} requests</span>
            <span v-if="!showInternal">{{ internalInspectCount }} internal hidden</span>
          </div>

          <div
            ref="inspectListRef"
            class="min-h-0 flex-1 overflow-y-auto outline-none"
            tabindex="0"
            role="listbox"
            aria-label="Inspect list"
            @keydown="handleInspectListKeydown"
          >
            <div class="grid grid-cols-[3.55rem_minmax(0,1fr)_3.25rem_3.25rem_2.15rem] gap-2 border-b border-border/60 px-3 py-1 text-[9px] font-semibold uppercase tracking-[0.1em] text-muted">
              <span>Method</span>
              <span>Path</span>
              <span class="text-right">Duration</span>
              <span class="text-right">Time</span>
              <span class="text-right"></span>
            </div>
            <button
              v-for="inspect in filteredInspects"
              :key="inspect.trace_id"
              type="button"
              :ref="(el) => setInspectRowRef(inspect.trace_id, el)"
              role="option"
              :aria-selected="inspect.trace_id === selectedInspectId"
              class="relative grid w-full grid-cols-[3.55rem_minmax(0,1fr)_3.25rem_3.25rem_2.15rem] items-center gap-2 border-b border-border/50 px-3 py-1 text-left transition outline-none focus:outline-none focus-visible:outline-none focus-visible:ring-0 focus-visible:ring-offset-0"
              :class="inspectRowClass(inspect)"
              @click="selectInspect(inspect.trace_id)"
            >
              <span
                class="absolute inset-y-1 left-0 w-0.5 rounded-full"
                :class="{
                  'bg-emerald-400': String(inspect.status || '').toLowerCase() === 'ok',
                  'bg-amber-400': String(inspect.status || '').toLowerCase() === 'warning',
                  'bg-rose-400': String(inspect.status || '').toLowerCase() === 'error',
                  'bg-border': !['ok', 'warning', 'error'].includes(String(inspect.status || '').toLowerCase()),
                }"
              />
              <div class="flex items-center">
                <span
                  class="inline-flex h-5 min-w-[2.7rem] items-center justify-center rounded-md border px-1.5 text-[10px] font-semibold leading-none"
                  :class="methodPillClass(inspectMethod(inspect) || inspect.source)"
                >
                  {{ inspectMethod(inspect) || (inspect.source || "app").toUpperCase() }}
                </span>
              </div>
              <div class="min-w-0">
                <p class="block truncate text-[11px] font-medium leading-tight text-foreground" :title="inspectDisplayName(inspect)">
                  {{ inspectDisplayName(inspect) || inspect.name || inspect.trace_id }}
                </p>
              </div>
              <div class="text-right text-[10px] font-semibold tabular-nums whitespace-nowrap" :class="durationClass(inspect.duration_ms)">
                {{ formatDuration(inspect.duration_ms) }}
              </div>
              <div class="text-right text-[10px] text-muted tabular-nums whitespace-nowrap">
                {{ formatTimeAgo(inspect.started_at) || "now" }}
              </div>
              <div class="flex justify-end">
                <span class="inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full border border-violet-500/25 bg-violet-500/8 px-1 text-[9px] text-violet-200">
                  {{ inspect.event_count }}
                </span>
              </div>
            </button>
            <div v-if="filteredInspects.length === 0" class="rounded-2xl border border-dashed border-border/60 px-4 py-8 text-sm text-muted">
              No inspects matched the current filters.
            </div>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture flex min-h-0 flex-col overflow-hidden border-border/70 bg-card/95">
        <CardHeader class="pb-2.5">
          <template #title>
            <CardTitle class="flex flex-wrap items-center gap-3 text-[clamp(1.2rem,2vw,1.65rem)] leading-tight">
              <span
                v-if="selectedInspectRecord && requestExchange?.method"
                class="text-[1.05rem] font-semibold"
                :class="methodTextClass(requestExchange.method)"
              >
                {{ requestExchange.method }}
              </span>
              <span>{{ selectedInspectRecord ? inspectDisplayName(selectedInspectRecord.summary) : `${inspectTitle} detail` }}</span>
            </CardTitle>
          </template>
          <template #action>
            <div v-if="selectedInspectRecord" class="flex flex-wrap items-center gap-2">
              <button
                type="button"
                class="inline-flex h-9 items-center gap-2 rounded-lg border border-border/70 bg-muted/40 px-3 text-sm font-medium text-foreground transition hover:bg-muted"
                @click="copyCurl"
              >
                <Copy class="h-4 w-4" />
                Copy as cURL
              </button>
              <span class="inline-flex h-8 items-center rounded-full border border-border/60 bg-background/30 px-3 text-[11px] font-medium text-foreground">
                {{ selectedInspectRecord.summary.source || "app" }}
              </span>
              <span
                class="inline-flex h-8 items-center rounded-full px-3 text-[11px] font-medium"
                :class="requestStatusCode > 0 ? statusPillClass(requestStatusCode) : 'border-border/60 bg-background/30 text-foreground'"
              >
                {{ requestStatusCode > 0 ? `${requestStatusCode} OK` : (selectedInspectRecord.summary.status || "running") }}
              </span>
            </div>
          </template>
          <template #description>
            <CardDescription v-if="!selectedInspectRecord">Select an inspect to view its event timeline.</CardDescription>
          </template>
        </CardHeader>
        <CardContent class="min-h-0 flex-1 overflow-hidden">
          <div v-if="selectedInspectRecord" class="flex h-full min-h-0 flex-col gap-4">
            <section class="flex flex-wrap items-center gap-2 text-[11px]">
              <button
                type="button"
                class="inline-flex max-w-full items-center gap-1.5 rounded-full border px-3 py-1 font-mono text-[11px] transition hover:bg-background/80 hover:text-foreground"
                :class="inspectIDPillClass"
                @click="copyInspectID(selectedInspectRecord.summary.trace_id)"
              >
                <span class="break-all text-left">{{ selectedInspectRecord.summary.trace_id }}</span>
                <Copy class="h-3 w-3" />
              </button>
              <span
                v-if="requestHostname"
                :class="['inline-flex items-center gap-1.5 rounded-full border px-3 py-1', valuePillClass]"
              >
                {{ requestHostname }}
              </span>
              <span
                v-if="requestExchange?.method"
                :class="['inline-flex items-center gap-1.5 rounded-full border px-3 py-1', methodValuePillClass]"
              >
                {{ requestExchange.method }}
              </span>
              <span
                v-if="requestStatusCode > 0"
                :class="['inline-flex items-center gap-1.5 rounded-full border px-3 py-1', statusPillClass(requestStatusCode)]"
              >
                <span class="font-medium">{{ requestStatusCode }} OK</span>
              </span>
              <span :class="['inline-flex items-center gap-1.5 rounded-full border px-3 py-1', startedPillClass]">
                started <span class="opacity-60">•</span> <span class="font-medium">{{ formatDateTime(selectedInspectRecord.summary.started_at) }}</span><span class="opacity-70">({{ formatTimeAgo(selectedInspectRecord.summary.started_at) }})</span>
              </span>
              <span :class="['inline-flex items-center gap-1.5 rounded-full border px-3 py-1', durationPillClass(selectedInspectRecord.summary.duration_ms)]">
                duration <span class="opacity-60">•</span> <span class="font-medium">{{ formatDuration(selectedInspectRecord.summary.duration_ms) }}</span>
              </span>
              <span :class="['inline-flex items-center gap-1.5 rounded-full border px-3 py-1', eventsPillClass]">
                events <span class="opacity-60">•</span> <span class="font-medium">{{ selectedInspectRecord.events.length }}</span>
              </span>
              <span v-if="requestHostname" :class="['inline-flex items-center gap-1.5 rounded-full border px-3 py-1', hostPillClass]">
                host <span class="opacity-60">•</span> <span class="font-medium">{{ requestHostname }}</span>
              </span>
              <span
                v-if="requestExchange?.method"
                :class="['inline-flex items-center gap-1.5 rounded-full border px-3 py-1', labelPillClass('method')]"
              >
                method <span class="opacity-60">•</span> <span class="font-medium">{{ requestExchange.method }}</span>
              </span>
              <span
                v-if="requestPathOnly"
                :class="['inline-flex items-center gap-1.5 rounded-full border px-3 py-1', labelPillClass('path')]"
              >
                path <span class="opacity-60">•</span> <span class="font-medium">{{ requestPathOnly }}</span>
              </span>
              <span
                v-if="inspectStatusCode(selectedInspectRecord.summary)"
                :class="['inline-flex items-center gap-1.5 rounded-full border px-3 py-1', labelPillClass('status_code')]"
              >
                status_code <span class="opacity-60">•</span> <span class="font-medium">{{ inspectStatusCode(selectedInspectRecord.summary) }}</span>
              </span>
            </section>

            <Tabs v-if="requestExchange" v-model="activeInspectTab" class="min-h-0 flex-1 gap-3">
              <div class="flex items-center justify-between gap-3">
                <TabsList class="w-fit rounded-2xl border border-border/60 bg-muted/40 p-1">
                  <TabsTrigger value="timeline" class="inline-flex items-center gap-1.5">
                    <Workflow class="h-3.5 w-3.5" />
                    Timeline
                  </TabsTrigger>
                  <TabsTrigger value="request" class="inline-flex items-center gap-1.5">
                    <ClipboardList class="h-3.5 w-3.5" />
                    Request
                  </TabsTrigger>
                  <TabsTrigger value="response" class="inline-flex items-center gap-1.5">
                    <ScrollText class="h-3.5 w-3.5" />
                    Response
                  </TabsTrigger>
                </TabsList>
                <div class="w-[11.5rem]"></div>
              </div>
              <TabsContent value="timeline" class="min-h-0 mt-0 pt-1">
                <div class="max-h-full overflow-y-auto rounded-2xl border border-border/60 bg-muted/10">
                  <div class="border-b border-border/60 px-4 py-2.5">
                    <div class="flex items-center justify-between gap-3">
                      <p class="text-xs font-medium text-foreground">Event timeline</p>
                      <p class="text-[11px] text-muted">ordered capture</p>
                    </div>
                  </div>
                  <div v-if="timelineEvents.length === 0" class="px-4 py-10">
                    <div class="rounded-xl border border-dashed border-border/60 bg-background/40 px-4 py-8 text-center">
                      <p class="text-sm font-medium text-foreground">No events found</p>
                      <p class="mt-1 text-[11px] text-muted">This inspect does not have any captured timeline events yet.</p>
                    </div>
                  </div>
                  <div v-else class="relative divide-y divide-border/60">
                    <div class="pointer-events-none absolute bottom-0 left-[7rem] top-0 hidden w-px bg-border/70 lg:block" />
                    <div
                      v-for="event in timelineEvents"
                      :key="`${event.seq}-${event.at}`"
                      class="relative grid items-center gap-1.5 px-4 py-0.5 lg:grid-cols-[6.4rem_minmax(0,1fr)]"
                    >
                      <span class="absolute left-[6.8rem] top-1/2 z-10 hidden h-2.5 w-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border border-zinc-500/80 bg-zinc-300 shadow-[0_0_0_2px_rgba(23,23,26,0.96)] lg:block" />
                      <div class="relative pr-3 text-[11px] text-muted">
                        <p class="flex h-7 items-center whitespace-nowrap font-medium">
                          <span :class="inspectOffsetClass(selectedInspectRecord.summary.started_at, event.at)">
                            {{ formatInspectOffset(selectedInspectRecord.summary.started_at, event.at) }}
                          </span>
                          <span class="ml-1 tabular-nums text-muted">{{ formatTime(event.at) }}</span>
                        </p>
                      </div>
                      <div class="min-w-0 space-y-1">
                        <div class="pb-0.5">
                          <div class="flex min-w-0 items-center gap-1.5 text-[11px] leading-none">
                            <span
                              class="inline-flex h-6 shrink-0 items-center gap-1.25 rounded-full border px-2 text-[10px] font-medium capitalize"
                              :class="eventKindPillClass(event.kind)"
                            >
                              <component :is="eventKindIcon(event.kind)" class="h-3.25 w-3.25" />
                              {{ event.kind }}
                            </span>
                            <Badge v-if="event.level" class="shrink-0 self-center" variant="secondary">{{ event.level }}</Badge>
                            <Badge v-if="event.status" class="shrink-0 self-center" :variant="statusBadgeVariant(event.status)">{{ event.status }}</Badge>
                            <div class="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
                              <p class="shrink-0 text-[13px] font-semibold leading-none text-foreground">{{ eventHeadline(event) }}</p>
                              <span
                                v-if="eventInlineSummary(event)"
                                class="truncate text-[11px] text-muted"
                              >
                                {{ eventInlineSummary(event) }}
                              </span>
                            </div>
                            <span
                              v-if="eventInlineDuration(event)"
                              class="shrink-0 text-[11px]"
                              :class="eventInlineDurationClass(event)"
                            >
                              {{ eventInlineDuration(event) }}
                            </span>
                          </div>
                        </div>
                        <div
                          v-if="eventSummaryLine(event)"
                          class="text-[11px] text-muted"
                        >
                          {{ eventSummaryLine(event) }}
                        </div>
                        <div
                          v-if="eventShapePreview(event)"
                          class="space-y-1.5"
                        >
                          <div class="group/query relative rounded-md border border-border/50 bg-background/80 px-2.5 py-1.5">
                            <div
                              v-if="event.kind === 'query'"
                              class="absolute right-2 top-1 flex items-center gap-1 opacity-0 transition group-hover/query:opacity-100 focus-within:opacity-100"
                            >
                              <button
                                type="button"
                                class="inline-flex h-6 items-center gap-1 rounded-md border border-border/50 bg-background/90 px-2 text-[11px] text-muted shadow-sm backdrop-blur transition hover:bg-background hover:text-foreground"
                                @click="copyRawQuery(event)"
                              >
                                <Copy class="h-3 w-3" />
                                Raw
                              </button>
                              <button
                                type="button"
                                class="inline-flex h-6 items-center gap-1 rounded-md border border-border/50 bg-background/90 px-2 text-[11px] text-muted shadow-sm backdrop-blur transition hover:bg-background hover:text-foreground"
                                @click="copyQueryShape(event)"
                              >
                                <Copy class="h-3 w-3" />
                                Normalized
                              </button>
                            </div>
                            <pre class="whitespace-pre-wrap break-words text-[11px] leading-5 text-muted"><code v-html="eventShapePreviewHTML(event)"></code></pre>
                          </div>
                        </div>
                        <div v-if="showEventExtraFields(event)" class="rounded-lg border border-border/50 bg-background/60 px-2.5 py-1.5">
                          <div
                            v-if="isSingleErrorField(event)"
                            class="text-[11px] leading-5 text-slate-200"
                            :title="singleErrorFieldValue(event)"
                          >
                            {{ singleErrorFieldValue(event) }}
                          </div>
                          <dl class="grid gap-x-4 gap-y-1 text-[11px] sm:grid-cols-2 xl:grid-cols-3">
                            <template v-if="!isSingleErrorField(event)">
                            <template v-for="[key, value] in eventExtraFields(event)" :key="`${event.seq}-extra-${key}`">
                              <dt class="text-muted">{{ key }}</dt>
                              <dd class="break-words whitespace-pre-wrap" :class="genericValueClass(value)" :title="value">{{ value }}</dd>
                            </template>
                            </template>
                          </dl>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </TabsContent>
              <TabsContent value="request" class="min-h-0 flex-1 mt-0 pt-1">
                <div class="h-full space-y-4 overflow-y-auto rounded-2xl border border-border/60 bg-muted/10 p-5">
                  <section class="rounded-2xl border border-border/60 bg-background/40 px-5 py-4">
                    <div class="flex flex-wrap items-start justify-between gap-4">
                      <div>
                        <p class="text-[11px] font-semibold uppercase tracking-[0.14em] text-muted">Request</p>
                        <div class="mt-3 flex flex-wrap items-center gap-3">
                          <span :class="['inline-flex rounded-lg border px-2.5 py-1 text-sm font-semibold leading-none', methodPillClass(requestExchange?.method)]">
                            {{ requestExchange?.method || "GET" }}
                          </span>
                          <p class="text-xl font-semibold leading-tight text-foreground">
                            {{ requestPathOnly || "/" }}
                          </p>
                        </div>
                        <div class="mt-3 flex items-center gap-2 text-xs text-sky-300">
                          <Link2 class="h-3.5 w-3.5 shrink-0 opacity-80" />
                          <span class="break-all">{{ requestURL }}</span>
                        </div>
                      </div>
                      <div class="flex items-center gap-3 text-[12px] text-muted">
                        <span class="text-emerald-300">{{ requestDurationDisplay }}</span>
                        <span>•</span>
                        <span>{{ requestApproxBytesLabel }}</span>
                        <span>•</span>
                        <span>{{ formatTimeAgo(selectedInspectRecord.summary.started_at) || "just now" }}</span>
                      </div>
                    </div>
                  </section>
                  <section class="overflow-hidden rounded-2xl border border-border/60 bg-background/40">
                    <div class="flex items-center justify-between gap-3 border-b border-border/60 px-4 py-2.5">
                      <button
                        type="button"
                        class="inline-flex items-center gap-2 text-left text-sm font-medium text-foreground transition hover:text-foreground/80"
                        @click="requestHeadersOpen = !requestHeadersOpen"
                      >
                        <ChevronDown class="h-3.5 w-3.5 transition-transform" :class="requestHeadersOpen ? 'rotate-0' : '-rotate-90'" />
                        <span>Headers</span>
                        <span class="inline-flex min-w-6 items-center justify-center rounded-md bg-muted/60 px-1.5 py-0.5 text-[11px] text-muted">{{ requestHeaderCount }}</span>
                      </button>
                      <button
                        v-if="requestHeaderCount > 0"
                        type="button"
                        class="inline-flex h-6 items-center gap-1 rounded-md border border-border/60 bg-background/50 px-2 text-[11px] font-medium text-foreground transition hover:bg-background/80"
                        @click="copyHeadersBlock('Request headers', requestHeaderEntries)"
                      >
                        <Copy class="h-3 w-3" />
                        Copy all
                      </button>
                    </div>
                    <div v-if="requestHeadersOpen">
                      <div v-if="requestHeaderCount > 0">
                        <div class="grid grid-cols-[14rem_minmax(0,1fr)_2rem] border-b border-border/60 px-4 py-1.5 text-[11px] text-muted">
                          <span>Header</span>
                          <span class="pl-2">Value</span>
                          <span></span>
                        </div>
                        <div
                          v-for="[key, value] in requestHeaderEntries"
                          :key="`request-${key}`"
                          class="grid grid-cols-[14rem_minmax(0,1fr)_2rem] items-start gap-3 border-b border-border/60 px-4 py-1.5 last:border-b-0"
                        >
                          <div class="text-[13px] font-medium leading-5 text-slate-100">{{ key }}</div>
                          <div class="min-w-0 break-words whitespace-pre-wrap font-mono text-[12px] leading-5 text-slate-200" :title="value">{{ value }}</div>
                          <button
                            type="button"
                            class="mt-0.5 inline-flex h-6 w-6 items-center justify-center rounded-md border border-border/50 bg-background/40 text-muted transition hover:bg-background/70 hover:text-foreground"
                            :aria-label="`Copy header ${key}`"
                            @click="copyHeaderValue(key, value)"
                          >
                            <Copy class="h-3 w-3" />
                          </button>
                        </div>
                      </div>
                      <div v-else class="px-5 py-6 text-sm text-muted">No request headers</div>
                    </div>
                  </section>
                  <section class="rounded-2xl border border-border/60 bg-background/40 p-4">
                    <div class="mb-4 flex items-center justify-between gap-3">
                      <button
                        type="button"
                        class="inline-flex items-center gap-2 text-left text-sm font-medium text-foreground transition hover:text-foreground/80"
                        @click="requestBodyOpen = !requestBodyOpen"
                      >
                        <ChevronDown class="h-3.5 w-3.5 transition-transform" :class="requestBodyOpen ? 'rotate-0' : '-rotate-90'" />
                        <span>Body</span>
                        <span class="inline-flex items-center justify-center rounded-md bg-muted/60 px-1.5 py-0.5 text-[11px] text-muted">{{ requestBodyKindLabel }}</span>
                      </button>
                      <div class="flex items-center gap-2">
                        <button
                          type="button"
                          class="inline-flex h-6 items-center gap-1 rounded-md border border-border/60 bg-background/50 px-2 text-[11px] font-medium text-foreground transition hover:bg-background/80"
                          @click="copyBody('Request body', requestBodyRaw)"
                        >
                          <Copy class="h-3 w-3" />
                          Copy raw
                        </button>
                        <button
                          v-if="requestBodyIsJSON"
                          type="button"
                          class="inline-flex h-6 items-center gap-1 rounded-md border border-border/60 bg-background/50 px-2 text-[11px] font-medium text-foreground transition hover:bg-background/80"
                          @click="copyBody('Request body', requestBodyPretty)"
                        >
                          <Copy class="h-3 w-3" />
                          Copy pretty
                        </button>
                      </div>
                    </div>
                    <div v-if="requestBodyOpen">
                      <div
                        v-if="requestBodyIsEmpty"
                        class="flex min-h-40 items-center justify-center rounded-[1.1rem] border border-border/50 bg-muted/10 px-6 py-8 text-center"
                      >
                        <div class="flex items-center gap-4 text-muted">
                          <ScrollText class="h-12 w-12 opacity-70" />
                          <div class="text-left">
                            <p class="text-2xl font-medium text-slate-300">No request body</p>
                          </div>
                        </div>
                      </div>
                      <div v-else class="overflow-x-auto rounded-[1.1rem] border border-border/50 bg-muted/10 px-4 py-3">
                        <pre class="whitespace-pre text-[12px] leading-6 text-slate-200"><code v-html="requestBodyDisplayHTML"></code></pre>
                      </div>
                    </div>
                  </section>
                </div>
              </TabsContent>
              <TabsContent value="response" class="min-h-0 flex-1 mt-0 pt-1">
                <div class="h-full space-y-4 overflow-y-auto rounded-2xl border border-border/60 bg-muted/10 p-5">
                  <section class="rounded-2xl border border-border/60 bg-background/40 px-5 py-4">
                    <div class="flex flex-wrap items-start justify-between gap-4">
                      <div>
                        <p class="text-[11px] font-semibold uppercase tracking-[0.14em] text-muted">Response</p>
                        <div class="mt-3 flex flex-wrap items-center gap-3">
                          <span :class="['inline-flex rounded-lg border px-2.5 py-1 text-sm font-semibold leading-none', statusPillClass(requestStatusCode)]">
                            {{ requestStatusCode || "?" }}
                          </span>
                          <p class="text-xl font-semibold leading-tight text-foreground">
                            {{ responseStatusLine }}
                          </p>
                        </div>
                        <div class="mt-3 flex items-center gap-2 text-xs text-sky-300">
                          <ScrollText class="h-3.5 w-3.5 shrink-0 opacity-80" />
                          <span class="break-all">{{ responseContentType }}</span>
                        </div>
                      </div>
                      <div class="flex items-center gap-3 text-[12px] text-muted">
                        <span class="text-emerald-300">{{ requestDurationDisplay }}</span>
                        <span>•</span>
                        <span>{{ responseApproxBytesLabel }}</span>
                        <span>•</span>
                        <span>{{ formatTimeAgo(selectedInspectRecord.summary.started_at) || "just now" }}</span>
                      </div>
                    </div>
                  </section>
                  <section class="overflow-hidden rounded-2xl border border-border/60 bg-background/40">
                    <div class="flex items-center justify-between gap-3 border-b border-border/60 px-4 py-2.5">
                      <button
                        type="button"
                        class="inline-flex items-center gap-2 text-left text-sm font-medium text-foreground transition hover:text-foreground/80"
                        @click="responseHeadersOpen = !responseHeadersOpen"
                      >
                        <ChevronDown class="h-3.5 w-3.5 transition-transform" :class="responseHeadersOpen ? 'rotate-0' : '-rotate-90'" />
                        <span>Headers</span>
                        <span class="inline-flex min-w-6 items-center justify-center rounded-md bg-muted/60 px-1.5 py-0.5 text-[11px] text-muted">{{ responseHeaderCount }}</span>
                      </button>
                      <button
                        v-if="responseHeaderCount > 0"
                        type="button"
                        class="inline-flex h-6 items-center gap-1 rounded-md border border-border/60 bg-background/50 px-2 text-[11px] font-medium text-foreground transition hover:bg-background/80"
                        @click="copyHeadersBlock('Response headers', responseHeaderEntries)"
                      >
                        <Copy class="h-3 w-3" />
                        Copy all
                      </button>
                    </div>
                    <div v-if="responseHeadersOpen">
                      <div v-if="responseHeaderCount > 0">
                        <div class="grid grid-cols-[14rem_minmax(0,1fr)_2rem] border-b border-border/60 px-4 py-1.5 text-[11px] text-muted">
                          <span>Header</span>
                          <span class="pl-2">Value</span>
                          <span></span>
                        </div>
                        <div
                          v-for="[key, value] in responseHeaderEntries"
                          :key="`response-${key}`"
                          class="grid grid-cols-[14rem_minmax(0,1fr)_2rem] items-start gap-3 border-b border-border/60 px-4 py-1.5 last:border-b-0"
                        >
                          <div class="text-[13px] font-medium leading-5 text-slate-100">{{ key }}</div>
                          <div class="min-w-0 break-words whitespace-pre-wrap font-mono text-[12px] leading-5 text-slate-200" :title="value">{{ value }}</div>
                          <button
                            type="button"
                            class="mt-0.5 inline-flex h-6 w-6 items-center justify-center rounded-md border border-border/50 bg-background/40 text-muted transition hover:bg-background/70 hover:text-foreground"
                            :aria-label="`Copy header ${key}`"
                            @click="copyHeaderValue(key, value)"
                          >
                            <Copy class="h-3 w-3" />
                          </button>
                        </div>
                      </div>
                      <div v-else class="px-5 py-6 text-sm text-muted">No response headers</div>
                    </div>
                  </section>
                  <section class="rounded-2xl border border-border/60 bg-background/40 p-4">
                    <div class="mb-4 flex items-center justify-between gap-3">
                      <button
                        type="button"
                        class="inline-flex items-center gap-2 text-left text-sm font-medium text-foreground transition hover:text-foreground/80"
                        @click="responseBodyOpen = !responseBodyOpen"
                      >
                        <ChevronDown class="h-3.5 w-3.5 transition-transform" :class="responseBodyOpen ? 'rotate-0' : '-rotate-90'" />
                        <span>Body</span>
                        <span class="inline-flex items-center justify-center rounded-md bg-muted/60 px-1.5 py-0.5 text-[11px] text-muted">{{ responseBodyKindLabel }}</span>
                      </button>
                      <div class="flex items-center gap-2">
                        <button
                          type="button"
                          class="inline-flex h-6 items-center gap-1 rounded-md border border-border/60 bg-background/50 px-2 text-[11px] font-medium text-foreground transition hover:bg-background/80"
                          @click="copyBody('Response body', responseBodyRaw)"
                        >
                          <Copy class="h-3 w-3" />
                          Copy raw
                        </button>
                        <button
                          v-if="responseBodyIsJSON"
                          type="button"
                          class="inline-flex h-6 items-center gap-1 rounded-md border border-border/60 bg-background/50 px-2 text-[11px] font-medium text-foreground transition hover:bg-background/80"
                          @click="copyBody('Response body', responseBodyPretty)"
                        >
                          <Copy class="h-3 w-3" />
                          Copy pretty
                        </button>
                      </div>
                    </div>
                    <div v-if="responseBodyOpen">
                      <div
                        v-if="responseBodyIsEmpty"
                        class="flex min-h-40 items-center justify-center rounded-[1.1rem] border border-border/50 bg-muted/10 px-6 py-8 text-center"
                      >
                        <div class="flex items-center gap-4 text-muted">
                          <ScrollText class="h-12 w-12 opacity-70" />
                          <div class="text-left">
                            <p class="text-2xl font-medium text-slate-300">No response body</p>
                          </div>
                        </div>
                      </div>
                      <div v-else class="overflow-x-auto rounded-[1.1rem] border border-border/50 bg-muted/10 px-4 py-3">
                        <pre class="whitespace-pre text-[12px] leading-6 text-slate-200"><code v-html="responseBodyDisplayHTML"></code></pre>
                      </div>
                    </div>
                  </section>
                </div>
              </TabsContent>
            </Tabs>
            <div v-else class="max-h-full overflow-y-auto rounded-2xl border border-border/60 bg-muted/10">
              <div class="border-b border-border/60 px-4 py-2.5">
                <div class="flex items-center justify-between gap-3">
                  <p class="text-xs font-medium text-foreground">Event timeline</p>
                  <p class="text-[11px] text-muted">ordered capture</p>
                </div>
              </div>
              <div v-if="timelineEvents.length === 0" class="px-4 py-10">
                <div class="rounded-xl border border-dashed border-border/60 bg-background/40 px-4 py-8 text-center">
                  <p class="text-sm font-medium text-foreground">No events found</p>
                  <p class="mt-1 text-[11px] text-muted">This inspect does not have any captured timeline events yet.</p>
                </div>
              </div>
              <div v-else class="relative divide-y divide-border/60">
                <div class="pointer-events-none absolute bottom-0 left-[7rem] top-0 hidden w-px bg-border/70 lg:block" />
                <div
                  v-for="event in timelineEvents"
                  :key="`${event.seq}-${event.at}`"
                  class="relative grid items-center gap-1.5 px-4 py-1 lg:grid-cols-[6.6rem_minmax(0,1fr)]"
                >
                  <span class="absolute left-[7rem] top-1/2 z-10 hidden h-2.5 w-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border border-zinc-500/80 bg-zinc-300 shadow-[0_0_0_2px_rgba(23,23,26,0.96)] lg:block" />
                  <div class="relative pr-3 text-[11px] text-muted">
                    <p class="flex h-8 items-center whitespace-nowrap font-medium">
                      <span :class="inspectOffsetClass(selectedInspectRecord.summary.started_at, event.at)">
                        {{ formatInspectOffset(selectedInspectRecord.summary.started_at, event.at) }}
                      </span>
                      <span class="ml-1 tabular-nums text-muted">{{ formatTime(event.at) }}</span>
                    </p>
                  </div>
                  <div class="min-w-0 space-y-1">
                    <div class="pb-0.5">
                      <div class="flex min-w-0 items-center gap-1.5 text-[11px] leading-none">
                        <span
                          class="inline-flex h-7 shrink-0 items-center gap-1.25 rounded-full border px-2.25 text-[10px] font-medium capitalize"
                          :class="eventKindPillClass(event.kind)"
                        >
                          <component :is="eventKindIcon(event.kind)" class="h-3.25 w-3.25" />
                          {{ event.kind }}
                        </span>
                        <Badge v-if="event.level" class="shrink-0 self-center" variant="secondary">{{ event.level }}</Badge>
                        <Badge v-if="event.status" class="shrink-0 self-center" :variant="statusBadgeVariant(event.status)">{{ event.status }}</Badge>
                        <div class="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
                          <p class="shrink-0 text-sm font-semibold leading-none text-foreground">{{ eventHeadline(event) }}</p>
                          <span
                            v-if="eventInlineSummary(event)"
                            class="truncate text-[11px] text-muted"
                          >
                            {{ eventInlineSummary(event) }}
                          </span>
                        </div>
                        <span
                          v-if="eventInlineDuration(event)"
                          class="shrink-0 text-[11px]"
                          :class="eventInlineDurationClass(event)"
                        >
                          {{ eventInlineDuration(event) }}
                        </span>
                      </div>
                    </div>
                    <div
                      v-if="eventSummaryLine(event)"
                      class="text-[11px] text-muted"
                    >
                      {{ eventSummaryLine(event) }}
                    </div>
                    <div
                      v-if="eventShapePreview(event)"
                      class="space-y-1.5"
                    >
                      <div class="group/query relative rounded-md border border-border/50 bg-background/80 px-2.5 py-1.5">
                        <div
                          v-if="event.kind === 'query'"
                          class="absolute right-2 top-1 flex items-center gap-1 opacity-0 transition group-hover/query:opacity-100 focus-within:opacity-100"
                        >
                          <button
                            type="button"
                            class="inline-flex h-6 items-center gap-1 rounded-md border border-border/50 bg-background/90 px-2 text-[11px] text-muted shadow-sm backdrop-blur transition hover:bg-background hover:text-foreground"
                            @click="copyRawQuery(event)"
                          >
                            <Copy class="h-3 w-3" />
                            Raw
                          </button>
                          <button
                            type="button"
                            class="inline-flex h-6 items-center gap-1 rounded-md border border-border/50 bg-background/90 px-2 text-[11px] text-muted shadow-sm backdrop-blur transition hover:bg-background hover:text-foreground"
                            @click="copyQueryShape(event)"
                          >
                            <Copy class="h-3 w-3" />
                            Normalized
                          </button>
                        </div>
                        <pre class="whitespace-pre-wrap break-words text-[11px] leading-5 text-muted"><code v-html="eventShapePreviewHTML(event)"></code></pre>
                      </div>
                    </div>
                    <div v-if="showEventExtraFields(event)" class="rounded-lg border border-border/50 bg-background/60 px-2.5 py-1.5">
                      <div
                        v-if="isSingleErrorField(event)"
                        class="text-[11px] leading-5 text-slate-200"
                        :title="singleErrorFieldValue(event)"
                      >
                        {{ singleErrorFieldValue(event) }}
                      </div>
                      <dl class="grid gap-x-4 gap-y-1 text-[11px] sm:grid-cols-2 xl:grid-cols-3">
                        <template v-if="!isSingleErrorField(event)">
                        <template v-for="[key, value] in eventExtraFields(event)" :key="`${event.seq}-extra-${key}`">
                          <dt class="text-muted">{{ key }}</dt>
                              <dd class="break-words whitespace-pre-wrap" :class="genericValueClass(value)" :title="value">{{ value }}</dd>
                        </template>
                        </template>
                      </dl>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="rounded-2xl border border-dashed border-border/60 px-4 py-12 text-sm text-muted">
            {{ loadingSelectedInspect ? "Loading inspect detail..." : "Select an inspect from the list to view its event timeline." }}
          </div>
        </CardContent>
      </Card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Binary, Bot, ChevronDown, ClipboardList, Copy, Database, HardDrive, Link2, Package, Route, ScrollText, Tag, Terminal, TriangleAlert, Workflow } from "lucide-vue-next";
import { useRoute, useRouter } from "vue-router";
import { toast } from "vue-sonner";
import { lighthousePath } from "../lib/base-path";
import { formatJSONDisplay, maybePrettyJSON, renderBodyHTML } from "../lib/json-preview";
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

type InspectSummary = {
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

type InspectEvent = {
  seq: number;
  at: string;
  kind: string;
  level?: string;
  name?: string;
  message?: string;
  status?: string;
  http?: {
    method?: string;
    scheme?: string;
    host?: string;
    uri?: string;
    request_body?: string;
    request_headers_raw?: Array<{ name?: unknown; value?: unknown }> | Record<string, unknown>;
    request_body_raw?: string;
    response_status?: number;
    response_headers?: Array<{ name?: unknown; value?: unknown }> | Record<string, unknown>;
    response_body?: string;
  };
  attributes?: Record<string, unknown>;
};

type InspectRecord = {
  summary: InspectSummary;
  events: InspectEvent[];
};

type HTTPExchange = {
  method: string;
  scheme: string;
  host: string;
  uri: string;
  requestHeaders: Record<string, string>;
  requestBody: string;
  rawRequestHeaders: Record<string, string>;
  rawRequestBody: string;
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
const allStatusValue = "__all_status__";
const allTimeValue = "__all_time__";
const inspectListFetchLimit = 1000;
const focusRefreshCooldownMs = 2_000;
const refreshing = ref(false);
const loadingSelectedInspect = ref(false);
const inspects = ref<InspectSummary[]>([]);
const selectedInspectId = ref("");
const selectedInspectRecord = ref<InspectRecord | null>(null);
const inspectListRef = ref<HTMLElement | null>(null);
const query = ref("");
const sourceFilter = ref("");
const statusFilter = ref("");
const timeWindow = ref("");
const showInternal = ref(false);
const route = useRoute();
const router = useRouter();
const activeInspectTab = ref("timeline");
const inspectTabs = new Set(["timeline", "request", "response"]);
const desiredInspectTab = ref("timeline");
const requestHeadersOpen = ref(true);
const responseHeadersOpen = ref(true);
const requestBodyOpen = ref(true);
const responseBodyOpen = ref(true);
const initialInspectScrollDone = ref(false);
const inspectRowRefs = new Map<string, HTMLElement>();
const lastRefreshAt = ref(0);
let selectedInspectLoadToken = 0;

const asObject = (value: unknown): Record<string, unknown> => {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return value as Record<string, unknown>;
};

const readObjectField = (record: Record<string, unknown>, lower: string, upper = "") => {
  if (record[lower] !== undefined) return record[lower];
  if (upper && record[upper] !== undefined) return record[upper];
  return undefined;
};

const normalizeInspectEvent = (value: unknown): InspectEvent => {
  const raw = asObject(value);
  return {
    seq: Number(readObjectField(raw, "seq", "Seq") || 0),
    at: String(readObjectField(raw, "at", "At") || ""),
    kind: String(readObjectField(raw, "kind", "Kind") || ""),
    level: String(readObjectField(raw, "level", "Level") || ""),
    name: String(readObjectField(raw, "name", "Name") || ""),
    message: String(readObjectField(raw, "message", "Message") || ""),
    status: String(readObjectField(raw, "status", "Status") || ""),
    http: asObject(readObjectField(raw, "http", "HTTP")) as InspectEvent["http"],
    attributes: asObject(readObjectField(raw, "attributes", "Attributes")),
  };
};

const normalizeInspectRecord = (value: unknown): InspectRecord => {
  const raw = asObject(value);
  const summary = asObject(readObjectField(raw, "summary", "Summary"));
  const eventsRaw = readObjectField(raw, "events", "Events");
  const events = Array.isArray(eventsRaw) ? eventsRaw.map(normalizeInspectEvent) : [];
  return {
    summary: {
      trace_id: String(readObjectField(summary, "trace_id", "TraceID") || ""),
      source: String(readObjectField(summary, "source", "Source") || ""),
      name: String(readObjectField(summary, "name", "Name") || ""),
      status: String(readObjectField(summary, "status", "Status") || ""),
      started_at: String(readObjectField(summary, "started_at", "StartedAt") || ""),
      updated_at: String(readObjectField(summary, "updated_at", "UpdatedAt") || ""),
      ended_at: String(readObjectField(summary, "ended_at", "EndedAt") || ""),
      duration_ms: Number(readObjectField(summary, "duration_ms", "DurationMS") || 0),
      event_count: Number(readObjectField(summary, "event_count", "EventCount") || events.length),
      labels: asObject(readObjectField(summary, "labels", "Labels")) as Record<string, string>,
    },
    events,
  };
};

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
  Array.from(new Set(inspects.value.map((inspect) => inspect.source).filter(Boolean))).sort()
);

const showSourceBadgeInList = computed(() => !inspectSource.value);

const statusOptions = computed(() =>
  Array.from(new Set(inspects.value.map((inspect) => String(inspect.status || "").trim().toLowerCase()).filter(Boolean))).sort()
);

const internalInspectCount = computed(() => inspects.value.filter((inspect) => isInternalInspect(inspect)).length);

const statusFilterModel = computed({
  get: () => statusFilter.value || allStatusValue,
  set: (value: string) => {
    statusFilter.value = value === allStatusValue ? "" : value;
  },
});

const timeWindowModel = computed({
  get: () => timeWindow.value || allTimeValue,
  set: (value: string) => {
    timeWindow.value = value === allTimeValue ? "" : value;
  },
});

const filteredInspects = computed(() => {
  const needle = query.value.trim().toLowerCase();
  const now = Date.now();
  const minStartedAt = resolveTimeWindowStart(now, timeWindow.value);
  return inspects.value.filter((inspect) => {
    if (!showInternal.value && isInternalInspect(inspect)) return false;
    if (inspectSource.value && inspect.source !== inspectSource.value) return false;
    if (!inspectSource.value && sourceFilter.value && inspect.source !== sourceFilter.value) return false;
    if (statusFilter.value && String(inspect.status || "").trim().toLowerCase() !== statusFilter.value) return false;
    if (minStartedAt > 0) {
      const startedAt = new Date(inspect.started_at).getTime();
      if (!Number.isFinite(startedAt) || startedAt < minStartedAt) return false;
    }
    if (!needle) return true;
    return inspectSearchFields(inspect).some((field) => field.includes(needle));
  });
});

const setInspectRowRef = (inspectID: string, el: Element | null) => {
  if (el instanceof HTMLElement) {
    inspectRowRefs.set(inspectID, el);
    return;
  }
  inspectRowRefs.delete(inspectID);
};

const sleep = (ms: number) => new Promise((resolve) => window.setTimeout(resolve, ms));

const focusInspectRow = async (inspectID: string) => {
  if (!inspectID) return;
  await nextTick();
  inspectRowRefs.get(inspectID)?.focus();
};

const scrollSelectedInspectIntoView = async (behavior: ScrollBehavior = "smooth") => {
  if (!selectedInspectId.value) return false;
  await nextTick();
  await nextTick();
  const container = inspectListRef.value;
  const target = inspectRowRefs.get(selectedInspectId.value);
  if (!container || !target) return false;
  const containerRect = container.getBoundingClientRect();
  const targetRect = target.getBoundingClientRect();
  const fullyVisible = targetRect.top >= containerRect.top && targetRect.bottom <= containerRect.bottom;
  if (fullyVisible) return true;
  target.scrollIntoView({ block: "nearest", inline: "nearest", behavior });
  return true;
};

const scrollSelectedInspectIntoViewWithRetry = async (behavior: ScrollBehavior = "smooth") => {
  const delays = [0, 50, 120, 250];
  for (const delay of delays) {
    if (delay > 0) {
      await sleep(delay);
    }
    if (await scrollSelectedInspectIntoView(behavior)) {
      return true;
    }
  }
  return false;
};

const selectInspectByIndex = async (index: number) => {
  if (filteredInspects.value.length === 0) return;
  const clampedIndex = Math.max(0, Math.min(index, filteredInspects.value.length - 1));
  const inspectID = filteredInspects.value[clampedIndex]?.trace_id || "";
  if (!inspectID) return;
  await selectInspect(inspectID);
  await focusInspectRow(inspectID);
  await scrollSelectedInspectIntoView("auto");
};

const handleInspectListKeydown = async (event: KeyboardEvent) => {
  const target = event.target as HTMLElement | null;
  const tagName = String(target?.tagName || "").toLowerCase();
  if (target?.isContentEditable || tagName === "input" || tagName === "textarea" || tagName === "select") {
    return;
  }

  const currentIndex = filteredInspects.value.findIndex((inspect) => inspect.trace_id === selectedInspectId.value);
  switch (event.key) {
    case "ArrowDown":
      event.preventDefault();
      await selectInspectByIndex(currentIndex >= 0 ? currentIndex + 1 : 0);
      return;
    case "ArrowUp":
      event.preventDefault();
      await selectInspectByIndex(currentIndex >= 0 ? currentIndex - 1 : filteredInspects.value.length - 1);
      return;
    case "Home":
      event.preventDefault();
      await selectInspectByIndex(0);
      return;
    case "End":
      event.preventDefault();
      await selectInspectByIndex(filteredInspects.value.length - 1);
      return;
  }
};

const normalizeHeaderMap = (value: unknown): Record<string, string> => {
  if (Array.isArray(value)) {
    const out: Record<string, string> = {};
    for (const entry of value) {
      if (!entry || typeof entry !== "object") continue;
      const name = String((entry as Record<string, unknown>).name || "").trim();
      if (!name) continue;
      const raw = (entry as Record<string, unknown>).value;
      if (raw === undefined || raw === null) continue;
      out[name] = typeof raw === "string" ? raw : JSON.stringify(raw);
    }
    return out;
  }
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

const redactInspectHeaderValue = (name: string, value: string): string => {
  const normalized = String(name || "").trim().toLowerCase();
  if (!normalized) return value;
  switch (normalized) {
    case "authorization":
    case "proxy-authorization":
    case "cookie":
    case "set-cookie":
    case "x-api-key":
    case "x-auth-token":
    case "x-csrf-token":
    case "x-forwarded-access-token":
      return "[redacted]";
    default:
      return value;
  }
};

const redactedHeaderMap = (headers: Record<string, string>): Record<string, string> => {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(headers)) {
    out[key] = redactInspectHeaderValue(key, value);
  }
  return out;
};

const selectedInspectEventView = computed(() => {
  if (!selectedInspectRecord.value) {
    return {
      requestExchangeEvent: null as InspectEvent | null,
      requestLogEvent: null as InspectEvent | null,
      timelineEvents: [] as InspectEvent[],
    };
  }
  const sortedEvents = selectedInspectRecord.value.events
    .slice()
    .sort((left, right) => {
      const leftSeq = Number(left.seq || 0);
      const rightSeq = Number(right.seq || 0);
      if (leftSeq !== rightSeq) return leftSeq - rightSeq;
      const leftAt = new Date(left.at || 0).getTime();
      const rightAt = new Date(right.at || 0).getTime();
      if (leftAt !== rightAt) return leftAt - rightAt;
      return String(left.name || left.message || "").localeCompare(String(right.name || right.message || ""));
    });
  let requestExchangeEvent: InspectEvent | null = null;
  let requestLogEvent: InspectEvent | null = null;
  const timelineEvents: InspectEvent[] = [];
  for (const event of sortedEvents) {
    if (!requestExchangeEvent && event.kind === "http" && event.name === "http_exchange") {
      requestExchangeEvent = event;
      continue;
    }
    if (!requestLogEvent && event.kind === "log" && event.message === "HTTP Request") {
      requestLogEvent = event;
    }
    timelineEvents.push(event);
  }
  return { requestExchangeEvent, requestLogEvent, timelineEvents };
});

const requestExchange = computed<HTTPExchange | null>(() => {
  const event = selectedInspectEventView.value.requestExchangeEvent;
  if (!event) return null;
  const http = event.http;
  const rawRequestHeaders = normalizeHeaderMap(http?.request_headers_raw ?? event.attributes?.request_headers_raw);
  const requestHeaders = normalizeHeaderMap(event.attributes?.request_headers);
  const responseHeaders = normalizeHeaderMap(http?.response_headers ?? event.attributes?.response_headers);
  return {
    method: String(http?.method || readAttr(event, "method")),
    scheme: String(http?.scheme || readAttr(event, "scheme")),
    host: String(http?.host || readAttr(event, "host")),
    uri: String(http?.uri || readAttr(event, "uri")),
    requestHeaders: Object.keys(requestHeaders).length > 0 ? requestHeaders : redactedHeaderMap(rawRequestHeaders),
    requestBody: String(http?.request_body || readAttr(event, "request_body")),
    rawRequestHeaders,
    rawRequestBody: String(http?.request_body_raw || readAttr(event, "request_body_raw")),
    responseStatus: Number(http?.response_status ?? event.attributes?.response_status) || 0,
    responseHeaders,
    responseBody: String(http?.response_body || readAttr(event, "response_body")),
  };
});

const requestLogEvent = computed<InspectEvent | null>(() => selectedInspectEventView.value.requestLogEvent);

const normalizeInspectTab = (value: unknown) => {
  const tab = typeof value === "string" ? value.trim().toLowerCase() : "";
  return inspectTabs.has(tab) ? tab : "timeline";
};

const applyDesiredInspectTab = () => {
  const normalized = normalizeInspectTab(desiredInspectTab.value);
  if (!requestExchange.value && normalized !== "timeline") {
    activeInspectTab.value = "timeline";
    syncInspectTabToRoute("timeline");
    return;
  }
  activeInspectTab.value = normalized;
};

const timelineEvents = computed(() => selectedInspectEventView.value.timelineEvents);

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

const requestHostname = computed(() => {
  if (!requestExchange.value) return "";
  const rawHost = String(requestExchange.value.host || "").trim();
  if (!rawHost) return "";
  return rawHost.replace(/:\d+$/, "");
});

const requestPathOnly = computed(() => {
  if (!requestExchange.value) return "";
  try {
    const url = new URL(inspectURL(requestExchange.value));
    return url.pathname || "/";
  } catch {
    const uri = String(requestExchange.value.uri || "").trim();
    if (!uri) return "/";
    const q = uri.indexOf("?");
    return q >= 0 ? uri.slice(0, q) : uri;
  }
});

const requestStatusCode = computed(() => {
  if (requestExchange.value?.responseStatus) return requestExchange.value.responseStatus;
  return Number(requestLogEvent.value?.attributes?.status) || 0;
});

const requestRemoteIP = computed(() => readAttr(requestLogEvent.value, "remote_ip"));

const sortedEntries = (record: Record<string, string>) =>
  Object.entries(record).sort(([left], [right]) => left.localeCompare(right));

const requestHeaderEntries = computed(() =>
  activeInspectTab.value === "request" && requestExchange.value ? sortedEntries(requestExchange.value.requestHeaders) : []
);
const responseHeaderEntries = computed(() =>
  activeInspectTab.value === "response" && requestExchange.value ? sortedEntries(requestExchange.value.responseHeaders) : []
);
const requestHeaderCount = computed(() => requestHeaderEntries.value.length);
const responseHeaderCount = computed(() => responseHeaderEntries.value.length);

const requestBodyRaw = computed(() => {
  if (!requestExchange.value) return "";
  return requestExchange.value.requestBody || "(empty)";
});

const responseBodyRaw = computed(() => {
  if (!requestExchange.value) return "";
  return requestExchange.value.responseBody || "(empty)";
});

const requestBodyPretty = computed(() => (activeInspectTab.value === "request" ? formatJSONDisplay(requestBodyRaw.value) : ""));
const responseBodyPretty = computed(() => (activeInspectTab.value === "response" ? formatJSONDisplay(responseBodyRaw.value) : ""));
const requestBodyIsJSON = computed(() => activeInspectTab.value === "request" && maybePrettyJSON(requestBodyRaw.value) !== null);
const responseBodyIsJSON = computed(() => activeInspectTab.value === "response" && maybePrettyJSON(responseBodyRaw.value) !== null);
const requestBodyIsEmpty = computed(() => requestBodyRaw.value === "(empty)");
const responseBodyIsEmpty = computed(() => responseBodyRaw.value === "(empty)");
const requestBodyKindLabel = computed(() => {
  if (requestBodyIsEmpty.value) return "empty";
  return requestBodyIsJSON.value ? "json" : "text";
});
const responseBodyKindLabel = computed(() => {
  if (responseBodyIsEmpty.value) return "empty";
  return responseBodyIsJSON.value ? "json" : "text";
});

const requestBodyDisplayHTML = computed(() => (activeInspectTab.value === "request" ? renderBodyHTML(requestBodyRaw.value) : ""));
const responseBodyDisplayHTML = computed(() => (activeInspectTab.value === "response" ? renderBodyHTML(responseBodyRaw.value) : ""));

const requestDurationDisplay = computed(() => {
  const durationNs = Number(readAttr(requestLogEvent.value, "latency_ns")) || 0;
  const durationMs = Number(readAttr(requestLogEvent.value, "latency_ms")) || (durationNs > 0 ? durationNs / 1_000_000 : 0);
  return formatDuration(durationMs, durationNs);
});

const requestApproxBytes = computed(() => {
  if (!requestExchange.value) return 0;
  const lines = [`${requestExchange.value.method || "GET"} ${requestExchange.value.uri || "/"}`];
  for (const [key, value] of requestHeaderEntries.value) {
    lines.push(`${key}: ${value}`);
  }
  if (!requestBodyIsEmpty.value) {
    lines.push(requestBodyRaw.value);
  }
  return new TextEncoder().encode(lines.join("\n")).length;
});

const requestApproxBytesLabel = computed(() => {
  const bytes = requestApproxBytes.value;
  if (bytes < 1024) return `${bytes} B`;
  return `${(bytes / 1024).toFixed(1).replace(/\.0$/, "")} KB`;
});

const responseContentType = computed(() => {
  const entry = responseHeaderEntries.value.find(([key]) => key.toLowerCase() === "content-type");
  return entry?.[1] || "unknown content type";
});

const responseApproxBytes = computed(() => {
  if (!requestExchange.value) return 0;
  const lines = [`Status ${requestExchange.value.responseStatus || 0}`];
  for (const [key, value] of responseHeaderEntries.value) {
    lines.push(`${key}: ${value}`);
  }
  if (!responseBodyIsEmpty.value) {
    lines.push(responseBodyRaw.value);
  }
  return new TextEncoder().encode(lines.join("\n")).length;
});

const responseApproxBytesLabel = computed(() => {
  const bytes = responseApproxBytes.value;
  if (bytes < 1024) return `${bytes} B`;
  return `${(bytes / 1024).toFixed(1).replace(/\.0$/, "")} KB`;
});

const responseStatusLine = computed(() => {
  if (!requestExchange.value) return "";
  const code = requestExchange.value.responseStatus;
  return code > 0 ? `Status ${code}` : "Status unknown";
});

const shellEscape = (value: string) => `'${String(value).replaceAll("'", `'\"'\"'`)}'`;

const copyCurl = async () => {
  if (!requestExchange.value) return;
  try {
    const exchange = requestExchange.value;
    const url = inspectURL(exchange);
    const requestHeaders = Object.keys(exchange.rawRequestHeaders).length > 0 ? exchange.rawRequestHeaders : exchange.requestHeaders;
    const requestBody = exchange.rawRequestBody || exchange.requestBody;
    const lines: string[] = ["curl"];
    if (exchange.method && exchange.method.toUpperCase() !== "GET") {
      lines.push(`\t-X ${shellEscape(exchange.method.toUpperCase())}`);
    }
    for (const [key, value] of sortedEntries(requestHeaders)) {
      const lowerKey = key.toLowerCase();
      if (lowerKey === "host" || lowerKey === "content-length") continue;
      lines.push(`\t-H ${shellEscape(`${key}: ${value}`)}`);
    }
    if (requestBody) {
      lines.push(`\t--data-raw ${shellEscape(requestBody)}`);
    }
    lines.push(`\t${shellEscape(url)}`);
    const command = lines.join(" \\\n");
    await navigator.clipboard.writeText(command);
    toast.success("Request copied as curl command");
  } catch (err: any) {
    toast.error(err?.message || "Unable to copy curl command.");
  }
};

const copyHeaderValue = async (key: string, value: string) => {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(`${key} copied`);
  } catch (err: any) {
    toast.error(err?.message || "Unable to copy header value.");
  }
};

const copyHeadersBlock = async (label: string, entries: Array<[string, string]>) => {
  try {
    const text = entries.map(([key, value]) => `${key}: ${value}`).join("\n");
    await navigator.clipboard.writeText(text);
    toast.success(`${label} copied`);
  } catch (err: any) {
    toast.error(err?.message || `Unable to copy ${label.toLowerCase()}.`);
  }
};

const copyBody = async (label: string, value: string) => {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(`${label} copied`);
  } catch (err: any) {
    toast.error(err?.message || `Unable to copy ${label.toLowerCase()}.`);
  }
};

const copyQueryShape = async (event: InspectEvent) => {
  const shape = eventShapePreview(event);
  if (!shape) return;
  try {
    await navigator.clipboard.writeText(shape);
    toast.success("Normalized query copied");
  } catch (err: any) {
    toast.error(err?.message || "Unable to copy normalized query.");
  }
};

const copyRawQuery = async (event: InspectEvent) => {
  const raw = readAttr(event, "raw_sql");
  if (!raw) return;
  try {
    await navigator.clipboard.writeText(raw);
    toast.success("Raw query copied");
  } catch (err: any) {
    toast.error(err?.message || "Unable to copy raw query.");
  }
};

const copyInspectID = async (inspectID: string) => {
  try {
    await navigator.clipboard.writeText(inspectID);
    toast.success("Inspect ID copied");
  } catch (err: any) {
    toast.error(err?.message || "Unable to copy inspect ID.");
  }
};

const refresh = async () => {
  refreshing.value = true;
  try {
    const requestedSource = inspectSource.value || sourceFilter.value;
    const sourceQuery = requestedSource ? `&source=${encodeURIComponent(requestedSource)}` : "";
    const res = await fetch(lighthousePath(`/api/inspect?limit=${inspectListFetchLimit}${sourceQuery}`));
    if (!res.ok) return;
    const payload = (await res.json()) as { inspects?: InspectSummary[] };
    inspects.value = payload.inspects || [];
    const routeSelected = readRouteInspectID();
    const routeSelectedVisible = filteredInspects.value.some((inspect) => inspect.trace_id === routeSelected) ? routeSelected : "";
    const defaultSelected = routeSelectedVisible || filteredInspects.value[0]?.trace_id || inspects.value[0]?.trace_id || "";
    if (!selectedInspectId.value) {
      selectedInspectId.value = defaultSelected;
    }
    const stillSelected = filteredInspects.value.some((inspect) => inspect.trace_id === selectedInspectId.value);
    if (!stillSelected) {
      selectedInspectId.value = defaultSelected;
    }
    await loadSelectedInspect();
    await scrollSelectedInspectIntoViewWithRetry("auto");
    lastRefreshAt.value = Date.now();
  } finally {
    refreshing.value = false;
  }
};

const maybeRefreshOnWindowFocus = async () => {
  if (document.visibilityState !== "visible") return;
  if (refreshing.value) return;
  if (Date.now() - lastRefreshAt.value < focusRefreshCooldownMs) return;
  await refresh();
};

const handleVisibilityChange = () => {
  if (document.visibilityState !== "visible") return;
  if (refreshing.value) return;
  void refresh();
};

const handleWindowFocus = () => {
  void maybeRefreshOnWindowFocus();
};

const loadSelectedInspect = async () => {
  if (!selectedInspectId.value) {
    selectedInspectRecord.value = null;
    loadingSelectedInspect.value = false;
    activeInspectTab.value = "timeline";
    return;
  }
  const requestedInspectID = selectedInspectId.value;
  const loadToken = ++selectedInspectLoadToken;
  loadingSelectedInspect.value = true;
  const res = await fetch(lighthousePath(`/api/inspect/${encodeURIComponent(requestedInspectID)}`));
  if (loadToken !== selectedInspectLoadToken) return;
  if (!res.ok) {
    inspects.value = inspects.value.filter((inspect) => inspect.trace_id !== requestedInspectID);
    const nextInspectID = filteredInspects.value[0]?.trace_id || "";
    if (nextInspectID && nextInspectID !== requestedInspectID) {
      selectedInspectId.value = nextInspectID;
      syncInspectToRoute(nextInspectID);
      void loadSelectedInspect();
      return;
    }
    selectedInspectRecord.value = null;
    loadingSelectedInspect.value = false;
    activeInspectTab.value = "timeline";
    return;
  }
  selectedInspectRecord.value = normalizeInspectRecord(await res.json());
  loadingSelectedInspect.value = false;
  applyDesiredInspectTab();
};

const selectInspect = async (inspectID: string) => {
  if (selectedInspectId.value === inspectID && selectedInspectRecord.value?.summary.trace_id === inspectID) return;
  selectedInspectId.value = inspectID;
  syncInspectToRoute(inspectID);
  void loadSelectedInspect();
};

const readRouteInspectID = () => {
  const inspect = route.query.inspect;
  return typeof inspect === "string" ? inspect.trim() : "";
};

const syncInspectToRoute = (inspectID: string) => {
  const current = readRouteInspectID();
  const currentTab = normalizeInspectTab(route.query.tab);
  if (current === inspectID && currentTab === activeInspectTab.value) return;
  router.replace({
    query: {
      ...route.query,
      inspect: inspectID || undefined,
      tab: activeInspectTab.value === "timeline" ? undefined : activeInspectTab.value,
    },
  });
};

const syncInspectTabToRoute = (tab: string) => {
  const normalized = normalizeInspectTab(tab);
  const currentTab = normalizeInspectTab(route.query.tab);
  if (currentTab === normalized) return;
  router.replace({
    query: {
      ...route.query,
      tab: normalized === "timeline" ? undefined : normalized,
    },
  });
};

const parseHTTPInspectName = (inspect: InspectSummary) => {
  if (String(inspect.source || "").trim().toLowerCase() !== "http") {
    return { method: "", path: "" };
  }
  const name = String(inspect.name || "").trim();
  const spaceIndex = name.indexOf(" ");
  if (spaceIndex <= 0) {
    return { method: "", path: name };
  }
  const method = name.slice(0, spaceIndex).trim();
  const path = name.slice(spaceIndex + 1).trim();
  return { method, path };
};

const isInternalInspect = (inspect: InspectSummary) => {
  const path = String(parseHTTPInspectName(inspect).path || inspect.labels?.path || "").trim().toLowerCase();
  const name = String(inspect.name || "").trim().toLowerCase();
  return path.startsWith("/lighthouse/") || name.includes("/lighthouse/");
};

const resolveTimeWindowStart = (now: number, windowValue: string) => {
  switch (String(windowValue || "").trim()) {
    case "5m":
      return now - (5 * 60 * 1000);
    case "15m":
      return now - (15 * 60 * 1000);
    case "1h":
      return now - (60 * 60 * 1000);
    case "6h":
      return now - (6 * 60 * 60 * 1000);
    case "24h":
      return now - (24 * 60 * 60 * 1000);
    default:
      return 0;
  }
};

const inspectSearchFields = (inspect: InspectSummary) => {
  const fields = new Set<string>();
  const add = (value: unknown) => {
    const text = String(value || "").trim().toLowerCase();
    if (text) fields.add(text);
  };
  add(inspect.trace_id);
  add(inspect.source);
  add(inspect.name);
  add(inspect.status);
  add(inspectDisplayName(inspect));
  for (const [key, value] of Object.entries(inspect.labels || {})) {
    add(key);
    add(value);
    add(`${key}:${value}`);
  }
  const source = String(inspect.source || "").trim().toLowerCase();
  switch (source) {
    case "http":
      add(parseHTTPInspectName(inspect).path);
      add(parseHTTPInspectName(inspect).method);
      break;
    case "jobs":
    case "scheduler":
      add(inspect.labels?.job_name);
      break;
    case "cli":
      add(inspect.labels?.command);
      add(inspect.labels?.command_name);
      break;
  }
  return Array.from(fields);
};

const inspectDisplayName = (inspect: InspectSummary) => {
  const path = parseHTTPInspectName(inspect).path;
  if (path && path !== inspect.name) {
    return path;
  }
  return inspect.name || "Inspect";
};

const inspectMethod = (inspect: InspectSummary) => {
  const method = String(parseHTTPInspectName(inspect).method || "").trim();
  return method || "";
};

const shortInspectID = (inspectID: string) => {
  const value = String(inspectID || "").trim();
  if (value.length <= 14) return value;
  return value.slice(0, 14);
};

const inspectStatusCode = (inspect: InspectSummary) => {
  const value = String(inspect.labels?.status_code || "").trim();
  return /^\d+$/.test(value) ? value : "";
};

const statusCodeClass = (statusCode: string) => {
  const code = Number(statusCode);
  if (!Number.isFinite(code)) return "text-muted";
  if (code >= 500) return "text-rose-400";
  if (code >= 400) return "text-amber-400";
  if (code >= 200) return "text-emerald-400";
  return "text-muted";
};

const statusDotClass = (statusCode: string) => {
  const code = Number(statusCode);
  if (!Number.isFinite(code)) return "bg-border";
  if (code >= 500) return "bg-rose-400";
  if (code >= 400) return "bg-amber-400";
  if (code >= 200) return "bg-emerald-400";
  return "bg-border";
};

const inspectSourceIcon = (source?: string) => {
  switch ((source || "").trim()) {
    case "http":
      return Route;
    case "cli":
      return Terminal;
    case "jobs":
      return Bot;
    case "scheduler":
      return ClipboardList;
    case "startup":
      return Tag;
    default:
      return Binary;
  }
};

const primaryLabelEntry = (inspect: InspectSummary) => {
  const entries = labelEntries(inspect.labels);
  if (entries.length === 0) return null;
  return entries[0];
};

const primaryLabelKey = (inspect: InspectSummary) => primaryLabelEntry(inspect)?.[0] || "";
const primaryLabelValue = (inspect: InspectSummary) => primaryLabelEntry(inspect)?.[1] || "";
const secondaryLabelEntries = (inspect: InspectSummary) => labelEntries(inspect.labels).slice(1);

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

const formatTimeAgo = (value?: string) => {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const deltaMs = Math.max(0, Date.now() - date.getTime());
  if (deltaMs < 1000) return "just now";
  const deltaSeconds = Math.round(deltaMs / 1000);
  if (deltaSeconds < 60) return `${deltaSeconds}s ago`;
  const deltaMinutes = Math.round(deltaSeconds / 60);
  if (deltaMinutes < 60) return `${deltaMinutes}m ago`;
  const deltaHours = Math.round(deltaMinutes / 60);
  if (deltaHours < 24) return `${deltaHours}h ago`;
  const deltaDays = Math.round(deltaHours / 24);
  return `${deltaDays}d ago`;
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

const durationPillClass = (durationMs?: number) => {
  const ms = Number(durationMs) || 0;
  if (ms < 10) return "border-emerald-400/40 bg-emerald-500/12 text-emerald-200";
  if (ms < 50) return "border-sky-400/40 bg-sky-500/12 text-sky-200";
  if (ms < 150) return "border-amber-400/40 bg-amber-500/12 text-amber-200";
  if (ms < 500) return "border-orange-400/40 bg-orange-500/12 text-orange-200";
  return "border-rose-400/40 bg-rose-500/12 text-rose-200";
};

const inspectIDPillClass = "border-violet-400/30 bg-violet-500/10 text-violet-100";
const valuePillClass = "border-slate-500/40 bg-slate-500/10 text-slate-100";
const methodValuePillClass = "border-slate-500/40 bg-slate-500/10 text-slate-100";
const startedPillClass = "border-slate-400/30 bg-slate-500/10 text-slate-100";
const eventsPillClass = "border-fuchsia-400/35 bg-fuchsia-500/12 text-fuchsia-200";
const hostPillClass = "border-cyan-400/35 bg-cyan-500/12 text-cyan-200";
const ipPillClass = "border-indigo-400/35 bg-indigo-500/12 text-indigo-200";

const methodPillClass = (method?: string) => {
  switch (String(method || "").trim().toUpperCase()) {
    case "GET":
    case "HEAD":
    case "OPTIONS":
      return "border-sky-500/45 bg-sky-500/8 text-sky-100";
    case "POST":
      return "border-orange-500/45 bg-orange-500/10 text-orange-100";
    case "PUT":
    case "PATCH":
      return "border-sky-500/45 bg-sky-500/8 text-sky-100";
    case "DELETE":
      return "border-red-500/45 bg-red-500/10 text-red-100";
    default:
      return "border-border/60 bg-background/40 text-foreground";
  }
};

const methodTextClass = (method?: string) => {
  switch (String(method || "").trim().toUpperCase()) {
    case "GET":
    case "HEAD":
    case "OPTIONS":
      return "text-emerald-400";
    case "POST":
      return "text-violet-300";
    case "PUT":
    case "PATCH":
      return "text-amber-300";
    case "DELETE":
      return "text-rose-300";
    default:
      return "text-foreground";
  }
};

const statusPillClass = (statusCode?: number) => {
  const code = Number(statusCode) || 0;
  if (code >= 500) return "border-rose-400/40 bg-rose-500/12 text-rose-200";
  if (code >= 400) return "border-amber-400/40 bg-amber-500/12 text-amber-200";
  if (code >= 200 && code < 300) return "border-emerald-400/40 bg-emerald-500/12 text-emerald-200";
  return "border-border/60 bg-background/40 text-foreground";
};

const labelPillClass = (key?: string) => {
  switch (String(key || "").trim().toLowerCase()) {
    case "path":
      return "border-sky-400/35 bg-sky-500/12 text-sky-200";
    case "job_name":
    case "command":
    case "command_name":
      return "border-violet-400/35 bg-violet-500/12 text-violet-200";
    default:
      return "border-slate-400/30 bg-slate-500/10 text-slate-100";
  }
};

const formatInspectOffset = (startedAt?: string, eventAt?: string) => {
  if (!startedAt || !eventAt) return "+0 ms";
  const start = new Date(startedAt).getTime();
  const at = new Date(eventAt).getTime();
  if (Number.isNaN(start) || Number.isNaN(at)) return "+0 ms";
  return `+${formatDuration(Math.max(0, at - start))}`;
};

const inspectOffsetClass = (startedAt?: string, eventAt?: string) => {
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

const readAttr = (event: InspectEvent | null | undefined, key: string) => {
  const value = event?.attributes?.[key];
  if (value === undefined || value === null) return "";
  if (typeof value === "boolean") return value ? "true" : "false";
  if (typeof value === "number") return Number.isFinite(value) ? `${value}` : "";
  if (typeof value === "string") return value.trim();
  return JSON.stringify(value);
};

const eventHeadline = (event: InspectEvent) => {
  switch (event.kind) {
    case "cache": {
      const operation = readAttr(event, "operation") || event.name || "operation";
      const cacheName = readAttr(event, "cache") || "cache";
      return `${operation} ${cacheName}`;
    }
    case "storage": {
      const operation = readAttr(event, "operation") || event.name || "operation";
      const disk = readAttr(event, "disk") || "disk";
      return `${operation} ${disk}`;
    }
    case "event": {
      const operation = readAttr(event, "operation") || event.name || "event";
      const topic = readAttr(event, "topic");
      return topic ? `${operation} ${topic}` : operation;
    }
    case "mail": {
      const name = readAttr(event, "name") || event.name || "mail";
      return `send ${name}`;
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
      if (isHTTPRequestLog(event)) {
        return "HTTP Request";
      }
      return event.message || "log entry";
    case "error":
      return event.message || "error";
    default:
      return event.message || event.name || event.kind;
  }
};

const eventSubline = (event: InspectEvent) => {
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

const eventDurationParts = (event: InspectEvent) => {
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

const statusValueClass = (value: string) => {
  const code = Number(String(value || "").trim());
  if (!Number.isFinite(code)) return "";
  if (code >= 500) return "text-rose-300";
  if (code >= 400) return "text-amber-300";
  if (code >= 200) return "text-emerald-300";
  return "";
};

const genericValueClass = (value: string) => {
  const boolClass = boolValueClass(value);
  if (boolClass) return boolClass;
  return "text-slate-200";
};

const isHTTPRequestLog = (event: InspectEvent) => event.kind === "log" && String(event.message || "").trim() === "HTTP Request";
const isStructuredAppLog = (event: InspectEvent) => event.kind === "log" && !isHTTPRequestLog(event);

const formatLogFieldLabel = (key: string) => key.replaceAll("_", " ");

const structuredLogFields = (event: InspectEvent): Array<[string, string]> => {
  if (!isStructuredAppLog(event)) {
    return [];
  }
  const preferredOrder = [
    "monitors_total",
    "monitors_up",
    "monitors_down",
    "monitors_pending",
    "monitors_paused",
    "incidents_open",
    "checks_last_hour",
    "maintenance_active",
  ];
  const rank = new Map(preferredOrder.map((key, index) => [key, index]));
  return Object.entries(event.attributes || {})
    .filter(([key, value]) => !["duration_ms", "duration_ns", "source"].includes(key) && value !== undefined && value !== null && `${value}` !== "")
    .map(([key, value]) => [key, typeof value === "string" ? value : JSON.stringify(value)] as [string, string])
    .sort(([a], [b]) => {
      const aRank = rank.get(a);
      const bRank = rank.get(b);
      if (aRank !== undefined || bRank !== undefined) {
        return (aRank ?? Number.MAX_SAFE_INTEGER) - (bRank ?? Number.MAX_SAFE_INTEGER);
      }
      return a.localeCompare(b);
    });
};

const eventInlineFields = (event: InspectEvent): InlineField[] => {
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
        pair("driver", readAttr(event, "driver")),
        pair("key", readAttr(event, "key")),
        pair("hit", readAttr(event, "hit"), boolValueClass(readAttr(event, "hit"))),
        durationField(),
      ].filter(Boolean) as InlineField[];
    case "storage":
      return [
        pair("driver", readAttr(event, "driver")),
        pair("disk", readAttr(event, "disk")),
        pair("path", readAttr(event, "path")),
        durationField(),
      ].filter(Boolean) as InlineField[];
    case "event":
      return [
        pair("bus", readAttr(event, "bus")),
        pair("driver", readAttr(event, "driver")),
        pair("handler", readAttr(event, "handler")),
        durationField(),
      ].filter(Boolean) as InlineField[];
    case "mail":
      return [
        pair("driver", readAttr(event, "driver")),
        durationField(),
      ].filter(Boolean) as InlineField[];
    case "queue":
      return [
        pair("queue", readAttr(event, "queue")),
        pair("job_key", readAttr(event, "job_key")),
        pair("attempt", readAttr(event, "attempt")),
        pair("scheduled", readAttr(event, "scheduled"), boolValueClass(readAttr(event, "scheduled"))),
        durationField(),
      ].filter(Boolean) as InlineField[];
    case "query":
      return [
        pair("connection", readAttr(event, "connection")),
        pair("driver", readAttr(event, "driver")),
        pair("fingerprint", readAttr(event, "fingerprint")),
        pair("rows", readAttr(event, "rows")),
        durationField(),
      ].filter(Boolean) as InlineField[];
    case "log":
      if (isHTTPRequestLog(event)) {
        return [
          pair("method", readAttr(event, "method")),
          pair("status", readAttr(event, "status"), statusValueClass(readAttr(event, "status"))),
          pair("ip", readAttr(event, "remote_ip")),
          durationField(),
        ].filter(Boolean) as InlineField[];
      }
      return [
        durationField(),
      ].filter(Boolean) as InlineField[];
    default:
      return [];
  }
};

const eventInlineDurationField = (event: InspectEvent) => eventInlineFields(event).find((field) => field.key === "duration");

const eventInlineDuration = (event: InspectEvent) => eventInlineDurationField(event)?.value || "";

const eventInlineDurationClass = (event: InspectEvent) => eventInlineDurationField(event)?.valueClassName || "text-muted";

const eventInlineSummary = (event: InspectEvent) =>
  eventInlineFields(event)
    .filter((field) => field.key !== "duration")
    .map((field) => (field.label ? `${field.label} ${field.value}` : field.value))
    .join(" • ");

const eventKindIcon = (kind?: string) => {
  switch ((kind || "").toLowerCase()) {
    case "query":
      return Database;
    case "cache":
      return HardDrive;
    case "storage":
      return HardDrive;
    case "event":
      return Workflow;
    case "mail":
      return ScrollText;
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
    case "event":
      return "border-cyan-500/30 bg-cyan-500/10 text-cyan-200";
    case "mail":
      return "border-emerald-500/30 bg-emerald-500/10 text-emerald-200";
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

const eventSummaryLine = (event: InspectEvent) => {
  switch (event.kind) {
    case "queue": {
      const queueName = readAttr(event, "queue");
      const attempt = readAttr(event, "attempt");
      const scheduled = readAttr(event, "scheduled");
      const parts = [queueName && `queue ${queueName}`, attempt && `attempt ${attempt}`, scheduled && `scheduled ${scheduled}`].filter(Boolean);
      return parts.join(" · ");
    }
    case "event": {
      const topic = readAttr(event, "topic");
      const err = readAttr(event, "error");
      return [topic, err].filter(Boolean).join(" · ");
    }
    case "log": {
      if (isHTTPRequestLog(event)) {
        const uri = readAttr(event, "uri");
        const error = readAttr(event, "error");
        return [uri, error].filter(Boolean).join(" · ");
      }
      const attrs = structuredLogFields(event)
        .slice(0, 8)
        .map(([key, value]) => `${formatLogFieldLabel(key)} ${value}`);
      return attrs.join(" · ");
    }
    default:
      return "";
  }
};

const eventShapePreview = (event: InspectEvent) => {
  if (event.kind !== "query") return "";
  const shape = readAttr(event, "shape");
  if (shape.trim().toLowerCase() === "other") {
    return "";
  }
  return shape;
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

const eventShapePreviewHTML = (event: InspectEvent) => highlightSQL(eventShapePreview(event));

const eventExtraFields = (event: InspectEvent): Array<[string, string]> => {
  const omit = new Set(["cache", "operation", "driver", "key", "hit", "duration_ms", "duration_ns", "queue", "job_name", "job_key", "kind", "attempt", "scheduled", "connection", "target", "fingerprint", "shape", "raw_sql", "rows", "source", "disk", "path", "bus", "topic", "handler", "name"]);
  if (isHTTPRequestLog(event)) {
    ["uri", "status", "method", "remote_ip", "latency_ms", "latency_ns", "memory_bytes"].forEach((key) => omit.add(key));
  }
  const fields = Object.entries(event.attributes || {})
    .filter(([key, value]) => !omit.has(key) && value !== undefined && value !== null && `${value}` !== "")
    .map(([key, value]) => [key, typeof value === "string" ? value : JSON.stringify(value)]);
  if (isStructuredAppLog(event)) {
    const promotedKeys = new Set(structuredLogFields(event).slice(0, 8).map(([key]) => key));
    return fields.filter(([key]) => !promotedKeys.has(key));
  }
  return fields;
};

const isSingleErrorField = (event: InspectEvent) => {
  const fields = eventExtraFields(event);
  return fields.length === 1 && fields[0]?.[0] === "error";
};

const singleErrorFieldValue = (event: InspectEvent) => {
  if (!isSingleErrorField(event)) {
    return "";
  }
  return eventExtraFields(event)[0]?.[1] || "";
};

const showEventExtraFields = (event: InspectEvent) => {
  if (isHTTPRequestLog(event)) {
    return eventExtraFields(event).some(([key]) => key === "error");
  }
  return eventExtraFields(event).length > 0;
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

const inspectRowClass = (inspect: InspectSummary) => {
  const selected = inspect.trace_id === selectedInspectId.value;
  const base = selected
    ? "bg-white/[0.03] ring-1 ring-emerald-400/45 shadow-[0_0_0_1px_rgba(52,211,153,0.18)]"
    : "bg-transparent hover:bg-white/[0.025]";
  switch ((inspect.status || "").toLowerCase()) {
    case "ok":
      return `${base} border-emerald-400/30`;
    case "error":
      return `${base} border-destructive/30`;
    case "warning":
      return `${base} border-amber-400/30`;
    default:
      return `${base} border-border/55`;
  }
};

watch(requestExchange, (value) => {
  if (!value && activeInspectTab.value !== "timeline") {
    activeInspectTab.value = "timeline";
    syncInspectTabToRoute("timeline");
  }
});

watch(activeInspectTab, (value) => {
  const normalized = normalizeInspectTab(value);
  if (normalized !== value) {
    activeInspectTab.value = normalized;
    return;
  }
  if (!requestExchange.value && normalized !== "timeline") {
    activeInspectTab.value = "timeline";
    return;
  }
  desiredInspectTab.value = normalized;
  syncInspectTabToRoute(normalized);
});

watch([sourceFilter, showInternal, inspectSource], async () => {
  await refresh();
});

watch([query, statusFilter, timeWindow], async () => {
  const stillSelected = filteredInspects.value.some((inspect) => inspect.trace_id === selectedInspectId.value);
  if (stillSelected) {
    return;
  }
  const nextInspectID = filteredInspects.value[0]?.trace_id || "";
  selectedInspectId.value = nextInspectID;
  syncInspectToRoute(nextInspectID);
  await loadSelectedInspect();
});

watch(
  () => [route.path, route.query.inspect, route.query.tab],
  async () => {
    desiredInspectTab.value = normalizeInspectTab(route.query.tab);
    const routeInspectID = readRouteInspectID();
    if (routeInspectID && routeInspectID !== selectedInspectId.value) {
      selectedInspectId.value = routeInspectID;
      await loadSelectedInspect();
      return;
    }
    if (!routeInspectID) {
      await refresh();
      return;
    }
    applyDesiredInspectTab();
  }
);

onMounted(async () => {
  selectedInspectId.value = readRouteInspectID();
  desiredInspectTab.value = normalizeInspectTab(route.query.tab);
  await refresh();
  initialInspectScrollDone.value = await scrollSelectedInspectIntoViewWithRetry("auto");
  document.addEventListener("visibilitychange", handleVisibilityChange);
  window.addEventListener("focus", handleWindowFocus);
});

onBeforeUnmount(() => {
  document.removeEventListener("visibilitychange", handleVisibilityChange);
  window.removeEventListener("focus", handleWindowFocus);
});

watch(
  () => [filteredInspects.value.map((inspect) => inspect.trace_id).join("|"), selectedInspectId.value],
  async () => {
    if (initialInspectScrollDone.value) return;
    initialInspectScrollDone.value = await scrollSelectedInspectIntoViewWithRetry("auto");
  }
);
</script>
