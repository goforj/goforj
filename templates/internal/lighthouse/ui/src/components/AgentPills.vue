<template>
  <div class="flex max-w-[min(52vw,720px)] items-center gap-2 overflow-x-auto whitespace-nowrap">
    <span
      v-if="agentGroups.length === 0"
      class="inline-flex min-w-max items-center gap-1.5 rounded-full border border-border px-2.5 py-1 text-xs text-muted-foreground"
    >
      No agents
    </span>
    <span
      v-for="group in agentGroups"
      :key="group.key"
      class="inline-flex min-w-max items-center gap-1.5 rounded-full border border-border px-2.5 py-1 text-xs"
      :title="groupTooltip(group)"
    >
      <span
        class="inline-flex h-2 w-2 rounded-full"
        :class="group.stale ? 'bg-amber-400/70' : 'bg-emerald-400/80'"
      ></span>
      <span class="text-muted-foreground">{{ group.label }}</span>
      <span
        v-if="group.count > 1"
        class="rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-semibold leading-none text-foreground"
      >
        {{ group.count }}
      </span>
      <span class="font-semibold text-foreground">{{ formatUptime(group.started_at) }}</span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useLighthouseStore } from "../stores/lighthouse";

const store = useLighthouseStore();
const agentGroups = computed(() => groupAgents(store.state.agents));

const now = ref(Date.now());
let tickHandle: number | null = null;

onMounted(() => {
  tickHandle = window.setInterval(() => {
    now.value = Date.now();
  }, 10000);
});

onBeforeUnmount(() => {
  if (tickHandle) {
    window.clearInterval(tickHandle);
  }
});

const isStale = (agent: any) => {
  const staleAfterMs = 45000;
  if (agent.last_seen) {
    const seenAt = new Date(agent.last_seen).getTime();
    if (!Number.isNaN(seenAt)) {
      return Date.now() - seenAt > staleAfterMs;
    }
  }
  if (agent.connected_at) {
    const connectedAt = new Date(agent.connected_at).getTime();
    if (!Number.isNaN(connectedAt)) {
      return Date.now() - connectedAt > staleAfterMs;
    }
  }
  return true;
};

const groupAgents = (agents: any[]) => {
  const groups = new Map<string, any[]>();
  for (const agent of agents) {
    const key = String(agent.group_key || agent.source || "app").trim();
    if (!groups.has(key)) {
      groups.set(key, []);
    }
    groups.get(key)?.push(agent);
  }

  return Array.from(groups.entries())
    .map(([key, items]) => {
      const sorted = [...items].sort((a, b) => String(a.instance_key || a.host || a.source).localeCompare(String(b.instance_key || b.host || b.source)));
      const freshest = [...sorted].sort((a, b) => timestampValue(b.last_seen || b.connected_at) - timestampValue(a.last_seen || a.connected_at))[0] || sorted[0];
      return {
        key,
        label: groupLabel(freshest, key),
        count: sorted.length,
        agents: sorted,
        started_at: freshest?.started_at,
        stale: sorted.every(isStale),
      };
    })
    .sort((a, b) => a.label.localeCompare(b.label));
};

const groupLabel = (agent: any, fallback: string) => {
  const appTarget = String(agent?.app_target || "app").trim();
  const runtimeSource = String(agent?.runtime_source || agent?.source || fallback || "app").trim();
  if (!appTarget || appTarget === "app") {
    return runtimeSource.split("/")[0] || "app";
  }
  return `${appTarget}/${runtimeSource}`;
};

const timestampValue = (value?: string) => {
  if (!value) return 0;
  const parsed = new Date(value).getTime();
  return Number.isFinite(parsed) ? parsed : 0;
};

const groupTooltip = (group: any) => {
  const lines = group.agents.map((agent: any) => {
    const instance = String(agent.instance_key || agent.instance_id || agent.host || agent.key || "").trim();
    const connected = formatConnected(agent.connected_at);
    return instance ? `${group.label} / ${instance} · ${connected}` : `${group.label} · ${connected}`;
  });
  return lines.join("\n");
};

const formatUptime = (startedAt?: string) => {
  if (!startedAt) return "unknown";
  const start = new Date(startedAt).getTime();
  if (Number.isNaN(start)) return "unknown";
  const seconds = Math.max(0, Math.floor((now.value - start) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  return `${days}d`;
};

const formatConnected = (connectedAt?: string) => {
  if (!connectedAt) return "Connected: unknown";
  const connected = new Date(connectedAt).getTime();
  if (Number.isNaN(connected)) return "Connected: unknown";
  const seconds = Math.max(0, Math.floor((now.value - connected) / 1000));
  if (seconds < 60) return `Connected: ${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `Connected: ${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `Connected: ${hours}h`;
  const days = Math.floor(hours / 24);
  return `Connected: ${days}d`;
};
</script>
