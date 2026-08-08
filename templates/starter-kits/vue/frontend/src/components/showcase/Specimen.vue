<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'
import { componentImportPath, slugify } from '.'

const props = defineProps<{
  /** The ui component this specimen demonstrates, in PascalCase. */
  name: string
  /** One line on when to reach for it. */
  description?: string
  /** Extra component names this specimen also relies on. */
  also?: string[]
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
    :class="cn('scroll-mt-24 overflow-hidden rounded-xl border bg-card', props.class)"
  >
    <header class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 border-b px-4 py-2.5">
      <a :href="`#${anchor}`" class="text-sm font-medium text-foreground hover:underline underline-offset-4">
        {{ name }}
      </a>
      <code class="font-mono text-[11px] text-muted-foreground" :data-import="path">{{ path }}</code>
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
