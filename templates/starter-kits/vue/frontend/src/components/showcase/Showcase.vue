<script setup lang="ts">
import { computed } from 'vue'
import SpecimenSource from './SpecimenSource.vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { slugify } from '.'

const props = defineProps<{
  /** Section heading. Also used to derive the anchor id. */
  title: string
  /** One or two sentences on when to reach for this pattern. */
  description?: string
  /**
   * Source of the composed example, imported with Vite's `?raw` suffix from
   * the file the default slot renders. A composition is the most useful thing
   * on a reference page to copy, so it shows its source like a specimen does.
   */
  source?: string
  /** Path shown alongside the source. Composed examples have no single module. */
  sourcePath?: string
  class?: string
  contentClass?: string
}>()

const anchor = computed(() => slugify(props.title))
</script>

<template>
  <Card :id="anchor" :class="cn('scroll-mt-24', props.class)" data-slot="showcase">
    <CardHeader class="gap-2">
      <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <CardTitle class="text-lg font-semibold tracking-tight">
          <a :href="`#${anchor}`" class="hover:underline underline-offset-4">{{ title }}</a>
        </CardTitle>
        <SpecimenSource
          v-if="source"
          :name="title"
          :path="sourcePath ?? ''"
          :source="source"
        />
      </div>
      <CardDescription v-if="description">
        {{ description }}
      </CardDescription>
    </CardHeader>

    <CardContent :class="cn('grid gap-6', props.contentClass)">
      <slot />
    </CardContent>
  </Card>
</template>
