<script setup lang="ts">
import { computed } from 'vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { slugify } from '.'

const props = defineProps<{
  /** Section heading. Also used to derive the anchor id. */
  title: string
  /** One or two sentences on when to reach for this pattern. */
  description?: string
  class?: string
  contentClass?: string
}>()

const anchor = computed(() => slugify(props.title))
</script>

<template>
  <Card :id="anchor" :class="cn('scroll-mt-24', props.class)" data-slot="showcase">
    <CardHeader class="gap-2">
      <CardTitle class="text-lg font-semibold tracking-tight">
        <a :href="`#${anchor}`" class="hover:underline underline-offset-4">{{ title }}</a>
      </CardTitle>
      <CardDescription v-if="description">
        {{ description }}
      </CardDescription>
    </CardHeader>

    <CardContent :class="cn('grid gap-6', props.contentClass)">
      <slot />
    </CardContent>
  </Card>
</template>
