<template>
  <div>
    <section>
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <Activity class="h-4 w-4 text-muted-foreground" />
              Dev Watcher
            </CardTitle>
          </template>
          <template #description>
            <CardDescription>Watch development process output and control runtime helpers.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="devwatch-controls">
            <div class="devwatch-settings-row">
              <div class="devwatch-settings">
                <div class="flex items-center gap-2">
                  <span class="inline-flex items-center gap-1 text-muted uppercase tracking-[0.2em] text-[10px]">
                    <Database class="h-3 w-3" />
                    DB Query Logging
                  </span>
                  <ToggleGroup
                    type="single"
                    variant="outline"
                    size="sm"
                    :model-value="dbQueryLogging ? 'on' : 'off'"
                    @update:model-value="(value) => dbQueryLogging = value === 'on'"
                  >
                    <ToggleGroupItem value="off" class="uppercase tracking-[0.2em] text-[10px]">
                      <EyeOff class="h-3 w-3" />
                      Off
                    </ToggleGroupItem>
                    <ToggleGroupItem value="on" class="uppercase tracking-[0.2em] text-[10px]">
                      <Eye class="h-3 w-3" />
                      On
                    </ToggleGroupItem>
                  </ToggleGroup>
                </div>
                <div class="flex items-center gap-2">
                  <span class="inline-flex items-center gap-1 text-muted uppercase tracking-[0.2em] text-[10px]">
                    <Bug class="h-3 w-3" />
                    App Debug
                  </span>
                  <ToggleGroup
                    type="single"
                    variant="outline"
                    size="sm"
                    :model-value="appDebug"
                    @update:model-value="(value) => appDebug = value ?? appDebug"
                  >
                    <ToggleGroupItem
                      v-for="level in debugLevels"
                      :key="level"
                      :value="level"
                      class="uppercase tracking-[0.2em] text-[10px]"
                    >
                      {{ level }}
                    </ToggleGroupItem>
                  </ToggleGroup>
                </div>
              </div>
              <Button variant="secondary" :disabled="savingEnv || !envReady || !envDirty" @click="applyEnvSettings">
                Apply
              </Button>
            </div>
            <div class="devwatch-actions-row mb-1">
              <Tabs v-model="activeTab" class="devwatch-tabs">
                <TabsList class="devwatch-tabs-list">
                  <TabsTrigger
                    v-for="(tab, index) in watcherTabs"
                    :key="tab"
                    :value="tab"
                    class="devwatch-tab-trigger"
                  >
                    <span class="devwatch-tab-label">{{ tab }}</span>
                    <span v-if="index < 9" class="devwatch-tab-key">
                      <Keyboard class="h-3 w-3" />
                      {{ index + 1 }}
                    </span>
                    <span v-if="tab !== 'All' && getUnreadCount(tab) > 0" class="devwatch-tab-pill">
                      <ScrollText class="h-3 w-3" />
                      {{ getUnreadCount(tab) }}
                    </span>
                  </TabsTrigger>
                </TabsList>
              </Tabs>
              <div class="devwatch-actions-left">
                <div class="devwatch-filter">
                  <Search class="devwatch-filter-icon" />
                  <Input
                    v-model="filterText"
                    class="devwatch-filter-input"
                    placeholder="Filter output… (text or /regex/)"
                    @keydown.escape.prevent="filterText = ''"
                    ref="filterInput"
                  />
                  <button
                    v-if="filterText"
                    type="button"
                    class="devwatch-filter-clear"
                    @click="filterText = ''"
                    aria-label="Clear filter"
                  >
                    <X class="h-3.5 w-3.5" />
                  </button>
                </div>
                <Button variant="outline" class="devwatch-action-button text-[10px] uppercase tracking-[0.2em]" @click="togglePause">
                  <component :is="paused ? Play : Pause" class="mr-1 h-3.5 w-3.5" />
                  {{ paused ? "Resume" : "Pause" }}
                  <span
                    class="ml-1 inline-flex items-center gap-1 rounded-full border border-border/60 bg-muted/40 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
                  >
                    <Keyboard class="h-3 w-3" />
                    P
                  </span>
                  <span
                    v-if="paused && pendingLineCount > 0"
                    class="ml-1 inline-flex items-center gap-1 rounded-full border border-amber-400/50 bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-[0.12em] text-amber-200"
                  >
                    <ScrollText class="h-3 w-3" />
                    {{ pendingLineCount }}
                  </span>
                </Button>
                <Button variant="outline" class="devwatch-action-button" :disabled="!devwatchConnected" @click="restart">
                  <RotateCw class="mr-1 h-3.5 w-3.5" />
                  Restart Watchers
                  <span
                    class="ml-1 inline-flex items-center gap-1 rounded-full border border-border/60 bg-muted/40 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
                  >
                    <Keyboard class="h-3 w-3" />
                    R
                  </span>
                </Button>
              </div>
            </div>
          </div>
          <div v-if="envStatus" class="mb-3 text-xs" :class="envStatusTone">
            {{ envStatus }}
          </div>
          <div class="terminal-wrap" :class="paused ? 'terminal-wrap-paused' : ''">
            <div ref="terminalRef" class="terminal-pane" :class="paused ? 'terminal-pane-paused' : ''">
              <div ref="terminalLines" class="terminal-lines"></div>
              <div v-if="displayLineCount === 0" class="terminal-empty text-xs text-muted">
                {{ filterText ? "No lines match the current filter." : "Waiting for watcher output…" }}
              </div>
              <div class="terminal-follow-wrap" v-if="showFollowHint">
                <button class="terminal-follow" @click="resumeFollow">
                  Watch Output
                </button>
              </div>
            </div>
            <div v-if="paused" class="terminal-paused-overlay">
              <div class="terminal-paused-badge">
                <Pause class="h-7 w-7" />
                <span>Paused</span>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </section>

  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  useLighthouseStore,
  DevwatchLine,
  DevwatchSnapshot,
  DevwatchUpdate,
  EditorTarget,
} from "../stores/lighthouse";
import { ansiToHtml } from "../lib/ansi";
import { toast } from "vue-sonner";
import Button from "../components/ui/button/Button.vue";
import { Input } from "../components/ui/input";
import { ToggleGroup, ToggleGroupItem } from "../components/ui/toggle-group";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import { Tabs, TabsList, TabsTrigger } from "../components/ui/tabs";
import { Activity, Bug, Database, Eye, EyeOff, Keyboard, Pause, Play, RotateCw, ScrollText, Search, X } from "lucide-vue-next";

