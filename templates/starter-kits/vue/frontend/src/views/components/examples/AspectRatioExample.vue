<script setup lang="ts">
import { AspectRatio } from '@/components/ui/aspect-ratio'
</script>

<template>
  <div class="max-w-sm">
    <AspectRatio :ratio="16 / 9" class="overflow-hidden rounded-lg border bg-muted">
      <div class="flex h-full items-center justify-center text-sm text-muted-foreground">
        16 / 9 media slot
      </div>
    </AspectRatio>
  </div>
</template>
