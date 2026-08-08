<script setup lang="ts">
import { cn } from '@/lib/utils'
import ComponentTag from './ComponentTag.vue'

const props = defineProps<{
  /** Optional label for this grouping inside an example. */
  label?: string
  description?: string
  /** The ui components this grouping demonstrates, in PascalCase. */
  components?: string[]
  class?: string
}>()
</script>

<template>
  <div
    data-slot="showcase-row"
    :class="cn('grid content-start gap-4 rounded-lg bg-muted/40 p-4', props.class)"
  >
    <div v-if="label || description || components?.length" class="grid gap-1">
      <div v-if="label || components?.length" class="flex flex-wrap items-center gap-2">
        <p v-if="label" class="text-sm font-medium leading-none">{{ label }}</p>
        <ComponentTag v-if="components?.length" :names="components" />
      </div>
      <p v-if="description" class="text-sm text-muted-foreground">{{ description }}</p>
    </div>
    <slot />
  </div>
</template>
