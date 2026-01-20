<template>
  <div>
    <PageHeader label="Platform" title="Dev Watcher">
      <template #right>
        <Button :disabled="!devwatchConnected" @click="restart">
          Restart Watchers <span class="text-xs opacity-70 pl-1">(r)</span>
        </Button>
        <AgentPills />
        <LivePill />
      </template>
    </PageHeader>

    <section class="mt-8">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Watcher Output</p>
            <CardTitle>Streaming `forj dev` output.</CardTitle>
          </template>
          <template #description>
            <CardDescription>Matches the stdout/stderr you see in the dev watcher.</CardDescription>
          </template>
          <template #action>
            <div class="devwatch-controls">
                <div class="devwatch-settings">
                  <div class="flex items-center gap-2">
                  <span class="text-muted uppercase tracking-[0.2em] text-[10px]">DB</span>
                  <div class="flex items-center gap-1">
                    <button
                      class="pill-toggle"
                      :class="dbQueryLogging ? '' : 'pill-toggle-active'"
                      @click="dbQueryLogging = false"
                    >
                      Off
                    </button>
                    <button
                      class="pill-toggle"
                      :class="dbQueryLogging ? 'pill-toggle-active' : ''"
                      @click="dbQueryLogging = true"
                    >
                      On
                    </button>
                  </div>
                </div>
                <div class="flex items-center gap-2">
                  <span class="text-muted uppercase tracking-[0.2em] text-[10px]">Debug</span>
                  <div class="flex items-center gap-1">
                    <button
                      v-for="level in debugLevels"
                      :key="level"
                      class="pill-toggle"
                      :class="appDebug === level ? 'pill-toggle-active' : ''"
                      @click="appDebug = level"
                    >
                      {{ level }}
                    </button>
                  </div>
                </div>
                <Button :disabled="savingEnv || !envReady || !envDirty" @click="applyEnvSettings">
                  Apply
                </Button>
              </div>
              <Tabs v-model="activeTab">
                  <TabsList>
                    <TabsTrigger v-for="tab in watcherTabs" :key="tab" :value="tab">
                      {{ formatTabLabel(tab) }}
                    </TabsTrigger>
                  </TabsList>
              </Tabs>
            </div>
          </template>
        </CardHeader>
        <CardContent>
          <div v-if="envStatus" class="mb-3 text-xs" :class="envStatusTone">
            {{ envStatus }}
          </div>
          <div ref="terminalRef" class="terminal-pane">
            <div ref="terminalLines" class="terminal-lines">
              <div v-if="!hasTerminalLines" class="text-xs text-muted">Waiting for watcher output…</div>
            </div>
            <div class="terminal-follow-wrap" v-if="showFollowHint">
              <button class="terminal-follow" @click="resumeFollow">
                Watch Output
              </button>
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
  useDevconsoleStore,
  DevwatchLine,
  DevwatchSnapshot,
  DevwatchUpdate,
  EditorTarget,
} from "../stores/devconsole";
import { ansiToHtml } from "../lib/ansi";
import { toast } from "vue-sonner";
import AgentPills from "../components/AgentPills.vue";
import LivePill from "../components/LivePill.vue";
import PageHeader from "../components/PageHeader.vue";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import { Tabs, TabsList, TabsTrigger } from "../components/ui/tabs";

type LineReference = {
  path?: string;
  line?: number;
  symbol?: string;
};

type ManualLine = {
  id: number;
  html: string;
  watcher: string;
  reference?: LineReference;
};

const store = useDevconsoleStore();
const route = useRoute();
const router = useRouter();
const terminalRef = ref<HTMLElement | null>(null);
const terminalLines = ref<HTMLDivElement | null>(null);
const devwatchConnected = computed(() => store.state.devwatchConnected);
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
const formatTabLabel = (tab: string) => {
  if (tab === "All") {
    return tab;
  }
  const count = unreadCounts.value[tab] ?? 0;
  return count > 0 ? `${tab} (${count})` : tab;
};
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
const manualLines: ManualLine[] = [];
const perWatcherLines = new Map<string, ManualLine[]>();
const lineReferenceById = new Map<number, LineReference>();
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
};

