<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { appName } from '@/lib/app'
import logoMark from '@/assets/goforj-logo.png'

defineProps<{
  title: string
  description: string
}>()
</script>

<template>
  <div class="flex min-h-full flex-1 items-center justify-center bg-background p-6 md:p-10">
    <div class="flex w-full max-w-sm flex-col gap-8">
      <div class="flex flex-col items-center gap-4">
        <RouterLink to="/" class="flex flex-col items-center gap-2 font-medium">
          <img :src="logoMark" alt="" class="size-12 object-contain" />
          <span class="sr-only">{{ appName }}</span>
        </RouterLink>

        <div class="grid gap-2 text-center">
          <h1 class="text-xl font-medium">{{ title }}</h1>
          <p class="text-sm text-muted-foreground">{{ description }}</p>
        </div>
      </div>

      <slot />

      <div class="text-center text-sm text-muted-foreground">
        <slot name="footer" />
      </div>
    </div>
  </div>
</template>
