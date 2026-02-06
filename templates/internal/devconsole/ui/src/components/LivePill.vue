<template>
  <div class="status-pill" :class="connected ? '' : 'text-red-200'">
    <span class="status-dot" :class="connected ? 'status-dot-live' : 'status-dot-offline'" />
    {{ connected ? "Live" : "Offline" }}
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useDevconsoleStore } from "../stores/devconsole";

const store = useDevconsoleStore();
const connected = computed(() => store.state.socketConnected);
</script>
