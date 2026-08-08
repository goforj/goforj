<template>
  <section class="grid gap-6">
    <Card class="border-border/60">
      <CardHeader class="gap-4 p-6 md:p-8">
        <div class="flex flex-wrap items-center gap-2">
          <Badge>Local component reference</Badge>
          <Badge variant="outline">Organized by workflow</Badge>
        </div>
        <div class="grid max-w-3xl gap-3">
          <CardTitle class="text-3xl font-semibold tracking-tight md:text-4xl">
            Review the local shadcn-vue library as a set of focused reference pages.
          </CardTitle>
          <CardDescription class="max-w-2xl text-base">
            The reference is split into focused routes so teams can review one category at a time and lift patterns from realistic examples instead of a single oversized catalog.
          </CardDescription>
        </div>
      </CardHeader>
    </Card>

    <Card class="bg-muted/30">
      <CardContent class="flex flex-col gap-3 text-sm text-muted-foreground md:flex-row md:items-center md:justify-between">
        <p>
          These pages focus on local, product-shaped examples. For the full shadcn-vue documentation and component reference, see
          <a href="https://www.shadcn-vue.com/" target="_blank" rel="noreferrer" class="ml-1 font-medium text-foreground underline underline-offset-4">
            shadcn-vue.com
          </a>.
        </p>
        <Button as-child variant="outline" size="sm" class="shrink-0">
          <a href="https://www.shadcn-vue.com/" target="_blank" rel="noreferrer">
            Open docs
            <ArrowRight class="size-4" />
          </a>
        </Button>
      </CardContent>
    </Card>

    <div class="grid gap-6 md:grid-cols-2 xl:grid-cols-4">
      <Card v-for="section in sections" :key="section.url" class="h-full">
        <CardHeader>
          <div class="flex items-center gap-2">
            <component :is="section.icon" class="size-4 text-muted-foreground" />
            <CardTitle class="text-lg font-semibold tracking-tight">{{ section.title }}</CardTitle>
          </div>
          <CardDescription>{{ section.description }}</CardDescription>
        </CardHeader>
        <CardContent>
          <ul class="grid gap-2 text-sm text-muted-foreground">
            <li v-for="item in section.highlights" :key="item">{{ item }}</li>
          </ul>
        </CardContent>
        <CardFooter>
          <Button as-child class="w-full justify-between">
            <RouterLink :to="section.url">
              Open {{ section.title }}
              <ArrowRight class="size-4" />
            </RouterLink>
          </Button>
        </CardFooter>
      </Card>
    </div>

    <div class="grid gap-6">
      <Specimen
        name="Badge"
        description="Compact status labels. The four variants carry the status vocabulary the rest of the kit uses."
        :source="BadgeExampleSource"
      >
        <BadgeExample />
      </Specimen>

      <Specimen
        name="Alert"
        description="An inline message with an icon, title, and body. Use for page-level notices rather than transient feedback."
        :source="AlertExampleSource"
      >
        <AlertExample />
      </Specimen>

      <Specimen
        name="Item"
        description="A row with media, content, and trailing actions. The outline and muted variants cover most list surfaces."
        :also="['Button']"
        :source="ItemExampleSource"
      >
        <ItemExample />
      </Specimen>

      <Specimen
        name="AspectRatio"
        description="Locks a child to a fixed ratio. Use for media slots, embeds, and thumbnails."
        :source="AspectRatioExampleSource"
      >
        <AspectRatioExample />
      </Specimen>

      <Specimen
        name="Skeleton"
        description="Placeholder blocks that hold layout while content loads."
        :also="['Spinner', 'Badge']"
        :source="SkeletonExampleSource"
      >
        <SkeletonExample />
      </Specimen>

      <Specimen
        name="Empty"
        description="The zero-state surface: media, title, description, and one action."
        :also="['Avatar', 'Button']"
        :source="EmptyExampleSource"
      >
        <EmptyExample />
      </Specimen>
    </div>
  </section>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { ArrowRight, Database, LayoutTemplate, MousePointerClick, Workflow } from '@lucide/vue'
import BadgeExample from './examples/BadgeExample.vue'
import BadgeExampleSource from './examples/BadgeExample.vue?raw'
import AlertExample from './examples/AlertExample.vue'
import AlertExampleSource from './examples/AlertExample.vue?raw'
import ItemExample from './examples/ItemExample.vue'
import ItemExampleSource from './examples/ItemExample.vue?raw'
import AspectRatioExample from './examples/AspectRatioExample.vue'
import AspectRatioExampleSource from './examples/AspectRatioExample.vue?raw'
import SkeletonExample from './examples/SkeletonExample.vue'
import SkeletonExampleSource from './examples/SkeletonExample.vue?raw'
import EmptyExample from './examples/EmptyExample.vue'
import EmptyExampleSource from './examples/EmptyExample.vue?raw'
import { Specimen } from '@/components/showcase'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'


const sections = [
  {
    title: 'Forms',
    url: '/components/forms',
    icon: Workflow,
    description: 'Validation, field wrappers, selects, tags, OTP, and staged setup flows.',
    highlights: ['Vee-validate forms', 'Combobox and selects', 'Tags, OTP, and PIN inputs'],
  },
  {
    title: 'Navigation',
    url: '/components/navigation',
    icon: LayoutTemplate,
    description: 'Menus, disclosures, split panes, scroll containers, and content carousels.',
    highlights: ['Menubar and dropdowns', 'Context and popover menus', 'Resizable and scrollable surfaces'],
  },
  {
    title: 'Overlays',
    url: '/components/overlays',
    icon: MousePointerClick,
    description: 'Dialogs, sheets, drawers, command palette, and toast feedback patterns.',
    highlights: ['Invite and destructive dialogs', 'Sheet and drawer patterns', 'Command and sonner usage'],
  },
  {
    title: 'Data',
    url: '/components/data',
    icon: Database,
    description: 'Tables, pagination, calendars, and scheduling-oriented reference screens.',
    highlights: ['Admin-style tables', 'Pagination primitives', 'Single-date and range calendars'],
  },
]
</script>
