<template>
  <SidebarGroup>
    <SidebarGroupLabel>Resources</SidebarGroupLabel>
    <SidebarMenu class="mt-3">
      <SidebarMenuItem v-for="item in items" :key="item.title">
        <SidebarMenuButton as-child>
          <a :href="item.url" class="nav-link" target="_blank" rel="noreferrer">
            <component :is="item.icon" class="h-4 w-4 text-muted" />
            <span>{{ item.title }}</span>
          </a>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  </SidebarGroup>
</template>

<script setup lang="ts">
import { SidebarGroup, SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "./ui/sidebar";

type NavItem = {
  title: string;
  url: string;
  icon: any;
};

defineProps<{
  items: NavItem[];
}>();
</script>
