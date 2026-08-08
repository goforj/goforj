<script setup lang="ts">
import { cn } from '@/lib/utils'

// One treatment for every form result in the app, so success does not read as
// less consequential than failure purely because of how it is styled.
const props = defineProps<{
  variant?: 'error' | 'success'
  class?: string
}>()
</script>

<template>
  <p
    role="status"
    :class="cn(
      'rounded-lg border px-3 py-2 text-sm',
      props.variant === 'success'
        ? 'border-success/30 bg-success/10 text-success'
        : 'border-destructive/30 bg-destructive/10 text-destructive',
      props.class,
    )"
  >
    <slot />
  </p>
</template>
