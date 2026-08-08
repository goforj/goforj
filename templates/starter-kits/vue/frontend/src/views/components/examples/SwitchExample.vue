<script setup lang="ts">
import { ref } from 'vue'
import { Switch } from '@/components/ui/switch'

const enabled = ref(true)
const migrations = ref(true)
</script>

<template>
  <div class="grid gap-3">
    <div class="flex items-start justify-between gap-4 rounded-lg bg-muted/40 p-4">
      <div class="grid gap-1">
        <p class="text-sm font-medium leading-none">Project status</p>
        <p class="text-sm text-muted-foreground">
          A dedicated row suits a simple on or off preference better than a field grid.
        </p>
      </div>
      <div class="flex shrink-0 items-center gap-3">
        <span class="text-sm text-muted-foreground">{{ enabled ? 'Enabled' : 'Disabled' }}</span>
        <Switch v-model="enabled" />
      </div>
    </div>

    <div class="flex items-start justify-between gap-4 rounded-lg bg-muted/40 p-4">
      <div class="grid gap-1">
        <p class="text-sm font-medium leading-none">Run migrations on deploy</p>
        <p class="text-sm text-muted-foreground">Apply pending schema changes as part of each release.</p>
      </div>
      <Switch v-model="migrations" />
    </div>
  </div>
</template>