const resetTerminalState = () => {
  manualLines.splice(0, manualLines.length);
  perWatcherLines.clear();
  localWatcherList.value = [];
  localWatcherSet.clear();
  clearTerminal();
  unreadCounts.value = {};
  referenceByTab.value = {};
  lineReferenceById.clear();
};

const setLineReference = (id: number, reference?: LineReference) => {
  if (reference && (reference.path || reference.symbol)) {
    lineReferenceById.set(id, reference);
    return;
  }
  lineReferenceById.delete(id);
};

const ansiEscapeRegex = /\u001b\[[0-9;]*m/g;
const fileLineRegex = /(?:\.\/)?([^\s:()]+\.go)(?::(\d+))?/;
const symbolRegex = /#([\w\.]+)/;

const parseReference = (raw: string): LineReference | null => {
  const cleaned = raw.replace(ansiEscapeRegex, "");
  const fileMatch = fileLineRegex.exec(cleaned);
  const symbolMatch = symbolRegex.exec(cleaned);
  const path = fileMatch ? fileMatch[1] : undefined;
  const line = fileMatch && fileMatch[2] ? Number(fileMatch[2]) : undefined;
  const symbol = symbolMatch ? symbolMatch[1] : undefined;
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
    const manualLine: ManualLine = {
      id,
      html: ansiToHtml(raw),
      watcher,
      reference: reference ?? undefined,
    };
    converted.push(manualLine);
    if (reference?.path || reference?.symbol) {
      updateReferenceForLine(watcher, reference);
    }
    setLineReference(id, reference ?? undefined);
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
const renderActiveLines = () => {
  const container = terminalLines.value;
  if (!container) {
    return;
  }
  const source =
    activeTab.value === "All"
      ? manualLines
      : perWatcherLines.get(activeTab.value) ?? [];
  pruneStaleDomEntries(source);
  container.innerHTML = "";
  source.forEach((line) => {
    const node = document.createElement("div");
    node.className = "terminal-line";
    node.dataset.lineId = String(line.id);
    node.dataset.watcher = line.watcher;
    const content = document.createElement("div");
    content.className = "terminal-line-content";
    content.innerHTML = line.html;
    node.appendChild(content);
    const reference = line.reference ?? lineReferenceById.get(line.id);
    if (reference?.path || reference?.symbol) {
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
  lineReferenceById.clear();
  const converted = convertEntries(snapshot.lines.slice(-limit));
  manualLines.push(...converted);
  perWatcherLines.clear();
  manualLines.forEach(pushToWatcherBuffer);
  scheduleRender();
};

const handleAppend = (line: DevwatchLine) => {
  const limit = store.state.devwatchLimit;
  const [converted] = convertEntries([line]);
  manualLines.push(converted);
  pushToWatcherBuffer(converted);
  if (manualLines.length > limit) {
    const removed = manualLines.shift();
    if (removed) {
      lineReferenceById.delete(removed.id);
    }
  }
  markWatcherUnread(converted.watcher);
  scheduleRender();
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
    const res = await fetch("/__devconsole/api/env?file=.env");
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
    const res = await fetch("/__devconsole/api/env?file=.env", {
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

const handleKeydown = (event: KeyboardEvent) => {
  if (
    event.key.toLowerCase() !== "r" ||
    event.repeat ||
    event.metaKey ||
    event.ctrlKey
  ) {
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
  if (!devwatchConnected.value) {
    return;
  }
  event.preventDefault();
  restart();
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
  window.addEventListener("keydown", handleKeydown);
});

onUnmounted(() => {
  const el = terminalRef.value;
  if (el) {
    el.removeEventListener("scroll", handleScroll);
  }
  window.removeEventListener("keydown", handleKeydown);
  unsubscribeDevwatch?.();
  store.disconnectDevwatch();
});
</script>