type LineReference = {
  path?: string;
  line?: number;
  symbol?: string;
};

type ManualLine = {
  id: number;
  raw: string;
  html: string;
  watcher: string;
  reference?: LineReference;
};

const store = useLighthouseStore();
const route = useRoute();
const router = useRouter();
const terminalRef = ref<HTMLElement | null>(null);
const terminalLines = ref<HTMLDivElement | null>(null);
const filterInput = ref<InstanceType<typeof Input> | null>(null);
const devwatchConnected = computed(() => store.state.devwatchConnected);
const localClient = computed(() => store.state.localClient);
const activeTab = ref("All");
const localWatcherList = ref<string[]>([]);
const localWatcherSet = new Set<string>();

const registerLocalWatcher = (watcher: string) => {
  if (!watcher) {
    return;
  }
  if (localWatcherSet.has(watcher)) {
    return;
  }
  localWatcherSet.add(watcher);
  localWatcherList.value = [...localWatcherList.value, watcher];
};

const resetLocalWatchers = () => {
  localWatcherSet.clear();
  localWatcherList.value = [];
};

const watcherTabs = computed(() => {
  const tabs = ["All"];
  const seen = new Set<string>(tabs);
  const append = (values?: string[]) => {
    if (!values) {
      return;
    }
    for (const name of values) {
      if (!name || seen.has(name)) {
        continue;
      }
      tabs.push(name);
      seen.add(name);
    }
  };
  append(store.state.devwatchWatchers);
  append(localWatcherList.value);
  return tabs;
});
watch(activeTab, (value) => {
  if (value === "All") {
    return;
  }
  clearWatcherUnread(value);
});
watch(localClient, () => {
  scheduleRender();
});
const getUnreadCount = (tab: string) => unreadCounts.value[tab] ?? 0;
const envReady = ref(false);
const envStatus = ref("");
const envStatusTone = ref("text-muted");
const savingEnv = ref(false);
const envContent = ref("");
const dbQueryLogging = ref(false);
const appDebug = ref("1");
const baseDbQueryLogging = ref(false);
const baseAppDebug = ref("1");
const debugLevels = ["0", "1", "2", "3"];
const followTailByTab = ref<Record<string, boolean>>({});
const scrollTopByTab = ref<Record<string, number>>({});
const currentFollowTail = computed(() => followTailByTab.value[activeTab.value] !== false);
const envDirty = computed(
  () =>
    envReady.value &&
    (dbQueryLogging.value !== baseDbQueryLogging.value || appDebug.value !== baseAppDebug.value)
);
const paused = ref(false);
const pendingLines: ManualLine[] = [];
const pendingLineCount = ref(0);
const referenceByTab = ref<Record<string, LineReference>>({});
const currentReference = computed(() => referenceByTab.value[activeTab.value] ?? referenceByTab.value.All ?? null);
const referenceLabel = computed(() => {
  const reference = currentReference.value;
  if (!reference) {
    return "";
  }
  if (reference.path) {
    return reference.line ? `${reference.path}:${reference.line}` : reference.path;
  }
  return reference.symbol ?? "";
});
const hasTerminalLines = ref(false);
const displayLineCount = ref(0);
const filterText = ref("");
const manualLines: ManualLine[] = [];
const perWatcherLines = new Map<string, ManualLine[]>();
const unreadCounts = ref<Record<string, number>>({});
let nextLineId = 1;
let suppressScroll = false;
let unsubscribeDevwatch: (() => void) | null = null;
const clearTerminal = () => {
  const container = terminalLines.value;
  if (container) {
    container.innerHTML = "";
  }
  hasTerminalLines.value = false;
  displayLineCount.value = 0;
};

