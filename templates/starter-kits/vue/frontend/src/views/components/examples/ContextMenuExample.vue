<script setup lang="ts">
import { ref } from 'vue'
import {
  ContextMenu,
  ContextMenuCheckboxItem,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuRadioGroup,
  ContextMenuRadioItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'

const showHints = ref(true)
const region = ref('iad')
</script>

<template>
  <ContextMenu>
    <ContextMenuTrigger
      class="flex min-h-40 items-center justify-center rounded-lg border border-dashed bg-muted/30 p-6 text-sm text-muted-foreground"
    >
      Right click this deployment surface
    </ContextMenuTrigger>
    <ContextMenuContent class="w-60">
      <ContextMenuLabel>Deployment</ContextMenuLabel>
      <ContextMenuItem inset>Open logs</ContextMenuItem>
      <ContextMenuItem inset>Restart worker</ContextMenuItem>
      <ContextMenuSeparator />
      <ContextMenuCheckboxItem v-model:checked="showHints">Show hints</ContextMenuCheckboxItem>
      <ContextMenuSeparator />
      <ContextMenuLabel>Region</ContextMenuLabel>
      <ContextMenuRadioGroup v-model="region">
        <ContextMenuRadioItem value="iad">US East</ContextMenuRadioItem>
        <ContextMenuRadioItem value="sfo">US West</ContextMenuRadioItem>
        <ContextMenuRadioItem value="fra">Europe</ContextMenuRadioItem>
      </ContextMenuRadioGroup>
    </ContextMenuContent>
  </ContextMenu>
</template>
