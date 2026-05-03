<template>
  <SidebarGroup v-if="showGroup">
    <SidebarGroupLabel>Resources</SidebarGroupLabel>
    <SidebarMenu>
      <SidebarMenuItem v-for="item in items" :key="item.title">
        <SidebarMenuButton as-child :tooltip="item.title">
          <a :href="item.url" target="_blank" rel="noreferrer">
            <component :is="item.icon" v-if="item.icon" />
            <span>{{ item.title }}</span>
          </a>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  </SidebarGroup>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { SidebarGroup, SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem, useSidebar } from "./ui/sidebar";

type NavItem = {
  title: string;
  url: string;
  icon: any;
};

defineProps<{
  items: NavItem[];
}>();

const { isMobile, state } = useSidebar();
const showGroup = computed(() => isMobile.value || state.value === "expanded");
</script>
