<template>
  <div class="space-y-1">
    <p v-if="label" class="text-[10px] uppercase tracking-[0.3em] text-muted">{{ label }}</p>
    <slot />
    <p v-if="description" class="text-xs text-muted">{{ description }}</p>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{
  label?: string;
  description?: string;
}>()
</script>
