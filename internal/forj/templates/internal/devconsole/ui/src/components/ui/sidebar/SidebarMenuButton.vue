<template>
  <component
    :is="asChild ? 'span' : 'button'"
    class="sidebar-menu-button"
    data-slot="sidebar-menu-button"
  >
    <slot />
  </component>
</template>

<script setup lang="ts">
type Props = {
  asChild?: boolean;
};

defineProps<Props>();
</script>
