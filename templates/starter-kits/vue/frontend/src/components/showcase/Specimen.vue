<script setup lang="ts">
import { computed } from 'vue'
import SpecimenSource from './SpecimenSource.vue'
import { cn } from '@/lib/utils'
import { componentImportPath, slugify } from '.'

const props = defineProps<{
  /** The ui component this specimen demonstrates, in PascalCase. */
  name: string
  /** One line on when to reach for it. */
  description?: string
  /** Extra component names this specimen also relies on. */
  also?: string[]
  /**
   * Source of the rendered example, imported with Vite's `?raw` suffix from
   * the very same file the default slot renders. Importing one file both
   * ways is what keeps the listing and the live example from drifting.
   */
  source?: string
  class?: string
  contentClass?: string
}>()

const path = computed(() => componentImportPath(props.name))
const anchor = computed(() => slugify(props.name))
</script>

<template>
  <section
    :id="anchor"
    data-slot="specimen"
    :data-import="path"
    :class="cn('scroll-mt-24 overflow-hidden rounded-xl border bg-card', props.class)"
  >
    <header class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 border-b px-4 py-2.5">
      <h2 class="text-sm font-medium text-foreground">
        <a :href="`#${anchor}`" class="hover:underline underline-offset-4">{{ name }}</a>
      </h2>
      <SpecimenSource v-if="source" :name="name" :path="path" :source="source" />
    </header>

    <div class="grid gap-4 p-4">
      <p v-if="description" class="text-sm text-muted-foreground">{{ description }}</p>
      <p v-if="also?.length" class="-mt-1 flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
        <span>Composes with</span>
        <code
          v-for="extra in also"
          :key="extra"
          class="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px]"
          :data-import="componentImportPath(extra)"
        >{{ extra }}</code>
      </p>
      <div :class="cn('grid gap-4', props.contentClass)">
        <slot />
      </div>
    </div>
  </section>
</template>
