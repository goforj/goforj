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
    logoDarkSrc?: string;
    logoLightSrc?: string;
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
        class="data-[slot=sidebar-menu-button]:!h-auto data-[slot=sidebar-menu-button]:!px-1 data-[slot=sidebar-menu-button]:!py-1 group-data-[collapsible=icon]:!size-8 group-data-[collapsible=icon]:!p-0.5 group-data-[collapsible=icon]:!justify-center"
      >
        <RouterLink to="/">
          <img
            v-if="activeTeam.logoDarkSrc"
            :src="activeTeam.logoDarkSrc"
            :alt="activeTeam.name"
            class="hidden h-8 w-8 shrink-0 object-contain dark:block"
          />
          <img
            v-if="activeTeam.logoLightSrc"
            :src="activeTeam.logoLightSrc"
            :alt="activeTeam.name"
            class="h-8 w-8 shrink-0 object-contain dark:hidden"
          />
          <component
            v-else-if="activeTeam.logo"
            :is="activeTeam.logo"
            class="!size-10 shrink-0 group-data-[collapsible=icon]:!size-7"
          />
          <span class="text-base font-semibold tracking-tight group-data-[collapsible=icon]:hidden">{{ activeTeam.plan }}</span>
        </RouterLink>
      </SidebarMenuButton>
    </SidebarMenuItem>
  </SidebarMenu>
</template>
