<script setup lang="ts">
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar'

defineProps<{
  name: string
  logoSrc?: string
  logoCollapsedSrc?: string
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
          <img
            v-if="logoSrc"
            :src="logoSrc"
            alt=""
            class="h-8 w-auto shrink-0 object-contain group-data-[collapsible=icon]:hidden"
          />
          <img
            v-if="logoCollapsedSrc"
            :src="logoCollapsedSrc"
            alt=""
            class="hidden size-6 shrink-0 object-contain group-data-[collapsible=icon]:block"
          />
          <span class="truncate text-base font-semibold tracking-tight group-data-[collapsible=icon]:hidden">
            {{ name }}
          </span>
        </RouterLink>
      </SidebarMenuButton>
    </SidebarMenuItem>
  </SidebarMenu>
</template>
