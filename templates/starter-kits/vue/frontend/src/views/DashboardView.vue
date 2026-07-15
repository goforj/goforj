<template>
  <section class="grid gap-6">
    <Card class="overflow-hidden border-border/60 bg-[linear-gradient(135deg,color-mix(in_oklab,var(--card)_96%,white),var(--background))] text-card-foreground shadow-xl dark:bg-[linear-gradient(135deg,hsl(0_0%_5%),hsl(0_0%_8%))] dark:text-white">
      <CardHeader class="relative gap-4 p-6 md:p-8">
        <div class="flex flex-wrap items-center gap-2">
          <Badge class="border-primary/20 bg-primary/10 text-primary hover:bg-primary/15 dark:border-white/15 dark:bg-white/10 dark:text-white dark:hover:bg-white/15">Vue application shell</Badge>
          <Badge variant="outline" class="border-border/70 text-foreground dark:border-white/20 dark:text-white">Local shadcn-vue source</Badge>
        </div>
        <div class="grid max-w-3xl gap-3">
          <CardTitle class="text-3xl font-semibold tracking-tight md:text-5xl">
            Start from an application shell that already feels production-ready.
          </CardTitle>
          <CardDescription class="max-w-2xl text-base text-muted-foreground dark:text-slate-300">
            Vue, Vite, Tailwind, and shadcn-vue source files are rendered directly into your app so your team can inspect, adapt, or replace every layer.
          </CardDescription>
        </div>
      </CardHeader>
    </Card>

    <div class="grid gap-4 md:grid-cols-3">
      <Card v-for="item in cards" :key="item.title" class="transition-shadow hover:shadow-md">
        <CardHeader>
          <div class="mb-2 flex size-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <component :is="item.icon" class="size-5" />
          </div>
          <CardTitle>{{ item.title }}</CardTitle>
          <CardDescription>{{ item.description }}</CardDescription>
        </CardHeader>
      </Card>
    </div>

    <Card>
      <CardHeader>
        <div class="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <div>
            <CardTitle>Suggested next steps</CardTitle>
            <CardDescription>Replace the starter examples with the first workflow your product actually needs.</CardDescription>
          </div>
          <Badge variant="secondary">App-owned</Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div class="grid gap-3 md:grid-cols-3">
          <div class="rounded-lg border p-4">
            <p class="font-medium">1. Define a domain route</p>
            <p class="mt-1 text-sm text-muted-foreground">Create the API surface your first real screen depends on under <code>/api/v1</code>.</p>
          </div>
          <div class="rounded-lg border p-4">
            <p class="font-medium">2. Connect authentication</p>
            <p class="mt-1 text-sm text-muted-foreground">Wire sign-in, session bootstrap, and logout to the generated cookie-backed auth endpoints.</p>
          </div>
          <div class="rounded-lg border p-4">
            <p class="font-medium">3. Tailor the interface</p>
            <p class="mt-1 text-sm text-muted-foreground">Replace the starter content with product-specific views and extend the local shadcn-vue library as needed.</p>
          </div>
        </div>
      </CardContent>
    </Card>
  </section>
</template>

<script setup lang="ts">
import { Database, Layers3, ShieldCheck } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Card, CardDescription, CardHeader, CardTitle, CardContent } from '@/components/ui/card'

const cards = [
  {
    title: 'Backend-integrated',
    description: 'Generated routes and controllers give the frontend a server-owned foundation from the start.',
    icon: Database,
  },
  {
    title: 'Auth-ready',
    description: 'Sign-in, session bootstrap, password reset, and account surfaces can be connected without rebuilding the shell.',
    icon: ShieldCheck,
  },
  {
    title: 'Composable UI foundation',
    description: 'The copied shadcn-vue components are local source, so your team can evolve them as product code.',
    icon: Layers3,
  },
]
</script>