const resetTerminalState = () => {
  manualLines.splice(0, manualLines.length);
  perWatcherLines.clear();
  localWatcherList.value = [];
  localWatcherSet.clear();
  clearTerminal();
  unreadCounts.value = {};
  referenceByTab.value = {};
};

const ansiEscapeRegex = /\u001b\[[0-9;]*m/g;
const fileLineRegex = /(?:\.\/)?([^\s:()]+\.go)(?::(\d+))?/;
const symbolRegex = /#([\w\.]+)/g;

const parseReference = (raw: string): LineReference | null => {
  const cleaned = raw.replace(ansiEscapeRegex, "");
  const [firstLine] = cleaned.split(/\r?\n/);
  const fileMatch = fileLineRegex.exec(firstLine);
  const symbolMatches = Array.from(firstLine.matchAll(symbolRegex));
  const path = fileMatch ? fileMatch[1] : undefined;
  const line = fileMatch && fileMatch[2] ? Number(fileMatch[2]) : undefined;
  let symbol: string | undefined;
  for (const match of symbolMatches) {
    const candidate = match[1];
    if (!candidate) {
      continue;
    }
    const parts = candidate.split(".");
    if (parts.length >= 3) {
      symbol = candidate;
      break;
    }
  }
  if (!path && !symbol) {
    return null;
  }
  return { path, line, symbol };
};

const updateReferenceForLine = (watcher: string, reference?: LineReference) => {
  if (!reference || (!reference.path && !reference.symbol)) {
    return;
  }
  const key = watcher || "All";
  referenceByTab.value = {
    ...referenceByTab.value,
    [key]: reference,
    All: reference,
  };
};

const convertEntries = (entries: DevwatchLine[]) => {
  const converted: ManualLine[] = [];
  for (const entry of entries) {
    const raw = entry.line || "";
    const watcher = entry.watcher || "";
    const id = typeof entry.id === "number" && Number.isFinite(entry.id) ? entry.id : nextLineId++;
    registerLocalWatcher(watcher);
    const reference = parseReference(raw);
    console.log("[DevWatcher] parsed reference", { id, watcher, raw, reference });
    const manualLine: ManualLine = {
      id,
      raw,
      html: ansiToHtml(raw),
      watcher,
      reference: reference ?? undefined,
    };
    converted.push(manualLine);
    if (reference?.path || reference?.symbol) {
      updateReferenceForLine(watcher, reference);
    }
  }
  if (converted.length > 0) {
    hasTerminalLines.value = true;
  }
  return converted;
};

let renderPending = false;
const pruneStaleDomEntries = (source: ManualLine[]) => {
  const container = terminalLines.value;
  if (!container) {
    return;
  }
  const expectedIds = new Set(source.map((line) => String(line.id)));
  Array.from(container.children).forEach((node) => {
    const lineId = node.getAttribute("data-line-id");
    if (!lineId) {
      return;
    }
    if (!expectedIds.has(lineId)) {
      node.remove();
    }
  });
};
const HIGHLIGHT_START = "\u0007";
const HIGHLIGHT_END = "\u0008";

const escapeRegExp = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

const buildFilterSpec = (value: string) => {
  const trimmed = value.trim();
  if (!trimmed) {
    return {
      matches: () => true,
      highlight: null as ((line: ManualLine) => string) | null,
    };
  }
  const regexMatch = /^\/(.+)\/([gimsuy]*)$/.exec(trimmed);
  if (regexMatch) {
    const [, source, flagsRaw] = regexMatch;
    try {
      const matchFlags = flagsRaw.replace("g", "");
      const matchRegex = new RegExp(source, matchFlags);
      if (matchRegex.test("")) {
        return {
          matches: (line: ManualLine) => {
            matchRegex.lastIndex = 0;
            return matchRegex.test(line.raw);
          },
          highlight: null,
        };
      }
      const highlightFlags = flagsRaw.includes("g") ? flagsRaw : `${flagsRaw}g`;
      const highlightRegex = new RegExp(source, highlightFlags);
      return {
        matches: (line: ManualLine) => {
          matchRegex.lastIndex = 0;
          return matchRegex.test(line.raw);
        },
        highlight: (line: ManualLine) => {
          matchRegex.lastIndex = 0;
          if (!matchRegex.test(line.raw)) {
            return line.html;
          }
          const marked = line.raw.replace(highlightRegex, (match) => `${HIGHLIGHT_START}${match}${HIGHLIGHT_END}`);
          const html = ansiToHtml(marked);
          return html
            .split(HIGHLIGHT_START)
            .join('<mark class="devwatch-highlight">')
            .split(HIGHLIGHT_END)
            .join("</mark>");
        },
      };
    } catch {
      // Fall back to plain text handling.
    }
  }
  const needle = trimmed.toLowerCase();
  const highlightRegex = new RegExp(escapeRegExp(trimmed), "gi");
  return {
    matches: (line: ManualLine) => line.raw.toLowerCase().includes(needle),
    highlight: (line: ManualLine) => {
      if (!line.raw.toLowerCase().includes(needle)) {
        return line.html;
      }
      const marked = line.raw.replace(highlightRegex, (match) => `${HIGHLIGHT_START}${match}${HIGHLIGHT_END}`);
      const html = ansiToHtml(marked);
      return html
        .split(HIGHLIGHT_START)
        .join('<mark class="devwatch-highlight">')
        .split(HIGHLIGHT_END)
        .join("</mark>");
    },
  };
};

const filterSpec = computed(() => buildFilterSpec(filterText.value));

const renderActiveLines = () => {
  const container = terminalLines.value;
  if (!container) {
    return;
  }
  const source =
    activeTab.value === "All"
      ? manualLines
      : perWatcherLines.get(activeTab.value) ?? [];
  const { matches, highlight } = filterSpec.value;
  const filtered = source.filter(matches);
  displayLineCount.value = filtered.length;
  pruneStaleDomEntries(filtered);
  container.innerHTML = "";
  filtered.forEach((line) => {
    const node = document.createElement("div");
    node.className = "terminal-line";
    node.dataset.lineId = String(line.id);
    node.dataset.watcher = line.watcher;
    const content = document.createElement("div");
    content.className = "terminal-line-content";
    content.innerHTML = highlight ? highlight(line) : line.html;
    node.appendChild(content);
    const reference = line.reference;
    if (localClient.value && (reference?.path || reference?.symbol)) {
      const actions = document.createElement("div");
      actions.className = "terminal-line-actions";
      actions.appendChild(createEditorButton("goland", reference, "GoLand"));
      actions.appendChild(createEditorButton("vscode", reference, "VS Code"));
      node.appendChild(actions);
    }
    container.appendChild(node);
  });
};

const scheduleRender = () => {
  if (renderPending) {
    return;
  }
  renderPending = true;
  window.requestAnimationFrame(() => {
    renderPending = false;
    renderActiveLines();
    if (currentFollowTail.value) {
      scrollToBottom();
    }
  });
};

const matchesActiveTab = (line: ManualLine) => {
  if (activeTab.value === "All") {
    return true;
  }
  return line.watcher === activeTab.value;
};

const pushToWatcherBuffer = (line: ManualLine) => {
  if (!line.watcher) {
    return;
  }
  const limit = store.state.devwatchLimit;
  const buffer = perWatcherLines.get(line.watcher) ?? [];
  buffer.push(line);
  if (buffer.length > limit) {
    buffer.shift();
  }
  perWatcherLines.set(line.watcher, buffer);
};

const appendManualLine = (line: ManualLine) => {
  const limit = store.state.devwatchLimit;
  manualLines.push(line);
  pushToWatcherBuffer(line);
  if (manualLines.length > limit) {
    const removed = manualLines.shift();
  }
  markWatcherUnread(line.watcher);
  scheduleRender();
};

const flushPendingLines = () => {
  if (pendingLines.length === 0) {
    pendingLineCount.value = 0;
    return;
  }
  const queued = pendingLines.splice(0, pendingLines.length);
  pendingLineCount.value = pendingLines.length;
  queued.forEach(appendManualLine);
};
const togglePause = () => {
  paused.value = !paused.value;
  if (paused.value) {
    toast("Watcher log streaming paused (buffering)", {
      description: "New output is being buffered until you resume.",
    });
  } else {
    toast("Watcher log streaming resumed", {
      description: pendingLineCount.value > 0
        ? `Flushing ${pendingLineCount.value} queued lines.`
        : "Resuming live output.",
    });
  }
  if (!paused.value) {
    flushPendingLines();
    followTailByTab.value[activeTab.value] = true;
    scheduleRender();
    nextTick(() => {
      scrollToBottom();
    });
  }
};

const markWatcherUnread = (watcher: string) => {
  if (!watcher) {
    return;
  }
  if (activeTab.value === watcher) {
    return;
  }
  const next = { ...unreadCounts.value };
  next[watcher] = (next[watcher] ?? 0) + 1;
  unreadCounts.value = next;
};

const clearWatcherUnread = (watcher: string) => {
  if (!watcher) {
    return;
  }
  if (!unreadCounts.value[watcher]) {
    return;
  }
  const next = { ...unreadCounts.value };
  delete next[watcher];
  unreadCounts.value = next;
};

const editorDisplayName = (target: EditorTarget) => (target === "goland" ? "GoLand" : "VS Code");

const openInEditor = async (target: EditorTarget, reference?: LineReference) => {
  const ref = reference ?? currentReference.value;
  if (!ref?.path && !ref?.symbol) {
    return;
  }
  try {
    await store.openEditor({
      path: ref.path,
      line: ref.line ?? 1,
      target,
      symbol: ref.symbol,
    });
    toast(`Opened in ${editorDisplayName(target)}`);
  } catch (error: any) {
    const message = error?.message || "Unable to open editor.";
    toast(`Failed to open ${editorDisplayName(target)}: ${message}`);
  }
};

const createEditorButton = (target: EditorTarget, reference: LineReference, label: string) => {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "terminal-line-action";
  button.textContent = label;
  button.addEventListener("click", (event) => {
    event.stopPropagation();
    openInEditor(target, reference);
  });
  return button;
};

const handleSnapshot = (snapshot: DevwatchSnapshot) => {
  manualLines.splice(0, manualLines.length);
  const limit = store.state.devwatchLimit;
  resetLocalWatchers();
  pendingLines.length = 0;
  pendingLineCount.value = 0;
  const converted = convertEntries(snapshot.lines.slice(-limit));
  manualLines.push(...converted);
  perWatcherLines.clear();
  manualLines.forEach(pushToWatcherBuffer);
  scheduleRender();
};

const handleAppend = (line: DevwatchLine) => {
  const [converted] = convertEntries([line]);
  if (paused.value) {
    pendingLines.push(converted);
    pendingLineCount.value = pendingLines.length;
    return;
  }
  appendManualLine(converted);
};

const handleDevwatchUpdate = (update: DevwatchUpdate) => {
  if (update.kind === "snapshot") {
    handleSnapshot(update.snapshot);
    return;
  }
  handleAppend(update.line);
};

const loadEnv = async () => {
  envStatus.value = "";
  envStatusTone.value = "text-muted";
  try {
    const res = await fetch("/__lighthouse/api/env?file=.env");
    if (!res.ok) {
      envStatus.value = "Unable to load .env.";
      envStatusTone.value = "text-red-300/80";
      envReady.value = false;
      return;
    }
    const payload = (await res.json()) as { content?: string };
    envContent.value = payload.content || "";
    envReady.value = true;
    parseEnvSettings();
  } catch {
    envStatus.value = "Unable to load .env.";
    envStatusTone.value = "text-red-300/80";
    envReady.value = false;
  }
};

const parseEnvSettings = () => {
  const lines = envContent.value.split("\n");
  const findValue = (key: string) => {
    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith("#")) continue;
      const normalized = trimmed.startsWith("export ") ? trimmed.slice(7).trim() : trimmed;
      if (normalized.startsWith(`${key}=`)) {
        return normalized.slice(key.length + 1).trim();
      }
    }
    return "";
  };
  const dbValue = findValue("DB_QUERY_LOGGING");
  dbQueryLogging.value = dbValue === "true" || dbValue === "1";
  const debugValue = findValue("APP_DEBUG");
  if (debugLevels.includes(debugValue)) {
    appDebug.value = debugValue;
  }
  baseDbQueryLogging.value = dbQueryLogging.value;
  baseAppDebug.value = appDebug.value;
};

