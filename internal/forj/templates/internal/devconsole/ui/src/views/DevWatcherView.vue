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
            <Tabs v-model="activeTab">
              <TabsList>
                <TabsTrigger v-for="tab in watcherTabs" :key="tab" :value="tab">
                  {{ tab }}
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </template>
        </CardHeader>
        <CardContent>
          <div ref="terminalRef" class="terminal-pane">
            <div v-if="lines.length === 0" class="text-xs text-muted">Waiting for watcher output…</div>
            <div v-else class="terminal-lines">
              <div v-for="(line, idx) in lines" :key="idx" class="terminal-line" v-html="line" />
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
const followTailByTab = ref<Record<string, boolean>>({});
const scrollTopByTab = ref<Record<string, number>>({});
const currentFollowTail = computed(() => followTailByTab.value[activeTab.value] !== false);

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

const watcherTabs = computed(() => {
  const set = new Set<string>();
  parsedLines.value.forEach((line) => {
    if (line.watcher) {
      set.add(line.watcher);
    }
  });
  return ["All", ...Array.from(set).sort((a, b) => a.localeCompare(b))];
});

const lines = computed(() => {
  if (activeTab.value === "All") {
    return parsedLines.value.map((line) => line.html);
  }
  return parsedLines.value
    .filter((line) => line.watcher === activeTab.value)
    .map((line) => line.html);
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
