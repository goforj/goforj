<template>
  <header class="flex items-center justify-between">
    <div>
      <p class="text-xs uppercase tracking-[0.35em] text-muted">{{ label }}</p>
      <h2 class="mt-2 text-2xl font-semibold text-foreground">{{ title }}</h2>
    </div>
    <div class="flex items-center gap-3">
      <slot name="right" />
    </div>
  </header>
</template>

<script setup lang="ts">
defineProps<{
  label: string;
  title: string;
}>();
</script>