const updateEnvKey = (content: string, key: string, value: string) => {
  const lines = content.split("\n");
  let found = false;
  const updated = lines.map((line) => {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) return line;
    let exportPrefix = "";
    let working = trimmed;
    if (working.startsWith("export ")) {
      exportPrefix = "export ";
      working = working.slice(7).trim();
    }
    if (!working.startsWith(`${key}=`)) {
      return line;
    }
    found = true;
    const commentIndex = line.indexOf(" #");
    const comment = commentIndex >= 0 ? line.slice(commentIndex) : "";
    return `${exportPrefix}${key}=${value}${comment}`;
  });
  if (!found) {
    updated.push(`${key}=${value}`);
  }
  return updated.join("\n");
};

const applyEnvSettings = async () => {
  if (!envReady.value) return;
  if (
    !window.confirm(
      "Changing the environment settings will restart the dev watcher processes. Do you want to proceed?"
    )
  ) {
    return;
  }
  savingEnv.value = true;
  envStatus.value = "Saving...";
  envStatusTone.value = "text-sky-200/80";
  try {
    let next = envContent.value;
    next = updateEnvKey(next, "DB_QUERY_LOGGING", dbQueryLogging.value ? "true" : "false");
    next = updateEnvKey(next, "APP_DEBUG", appDebug.value);
    const res = await fetch("/__lighthouse/api/env?file=.env", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content: next }),
    });
    if (!res.ok) {
      envStatus.value = "Failed to save .env.";
      envStatusTone.value = "text-red-300/80";
      return;
    }
    envContent.value = next;
    envStatus.value = "Saved.";
    envStatusTone.value = "text-emerald-200/80";
    baseDbQueryLogging.value = dbQueryLogging.value;
    baseAppDebug.value = appDebug.value;
    if (store.state.devwatchConnected) {
      envStatus.value = "Saved. Restarting watchers...";
      envStatusTone.value = "text-emerald-200/80";
      await store.sendDevwatchControl("restart");
    }
    window.setTimeout(() => {
      if (envStatus.value === "Saved." || envStatus.value === "Saved. Restarting watchers...") {
        envStatus.value = "";
        envStatusTone.value = "text-muted";
      }
    }, 2500);
  } finally {
    savingEnv.value = false;
  }
};

