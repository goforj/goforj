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

    <div class="grid gap-6 xl:grid-cols-2">
      <Specimen
        name="Badge"
        description="Compact status labels. The four variants carry the status vocabulary the rest of the kit uses."
      >
        <div class="flex flex-wrap items-center gap-2">
          <Badge>Ready</Badge>
          <Badge variant="secondary">Queued</Badge>
          <Badge variant="outline">Draft</Badge>
          <Badge variant="destructive">Blocked</Badge>
        </div>
      </Specimen>

      <Specimen
        name="Alert"
        description="An inline message with an icon, title, and body. Use for page-level notices rather than transient feedback."
      >
        <Alert>
          <CircleCheckBig class="size-4" />
          <AlertTitle>Everything here ships as local source</AlertTitle>
          <AlertDescription>
            The generated app owns these examples, so teams can adapt them directly instead of relying on a remote component catalog.
          </AlertDescription>
        </Alert>
      </Specimen>

      <Specimen
        name="Item"
        description="A row with media, content, and trailing actions. The outline and muted variants cover most list surfaces."
        :also="['Button']"
      >
        <Item variant="outline">
          <ItemMedia variant="icon">
            <Layers3 class="size-4" />
          </ItemMedia>
          <ItemContent>
            <ItemTitle>Application shell</ItemTitle>
            <ItemDescription>Sidebar, app header, auth bootstrap, and local UI source ship together as one coherent starting point.</ItemDescription>
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
            <ItemDescription>Dark mode follows system preference and the light palette stays readable for day-to-day development.</ItemDescription>
          </ItemContent>
        </Item>
      </Specimen>

      <Specimen
        name="AspectRatio"
        description="Locks a child to a fixed ratio. Use for media slots, embeds, and thumbnails."
      >
        <AspectRatio :ratio="16 / 9" class="overflow-hidden rounded-lg border bg-muted">
          <div class="flex h-full items-center justify-center text-sm text-muted-foreground">
            16 / 9 media slot
          </div>
        </AspectRatio>
      </Specimen>

      <Specimen
        name="Skeleton"
        description="Placeholder blocks that hold layout while content loads."
        :also="['Spinner', 'Badge']"
      >
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
      </Specimen>

      <Specimen
        name="Empty"
        description="The zero-state surface: media, title, description, and one action. The dashed border is part of the component."
        :also="['Avatar', 'Button']"
      >
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="default" class="mb-2">
              <div class="flex -space-x-2">
                <Avatar v-for="member in pendingMembers" :key="member" class="ring-background size-8 ring-2">
                  <AvatarFallback>{{ member }}</AvatarFallback>
                </Avatar>
              </div>
            </EmptyMedia>
            <EmptyTitle>No team members added</EmptyTitle>
            <EmptyDescription>
              Invite collaborators when the workspace is ready for shared development.
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button size="sm">
              <Plus class="size-3.5" />
              Invite collaborators
            </Button>
          </EmptyContent>
        </Empty>

        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <LoaderCircle class="size-4 animate-spin" />
            </EmptyMedia>
            <EmptyTitle>Processing request</EmptyTitle>
            <EmptyDescription>
              Please wait while we process your request. Do not refresh the page.
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button variant="outline" size="sm">Cancel</Button>
          </EmptyContent>
        </Empty>
      </Specimen>
    </div>
  </section>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { ArrowRight, CircleCheckBig, Database, Layers3, LayoutTemplate, LoaderCircle, MousePointerClick, Plus, Sparkles, Workflow } from '@lucide/vue'
import { Specimen } from '@/components/showcase'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { AspectRatio } from '@/components/ui/aspect-ratio'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from '@/components/ui/item'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'

const pendingMembers = ['AR', 'MK', 'TS']

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
