<script setup lang="ts">
defineProps<{
  lightSrc: string
  darkSrc?: string
  alt?: string
}>()
</script>

<template>
  <span class="shrink-0 place-items-center">
    <img
      :src="lightSrc"
      :alt="alt ?? ''"
      :class="['col-start-1 row-start-1 size-full object-contain', darkSrc && 'dark:hidden']"
    />
    <img
      v-if="darkSrc"
      :src="darkSrc"
      :alt="alt ?? ''"
      class="col-start-1 row-start-1 hidden size-full object-contain dark:block"
    />
  </span>
</template>
