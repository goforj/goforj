<script setup lang="ts">
import { Badge } from '@/components/ui/badge'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

defineProps<{
  /** Leading badge, e.g. "Components". */
  eyebrow?: string
  /** Trailing outline badge naming this page, e.g. "Forms". */
  section?: string
  title: string
  description?: string
}>()
</script>

<template>
  <Card class="border-border/60">
    <CardHeader class="gap-3">
      <div v-if="eyebrow || section" class="flex flex-wrap items-center gap-2">
        <Badge v-if="eyebrow">{{ eyebrow }}</Badge>
        <Badge v-if="section" variant="outline">{{ section }}</Badge>
      </div>
      <CardTitle class="text-2xl font-semibold tracking-tight">{{ title }}</CardTitle>
      <CardDescription v-if="description" class="max-w-3xl text-base">
        {{ description }}
      </CardDescription>
    </CardHeader>
  </Card>
</template>
