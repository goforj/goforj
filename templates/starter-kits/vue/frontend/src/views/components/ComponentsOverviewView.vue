<template>
  <section class="grid gap-6">
    <Card class="overflow-hidden border-border/60 bg-[linear-gradient(135deg,color-mix(in_oklab,var(--card)_96%,white),var(--background))] shadow-xl dark:bg-[linear-gradient(135deg,hsl(0_0%_5%),hsl(0_0%_8%))]">
      <CardHeader class="gap-4 p-6 md:p-8">
        <div class="flex flex-wrap items-center gap-2">
          <Badge>Local component reference</Badge>
          <Badge variant="outline">Organized by workflow</Badge>
        </div>
        <div class="grid max-w-3xl gap-3">
          <CardTitle class="text-3xl font-semibold tracking-tight md:text-5xl dark:text-white">
            Review the local shadcn-vue library as a set of focused reference pages.
          </CardTitle>
          <CardDescription class="max-w-2xl text-base text-muted-foreground dark:text-slate-300">
            The reference is split into focused routes so teams can review one category at a time and lift patterns from realistic examples instead of a single oversized catalog.
          </CardDescription>
        </div>
      </CardHeader>
    </Card>

    <Card class="border-dashed border-border/70 bg-muted/20">
      <CardContent class="flex flex-col gap-3 p-6 text-sm text-muted-foreground md:flex-row md:items-center md:justify-between">
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

    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <Card v-for="section in sections" :key="section.url" class="h-full">
        <CardHeader>
          <div class="flex items-center gap-2">
            <component :is="section.icon" class="size-4 text-muted-foreground" />
            <CardTitle class="text-xl">{{ section.title }}</CardTitle>
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

    <div class="grid gap-4 xl:grid-cols-[1.1fr_0.9fr]">
      <Card>
        <CardHeader>
          <CardTitle>Surface primitives</CardTitle>
          <CardDescription>Reusable primitives still need a small landing area so teams understand the shell and theme language at a glance.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-5">
          <div class="flex flex-wrap items-center gap-2">
            <Badge>Ready</Badge>
            <Badge variant="secondary">Queued</Badge>
            <Badge variant="outline">Draft</Badge>
            <Badge variant="destructive">Blocked</Badge>
          </div>

          <Alert>
            <CircleCheckBig class="size-4" />
            <AlertTitle>Everything here ships as local source</AlertTitle>
            <AlertDescription>
              The generated app owns these examples, so teams can adapt them directly instead of relying on a remote component catalog.
            </AlertDescription>
          </Alert>

          <AspectRatio :ratio="16 / 9" class="overflow-hidden rounded-xl border bg-muted">
            <div class="flex h-full items-center justify-center bg-[linear-gradient(135deg,hsl(0_0%_8%),hsl(0_0%_12%))] text-sm font-medium text-white">
              Product Hero Media
            </div>
          </AspectRatio>

          <div class="grid gap-3">
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
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Loading and empty states</CardTitle>
          <CardDescription>Keep a few cross-cutting primitives visible here, then move the deeper walkthroughs into the focused child pages.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-5">
          <div class="grid gap-3">
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
          </div>

          <Empty class="border border-dashed py-10">
            <EmptyHeader>
              <EmptyMedia variant="default" class="mb-2">
                <div class="flex -space-x-2 grayscale">
                  <Avatar class="ring-background size-8 ring-2">
                    <AvatarImage src="https://github.com/shadcn.png" alt="@shadcn" />
                    <AvatarFallback>SH</AvatarFallback>
                  </Avatar>
                  <Avatar class="ring-background size-8 ring-2">
                    <AvatarImage src="https://github.com/maxleiter.png" alt="@maxleiter" />
                    <AvatarFallback>ML</AvatarFallback>
                  </Avatar>
                  <Avatar class="ring-background size-8 ring-2">
                    <AvatarImage src="https://github.com/evilrabbit.png" alt="@evilrabbit" />
                    <AvatarFallback>ER</AvatarFallback>
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

          <Empty class="border border-dashed py-10">
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
        </CardContent>
      </Card>
    </div>
  </section>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { ArrowRight, CircleCheckBig, Database, Layers3, LayoutTemplate, LoaderCircle, MousePointerClick, Plus, Sparkles, Workflow } from '@lucide/vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { AspectRatio } from '@/components/ui/aspect-ratio'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from '@/components/ui/item'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'

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
