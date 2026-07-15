<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { GitFork, LogIn } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { SidebarTrigger } from '@/components/ui/sidebar'

const route = useRoute()
const title = computed(() => (typeof route.meta.title === 'string' ? route.meta.title : 'Dashboard'))
</script>

<template>
  <header class="sticky top-0 z-20 border-b bg-background/80 backdrop-blur-xl">
    <div class="flex h-14 items-center gap-2 px-4 lg:px-6">
      <SidebarTrigger class="-ml-1" />
      <Separator orientation="vertical" class="mx-2 data-[orientation=vertical]:h-4" />
      <div>
        <p class="text-[0.68rem] font-semibold uppercase tracking-[0.24em] text-muted-foreground">Application</p>
        <h1 class="text-base font-semibold leading-none">{{ title }}</h1>
      </div>
      <div class="ml-auto flex items-center gap-2">
        <Button variant="ghost" size="sm" class="hidden gap-2 sm:inline-flex">
          <GitFork class="size-4" />
          Repository
        </Button>
        <Button variant="outline" size="sm" as-child>
          <RouterLink to="/login">
            <LogIn class="size-4" />
            Sign in
          </RouterLink>
        </Button>
      </div>
    </div>
  </header>
</template>
