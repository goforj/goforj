<template>
  <div class="status-pill" :class="connected ? '' : 'text-red-200'">
    <span class="status-dot" :class="connected ? 'bg-emerald-400' : 'bg-red-400'" />
    {{ connected ? "Live" : "Offline" }}
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useDevconsoleStore } from "../stores/devconsole";

const store = useDevconsoleStore();
const connected = computed(() => store.state.socketConnected);
</script>
