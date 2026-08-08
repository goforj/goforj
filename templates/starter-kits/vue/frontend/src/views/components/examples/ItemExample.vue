<script setup lang="ts">
import { Layers3, Sparkles } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemMedia,
  ItemTitle,
} from '@/components/ui/item'
</script>

<template>
  <div class="grid gap-3">
    <Item variant="outline">
      <ItemMedia variant="icon">
        <Layers3 class="size-4" />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Application shell</ItemTitle>
        <ItemDescription>
          Sidebar, app header, auth bootstrap, and local UI source ship together.
        </ItemDescription>
      </ItemContent>
      <ItemActions>
        <Button variant="ghost" size="sm">Open</Button>
      </ItemActions>
    </Item>

    <Item variant="muted">
      <ItemMedia variant="icon">
        <Sparkles class="size-4" />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Theme-aware by default</ItemTitle>
        <ItemDescription>
          Dark mode follows system preference and the light palette stays readable.
        </ItemDescription>
      </ItemContent>
    </Item>
  </div>
</template>
