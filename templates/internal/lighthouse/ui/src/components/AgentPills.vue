<template>
  <div class="flex flex-wrap items-center gap-2">
    <span
      v-if="agents.length === 0"
      class="inline-flex min-w-max items-center gap-1.5 rounded-full border border-border px-2.5 py-1 text-xs text-muted-foreground"
    >
      No agents
    </span>
    <span
      v-for="agent in agents"
      :key="agent.id + agent.source"
      class="inline-flex min-w-max items-center gap-1.5 rounded-full border border-border px-2.5 py-1 text-xs"
      :title="formatConnected(agent.connected_at)"
    >
      <span
        class="inline-flex h-2 w-2 rounded-full"
        :class="isStale(agent) ? 'bg-amber-400/70' : 'bg-emerald-400/80'"
      ></span>
      <span class="text-muted-foreground">{{ agent.source }}</span>
      <span class="font-semibold text-foreground">{{ formatUptime(agent.started_at) }}</span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useLighthouseStore } from "../stores/lighthouse";

const store = useLighthouseStore();
const agents = computed(() =>
  [...store.state.agents].sort((a, b) => a.source.localeCompare(b.source))
);

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
