<template>
  <section class="grid gap-6 px-4 py-6">
    <div class="grid gap-1">
      <h1 class="text-3xl font-semibold tracking-tight">Settings</h1>
      <p class="text-muted-foreground">Manage your profile and account settings</p>
    </div>

    <div class="flex flex-col gap-8 lg:flex-row lg:gap-12">
      <aside class="w-full max-w-xl lg:w-64">
        <nav class="grid gap-1" aria-label="Settings">
          <Button
            v-for="item in settingsNavItems"
            :key="item.url"
            variant="ghost"
            as-child
            class="justify-start"
            :class="isActive(item.url) ? 'bg-muted text-foreground' : 'text-muted-foreground'"
          >
            <RouterLink :to="item.url">
              {{ item.title }}
            </RouterLink>
          </Button>
        </nav>
      </aside>

      <div class="min-w-0 flex-1">
        <section class="max-w-xl space-y-10">
          <RouterView />
        </section>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { Button } from '@/components/ui/button'

const route = useRoute()

const settingsNavItems = [
  { title: 'Profile', url: '/settings/profile' },
  { title: 'Password', url: '/settings/password' },
  { title: 'Appearance', url: '/settings/appearance' },
]

function isActive(url: string) {
  return route.path === url
}
</script>
