<script setup lang="ts">
import { Activity } from '@lucide/vue'
import { Carousel, CarouselContent, CarouselItem, CarouselNext, CarouselPrevious } from '@/components/ui/carousel'
import { Item, ItemContent, ItemDescription, ItemMedia, ItemTitle } from '@/components/ui/item'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import { ScrollArea } from '@/components/ui/scroll-area'

const activity = [
  { title: 'Starter rendered', description: 'Vue shell copied into the generated frontend folder.' },
  { title: 'User authenticated', description: 'The shell can consume /api/v1/auth/me.' },
  { title: 'Theme detected', description: 'Dark mode follows system preference by default.' },
  { title: 'Components installed', description: 'The local src/components/ui tree is ready to import.' },
]

const slides = [
  { title: 'Dashboard baseline', description: 'Cards, charts, tables, and app navigation.' },
  { title: 'Auth surface', description: 'Login, logout, session bootstrap, and user shell.' },
  { title: 'Reference gallery', description: 'Menus, overlays, and form controls.' },
]
</script>

<template>
  <ResizablePanelGroup direction="horizontal" class="min-h-80 rounded-lg border">
    <ResizablePanel :default-size="46" :min-size="38" class="p-4">
      <p class="mb-3 text-sm font-medium leading-none">Activity feed</p>
      <ScrollArea class="h-56 rounded-lg border">
        <div class="grid gap-2 p-3">
          <Item v-for="entry in activity" :key="entry.title" size="sm">
            <ItemMedia variant="icon">
              <Activity class="size-4" />
            </ItemMedia>
            <ItemContent>
              <ItemTitle>{{ entry.title }}</ItemTitle>
              <ItemDescription>{{ entry.description }}</ItemDescription>
            </ItemContent>
          </Item>
        </div>
      </ScrollArea>
    </ResizablePanel>

    <ResizableHandle with-handle />

    <ResizablePanel :default-size="54" :min-size="34" class="p-4">
      <p class="mb-3 text-sm font-medium leading-none">Content carousel</p>
      <Carousel class="w-full">
        <CarouselContent>
          <CarouselItem v-for="slide in slides" :key="slide.title" class="md:basis-1/2">
            <div class="grid h-full gap-2 rounded-lg bg-muted/40 p-4">
              <p class="font-medium">{{ slide.title }}</p>
              <p class="text-sm text-muted-foreground">{{ slide.description }}</p>
            </div>
          </CarouselItem>
        </CarouselContent>
        <CarouselPrevious />
        <CarouselNext />
      </Carousel>
    </ResizablePanel>
  </ResizablePanelGroup>
</template>
