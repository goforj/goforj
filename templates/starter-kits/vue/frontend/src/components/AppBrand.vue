<script setup lang="ts">
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar'
import ThemeLogo from './ThemeLogo.vue'

defineProps<{
  name: string
  logoSrc?: string
  logoDarkSrc?: string
  logoCollapsedSrc?: string
  logoCollapsedDarkSrc?: string
}>()
</script>

<template>
  <SidebarMenu>
    <SidebarMenuItem>
      <SidebarMenuButton
        as-child
        class="h-auto! px-1! py-1! group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:p-0.5!"
      >
        <RouterLink to="/" :aria-label="name">
          <ThemeLogo
            v-if="logoSrc"
            :light-src="logoSrc"
            :dark-src="logoDarkSrc"
            class="grid size-8 group-data-[collapsible=icon]:hidden"
          />
          <ThemeLogo
            v-if="logoCollapsedSrc"
            :light-src="logoCollapsedSrc"
            :dark-src="logoCollapsedDarkSrc"
            class="hidden size-6 group-data-[collapsible=icon]:grid"
          />
          <span class="truncate text-base font-semibold tracking-tight group-data-[collapsible=icon]:hidden">
            {{ name }}
          </span>
        </RouterLink>
      </SidebarMenuButton>
    </SidebarMenuItem>
  </SidebarMenu>
</template>
