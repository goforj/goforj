<template>
  <div>
    <PageHeader label="Platform" title="Dev Watcher">
      <template #right>
        <Button :disabled="!devwatchConnected" @click="restart">
          Restart Watchers
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
                    {{ tab }}
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
            <div v-if="lines.length === 0" class="text-xs text-muted">Waiting for watcher output…</div>
            <div v-else class="terminal-lines">
              <div v-for="line in lines" :key="line.key" class="terminal-line" v-html="line.html" />
            </div>
            <div v-if="!currentFollowTail" class="terminal-follow-wrap">
              <button class="terminal-follow" @click="resumeFollow">
                Continue Watch
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
import { useDevconsoleStore } from "../stores/devconsole";
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

const store = useDevconsoleStore();
const route = useRoute();
const router = useRouter();
const terminalRef = ref<HTMLElement | null>(null);
const devwatchConnected = computed(() => store.state.devwatchConnected);
const activeTab = ref("All");
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

const normalizeWatcher = (value: string) => {
  return value
    .replace(/\x1b\[[0-9;]*m/g, "")
    .replace(/\r/g, "")
    .trim()
    .replace(/\s+/g, " ");
};

const inferWatcher = (line: string) => {
  if (!line) return "";
  const arrowMatch = line.match(/›\s*([^›]+?)\s*›/);
  if (arrowMatch?.[1]) {
    return normalizeWatcher(arrowMatch[1]);
  }
  const watcherMatch = line.match(/GoForj Watcher\s*·\s*([A-Za-z0-9 _-]+)/i);
  if (watcherMatch?.[1]) {
    return normalizeWatcher(watcherMatch[1]);
  }
  return "";
};

const parsedLines = computed(() => {
  let current = "";
  return store.state.devwatch.map((entry) => {
    const raw = entry.line || "";
    const detected = inferWatcher(raw);
    if (detected) {
      current = detected;
    }
    return {
      html: ansiToHtml(raw),
      watcher: detected || current,
    };
  });
});

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

const watcherTabs = computed(() => {
  const set = new Set<string>();
  parsedLines.value.forEach((line) => {
    if (line.watcher) {
      set.add(line.watcher);
    }
  });
  return ["All", ...Array.from(set).sort((a, b) => a.localeCompare(b))];
});

const hashLine = (value: string) => {
  let hash = 0;
  for (let i = 0; i < value.length; i += 1) {
    hash = (hash << 5) - hash + value.charCodeAt(i);
    hash |= 0;
  }
  return (hash >>> 0).toString(36);
};

const lines = computed(() => {
  const transform = (line: { html: string }) => ({
    key: hashLine(line.html),
    html: line.html,
  });
  if (activeTab.value === "All") {
    return parsedLines.value.map(transform);
  }
  return parsedLines.value
    .filter((line) => line.watcher === activeTab.value)
    .map(transform);
});

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
      requestAnimationFrame(scrollToBottom);
      return;
    }
    const saved = scrollTopByTab.value[activeTab.value];
    if (typeof saved === "number") {
      el.scrollTop = saved;
    }
  }
);
const restart = () => {
  store.sendDevwatchControl("restart");
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
  el.scrollTop = el.scrollHeight;
};

const handleScroll = () => {
  const el = terminalRef.value;
  if (!el) return;
  const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
  followTailByTab.value[activeTab.value] = nearBottom;
  scrollTopByTab.value[activeTab.value] = el.scrollTop;
};

watch(
  () => lines.value.length,
  async () => {
    await nextTick();
    if (followTailByTab.value[activeTab.value] !== false) {
      requestAnimationFrame(scrollToBottom);
    }
  }
);

onMounted(() => {
  store.connectDevwatch();
  loadEnv();
  nextTick(() => {
    const el = terminalRef.value;
    if (el) {
      el.addEventListener("scroll", handleScroll, { passive: true });
    }
  });
});

onUnmounted(() => {
  const el = terminalRef.value;
  if (el) {
    el.removeEventListener("scroll", handleScroll);
  }
  store.disconnectDevwatch();
});

const resumeFollow = () => {
  followTailByTab.value[activeTab.value] = true;
  requestAnimationFrame(scrollToBottom);
};
</script>
