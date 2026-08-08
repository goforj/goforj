<script setup lang="ts">
import { LoaderCircle } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
</script>

<template>
  <div class="grid gap-4">
    <div class="flex flex-wrap items-center gap-2">
      <Badge>
        <LoaderCircle class="size-3.5 animate-spin" />
        Syncing
      </Badge>
      <Badge variant="secondary">
        <LoaderCircle class="size-3.5 animate-spin" />
        Updating
      </Badge>
      <Badge variant="outline">
        <LoaderCircle class="size-3.5 animate-spin" />
        Loading
      </Badge>
    </div>

    <div class="grid gap-3">
      <Skeleton class="h-4 w-1/2" />
      <Skeleton class="h-4 w-3/4" />
      <div class="flex items-center gap-3">
        <Spinner class="size-5" />
        <p class="text-sm text-muted-foreground">Loading primitives fit into any shell.</p>
      </div>
    </div>
  </div>
</template>
