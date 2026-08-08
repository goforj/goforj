<script setup lang="ts">
import { Badge } from '@/components/ui/badge'
</script>

<template>
  <div class="flex flex-wrap items-center gap-2">
    <Badge>Ready</Badge>
    <Badge variant="secondary">Queued</Badge>
    <Badge variant="outline">Draft</Badge>
    <Badge variant="destructive">Blocked</Badge>
  </div>
</template>
