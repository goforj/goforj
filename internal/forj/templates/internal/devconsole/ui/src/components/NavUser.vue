<template>
  <div class="rounded-xl border border-border/70 bg-white/5 px-3 py-3">
    <div class="text-xs text-muted">GoForj Console</div>
    <button class="mt-3 w-full rounded-lg border border-border/70 bg-white/5 px-3 py-2 text-xs text-white/80 transition hover:border-white/20 hover:text-white" @click="$emit('logout')">
      Logout
    </button>
  </div>
</template>

<script setup lang="ts">
defineEmits<{
  (event: "logout"): void;
}>();
</script>
