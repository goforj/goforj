<template>
  <SidebarGroup>
    <SidebarGroupLabel>Platform</SidebarGroupLabel>
    <SidebarMenu>
      <SidebarMenuItem v-for="item in items" :key="item.title">
        <SidebarMenuButton as-child :is-active="isRouteActive(item.url)" :tooltip="item.title">
          <RouterLink :to="item.url">
            <component :is="item.icon" v-if="item.icon" />
            <span>{{ item.title }}</span>
          </RouterLink>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  </SidebarGroup>
</template>

<script setup lang="ts">
import { RouterLink, useRoute } from "vue-router";
import { SidebarGroup, SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "./ui/sidebar";

type NavItem = {
  title: string;
  url: string;
  icon: any;
};

defineProps<{
  items: NavItem[];
}>();

const route = useRoute();

const isRouteActive = (url: string) => {
  if (url === "/") {
    return route.path === "/";
  }
  return route.path.startsWith(url);
};
</script>
