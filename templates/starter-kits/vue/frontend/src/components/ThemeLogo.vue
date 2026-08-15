<script setup lang="ts">
defineProps<{
  lightSrc: string
  darkSrc?: string
  alt?: string
}>()
</script>

<!--
  The root must not be a span. SidebarMenuButton carries
  `group-data-[collapsible=icon]:[&>span]:hidden` to drop label text when the
  sidebar collapses to icons, and that selector is a step more specific than
  anything this component can set on itself — so a span root disappears from
  the collapsed rail no matter what display classes it is given. shadcn's own
  brand pattern wraps the mark in a div for the same reason.
-->
<template>
  <div class="shrink-0 place-items-center">
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
  </div>
</template>
