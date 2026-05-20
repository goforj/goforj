<template>
  <template v-for="section in sections" :key="section.title">
    <SidebarGroup>
      <SidebarGroupLabel>{{ section.title }}</SidebarGroupLabel>
      <SidebarMenu>
        <SidebarMenuItem v-for="item in section.items" :key="item.title">
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
</template>

<script setup lang="ts">
import { RouterLink, useRoute } from "vue-router";
import { SidebarGroup, SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "./ui/sidebar";

type NavItem = {
  title: string;
  url: string;
  icon: any;
};

type NavSection = {
  title: string;
  items: NavItem[];
};

defineProps<{
  sections: NavSection[];
}>();

const route = useRoute();

const isRouteActive = (url: string) => {
  if (url === "/") {
    return route.path === "/";
  }
  return route.path.startsWith(url);
};
</script>
