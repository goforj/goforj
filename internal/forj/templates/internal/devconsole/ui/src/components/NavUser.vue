<template>
  <div class="rounded-xl border border-border/70 bg-white/5 px-3 py-3">
    <button
      class="mt-1 flex w-full items-center justify-center gap-2 rounded-lg border border-border/70 bg-white/5 px-3 py-2 text-xs text-white/80 transition hover:border-white/20 hover:text-white active:translate-y-[0.5px] active:scale-[0.98]"
      @click="$emit('logout')"
    >
      <LogOut class="h-3.5 w-3.5" />
      Logout
    </button>
  </div>
</template>

<script setup lang="ts">
import { LogOut } from "lucide-vue-next";

defineEmits<{
  (event: "logout"): void;
}>();
</script>
