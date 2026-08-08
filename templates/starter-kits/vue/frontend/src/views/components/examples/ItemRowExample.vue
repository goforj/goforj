<script setup lang="ts">
import { BadgeCheck, ChevronRight } from '@lucide/vue'
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
      <ItemContent>
        <ItemTitle>Two-factor authentication</ItemTitle>
        <ItemDescription>Verify via email or phone number.</ItemDescription>
      </ItemContent>
      <ItemActions>
        <Button size="sm">Enable</Button>
      </ItemActions>
    </Item>

    <Item as="a" href="#" variant="outline" size="sm">
      <ItemMedia>
        <BadgeCheck class="size-5" />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Your profile has been verified.</ItemTitle>
      </ItemContent>
      <ItemActions>
        <ChevronRight class="size-4" />
      </ItemActions>
    </Item>
  </div>
</template>