watch(
  watcherTabs,
  (tabs) => {
    const map = { ...followTailByTab.value };
    const scrollMap = { ...scrollTopByTab.value };
    tabs.forEach((tab) => {
      if (map[tab] === undefined) {
        map[tab] = true;
      }
      if (scrollMap[tab] === undefined) {
        scrollMap[tab] = 0;
      }
    });
    followTailByTab.value = map;
    scrollTopByTab.value = scrollMap;
    if (!tabs.includes(activeTab.value)) {
      activeTab.value = "All";
    }
    const tabParam = typeof route.query.tab === "string" ? route.query.tab : "";
    if (tabParam && tabs.includes(tabParam)) {
      activeTab.value = tabParam;
    }
  },
  { immediate: true }
);

watch(
  () => activeTab.value,
  async (tab) => {
    await nextTick();
    const el = terminalRef.value;
    if (!el) return;
    if (tab && tab !== route.query.tab) {
      router.replace({ query: { ...route.query, tab } });
    }
    if (followTailByTab.value[activeTab.value] !== false) {
      scheduleRender();
      return;
    }
    const saved = scrollTopByTab.value[activeTab.value];
    if (typeof saved === "number") {
      el.scrollTop = saved;
    }
    scheduleRender();
  }
);

watch(filterText, (value) => {
  scheduleRender();
  const normalized = value.trim();
  const current =
    typeof route.query.filter === "string" ? route.query.filter : "";
  if (normalized === current) {
    return;
  }
  const nextQuery = { ...route.query };
  if (normalized) {
    nextQuery.filter = normalized;
  } else {
    delete nextQuery.filter;
  }
  router.replace({ query: nextQuery });
});

