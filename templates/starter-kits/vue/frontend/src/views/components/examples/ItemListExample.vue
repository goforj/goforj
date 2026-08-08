<script setup lang="ts">
import { Badge } from '@/components/ui/badge'
import { Item, ItemActions, ItemContent, ItemDescription, ItemTitle } from '@/components/ui/item'

const jobs = [
  { title: 'Export component inventory', description: 'Generate a report of installed UI primitives.', status: 'Queued', variant: 'secondary' as const },
  { title: 'Render preview build', description: 'Assemble a production bundle for design review.', status: 'Running', variant: 'default' as const },
  { title: 'Sync usage analytics', description: 'Blocked until the metrics query logger lands.', status: 'Blocked', variant: 'destructive' as const },
]
</script>

<template>
  <div class="grid gap-3">
    <Item v-for="job in jobs" :key="job.title" variant="outline" size="sm">
      <ItemContent>
        <ItemTitle>{{ job.title }}</ItemTitle>
        <ItemDescription>{{ job.description }}</ItemDescription>
      </ItemContent>
      <ItemActions>
        <Badge :variant="job.variant">{{ job.status }}</Badge>
      </ItemActions>
    </Item>
  </div>
</template>
