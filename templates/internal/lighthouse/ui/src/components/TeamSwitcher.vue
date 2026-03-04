<script setup lang="ts">
import type { Component } from "vue";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";

const props = defineProps<{
  teams: {
    name: string;
    logo?: Component;
    logoSrc?: string;
    logoCollapsedSrc?: string;
    plan: string;
  }[];
}>();

const activeTeam = props.teams[0];
</script>

<template>
  <SidebarMenu>
    <SidebarMenuItem>
      <SidebarMenuButton
        as-child
        class="data-[slot=sidebar-menu-button]:!h-auto data-[slot=sidebar-menu-button]:!px-2 data-[slot=sidebar-menu-button]:!py-2 group-data-[collapsible=icon]:!size-8 group-data-[collapsible=icon]:!p-0 group-data-[collapsible=icon]:!justify-center"
      >
        <RouterLink to="/">
          <img
            v-if="activeTeam.logoSrc"
            :src="activeTeam.logoSrc"
            :alt="activeTeam.name"
            class="h-8 w-auto shrink-0 object-contain group-data-[collapsible=icon]:hidden"
          />
          <img
            v-if="activeTeam.logoCollapsedSrc"
            :src="activeTeam.logoCollapsedSrc"
            :alt="activeTeam.name"
            class="hidden h-5 w-5 shrink-0 object-contain group-data-[collapsible=icon]:block"
          />
          <component v-else-if="activeTeam.logo" :is="activeTeam.logo" class="size-4" />
          <span class="text-base font-semibold tracking-tight group-data-[collapsible=icon]:hidden">{{ activeTeam.plan }}</span>
        </RouterLink>
      </SidebarMenuButton>
    </SidebarMenuItem>
  </SidebarMenu>
</template>