watch(
  () => route.query.filter,
  (value) => {
    const next = typeof value === "string" ? value : "";
    if (next === filterText.value) {
      return;
    }
    filterText.value = next;
  },
  { immediate: true }
);

const restart = () => {
  if (
    !window.confirm(
      "Restarting the watchers will stop all running processes in your project. Do you want to continue?"
    )
  ) {
    return;
  }
    store.sendDevwatchControl("restart");
  resetTerminalState();
  window.setTimeout(() => {
    store.disconnectDevwatch();
    store.connectDevwatch();
  }, 300);
  toast("Restart request sent", {
    description: "Dev watchers are restarting.",
  });
};

const scrollToBottom = () => {
  const el = terminalRef.value;
  if (!el) return;
  suppressScroll = true;
  el.scrollTop = el.scrollHeight;
  requestAnimationFrame(() => {
    suppressScroll = false;
  });
};

const showFollowHint = computed(() => followTailByTab.value[activeTab.value] === false);

const handleScroll = () => {
  if (suppressScroll) {
    return;
  }
  const el = terminalRef.value;
  if (!el) return;
  const atBottom = el.scrollTop + el.clientHeight + 2 >= el.scrollHeight;
  followTailByTab.value = {
    ...followTailByTab.value,
    [activeTab.value]: atBottom,
  };
  scrollTopByTab.value = {
    ...scrollTopByTab.value,
    [activeTab.value]: el.scrollTop,
  };
};

