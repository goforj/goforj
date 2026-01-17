<template>
  <SidebarGroup>
    <SidebarGroupLabel>Platform</SidebarGroupLabel>
    <SidebarMenu class="mt-3">
      <SidebarMenuItem v-for="item in items" :key="item.title">
        <SidebarMenuButton as-child>
          <RouterLink
            :to="item.url"
            class="nav-link"
            :class="isRouteActive(item.url) ? 'nav-link-active' : ''"
          >
            <component :is="item.icon" class="h-4 w-4 text-muted" />
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
