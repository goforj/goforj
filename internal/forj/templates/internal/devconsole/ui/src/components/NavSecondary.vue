<template>
  <SidebarMenu>
    <SidebarMenuItem v-for="item in items" :key="item.title">
      <SidebarMenuButton as-child :tooltip="item.title">
        <button v-if="item.action === 'logout'" @click="$emit('logout')">
          <component :is="item.icon" v-if="item.icon" />
          <span>{{ item.title }}</span>
        </button>
        <a v-else :href="item.url" target="_blank" rel="noreferrer">
          <component :is="item.icon" v-if="item.icon" />
          <span>{{ item.title }}</span>
        </a>
      </SidebarMenuButton>
    </SidebarMenuItem>
  </SidebarMenu>
</template>

<script setup lang="ts">
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "./ui/sidebar";

type NavItem = {
  title: string;
  url: string;
  icon: any;
  action?: "logout";
};

defineProps<{
  items: NavItem[];
}>();

defineEmits<{
  (event: "logout"): void;
}>();
</script>
