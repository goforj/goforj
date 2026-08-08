<script setup lang="ts">
import { Button } from '@/components/ui/button'
import { Item, ItemContent, ItemDescription, ItemTitle } from '@/components/ui/item'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'

const events = [
  { title: 'Release promoted', description: 'Production rollout completed without errors.' },
  { title: 'Health check recovered', description: 'An API spike resolved after autoscaling.' },
  { title: 'Audit trail captured', description: 'Configuration changes were recorded.' },
]
</script>

<template>
  <Sheet>
    <SheetTrigger as-child>
      <Button variant="outline">Open side sheet</Button>
    </SheetTrigger>

    <SheetContent class="flex w-full flex-col gap-6 p-6 sm:max-w-md">
      <SheetHeader class="gap-2 border-b p-0 pb-5 pr-10">
        <SheetTitle>Inspector panel</SheetTitle>
        <SheetDescription>
          Sheets suit detail views and side tasks that should not interrupt the page.
        </SheetDescription>
      </SheetHeader>

      <ScrollArea class="-mr-2 min-h-0 flex-1 pr-2">
        <div class="grid gap-4">
          <div class="grid gap-1 rounded-lg bg-muted/40 p-4">
            <p class="text-sm font-medium leading-none">Deployment status</p>
            <p class="text-2xl font-semibold tracking-tight">Healthy</p>
            <p class="text-sm text-muted-foreground">No active incidents in the last 24 hours.</p>
          </div>

          <Item v-for="event in events" :key="event.title" variant="outline" size="sm">
            <ItemContent>
              <ItemTitle>{{ event.title }}</ItemTitle>
              <ItemDescription>{{ event.description }}</ItemDescription>
            </ItemContent>
          </Item>
        </div>
      </ScrollArea>

      <div class="flex flex-wrap gap-2 border-t pt-4">
        <Button>Open logs</Button>
        <Button variant="outline">Restart worker</Button>
      </div>
    </SheetContent>
  </Sheet>
</template>