const resumeFollow = () => {
  followTailByTab.value[activeTab.value] = true;
  scheduleRender();
};

const focusFilter = () => {
  const el = (filterInput.value as any)?.$el as HTMLInputElement | undefined;
  if (!el) {
    return;
  }
  el.focus();
  el.select();
};

const handleKeydown = (event: KeyboardEvent) => {
  const key = event.key.toLowerCase();
  const code = event.code;
  if (event.repeat) {
    return;
  }
  const target = event.target as HTMLElement | null;
  if (!target) {
    return;
  }
  const tag = target.tagName.toUpperCase();
  if (
    tag === "INPUT" ||
    tag === "TEXTAREA" ||
    tag === "SELECT" ||
    target.isContentEditable
  ) {
    return;
  }
  const isRenderHotkey =
    (
      event.ctrlKey &&
      event.shiftKey &&
      !event.metaKey &&
      !event.altKey &&
      (key === "r" || code === "KeyR")
    ) ||
    (
      event.ctrlKey &&
      event.altKey &&
      !event.metaKey &&
      !event.shiftKey &&
      (key === "r" || code === "KeyR")
    );
  if (isRenderHotkey) {
    if (!devwatchConnected.value) {
      return;
    }
    event.preventDefault();
    store.sendDevwatchControl("render");
    toast("Render request sent", {
      description: "Running forj render.",
    });
    return;
  }
  if (event.metaKey || event.ctrlKey || event.altKey) {
    return;
  }
  if (/^[1-9]$/.test(key)) {
    const index = Number.parseInt(key, 10) - 1;
    const nextTab = watcherTabs.value[index];
    if (!nextTab) {
      return;
    }
    event.preventDefault();
    activeTab.value = nextTab;
    return;
  }
  if (key === "/") {
    event.preventDefault();
    focusFilter();
    return;
  }
  if (key === "escape") {
    if (filterText.value) {
      event.preventDefault();
      filterText.value = "";
    }
    return;
  }
  if (key !== "r" && key !== "p") {
    return;
  }
  if (!devwatchConnected.value) {
    return;
  }
  event.preventDefault();
  if (key === "r") {
    restart();
    return;
  }
  togglePause();
};

onMounted(() => {
  unsubscribeDevwatch = store.subscribeDevwatch(handleDevwatchUpdate);
  if (store.state.devwatch.length > 0) {
    handleSnapshot({ lines: store.state.devwatch });
  }
  store.connectDevwatch();
  loadEnv();
  nextTick(() => {
    const el = terminalRef.value;
    if (el) {
      el.addEventListener("scroll", handleScroll, { passive: true });
    }
  });
  window.addEventListener("keydown", handleKeydown, { capture: true });
});

onUnmounted(() => {
  const el = terminalRef.value;
  if (el) {
    el.removeEventListener("scroll", handleScroll);
  }
  window.removeEventListener("keydown", handleKeydown, { capture: true });
  unsubscribeDevwatch?.();
  store.disconnectDevwatch();
});
</script>
