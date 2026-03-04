<template>
  <div class="inline-flex min-w-max items-center gap-1.5 rounded-full border border-border px-2.5 py-1 text-xs">
    <span
      class="inline-flex h-2 w-2 rounded-full"
      :class="connected ? 'bg-emerald-400' : 'bg-rose-400'"
    />
    <span :class="connected ? 'text-foreground' : 'text-rose-300'">{{ connected ? "Live" : "Offline" }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useLighthouseStore } from "../stores/lighthouse";

const store = useLighthouseStore();
const connected = computed(() => store.state.socketConnected);
</script>
