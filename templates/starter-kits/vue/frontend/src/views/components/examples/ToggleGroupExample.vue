<script setup lang="ts">
import { ref } from 'vue'
import { Label } from '@/components/ui/label'
import { Toggle } from '@/components/ui/toggle'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

const channel = ref('internal')
</script>

<template>
  <div class="grid gap-4">
    <div class="grid gap-2">
      <Label>Release channel</Label>
      <ToggleGroup v-model="channel" type="single" variant="outline" class="justify-start">
        <ToggleGroupItem value="internal">Internal</ToggleGroupItem>
        <ToggleGroupItem value="public">Public</ToggleGroupItem>
      </ToggleGroup>
      <p class="text-xs text-muted-foreground">Single select: one item at a time.</p>
    </div>

    <div class="grid gap-2">
      <Label>Panels</Label>
      <div class="flex flex-wrap gap-2">
        <Toggle variant="outline" pressed>Preview</Toggle>
        <Toggle variant="outline">Logs</Toggle>
        <Toggle variant="outline">Metrics</Toggle>
      </div>
      <p class="text-xs text-muted-foreground">Standalone toggles when the options are independent.</p>
    </div>
  </div>
</template>
