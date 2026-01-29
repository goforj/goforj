<template>
  <SidebarGroup>
    <SidebarGroupLabel>Shortcuts</SidebarGroupLabel>
    <SidebarMenu>
      <SidebarMenuItem v-for="item in items" :key="item.title">
        <SidebarMenuButton as-child :tooltip="item.title">
          <button v-if="item.action === 'logout'" type="button" class="flex w-full items-center gap-2" @click="$emit('logout')">
            <component :is="item.icon" v-if="item.icon" />
            <span>{{ item.title }}</span>
            <span
              v-if="item.shortcut"
              class="ml-auto inline-flex items-center rounded-full border border-sidebar-border bg-sidebar/50 px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.16em] text-sidebar-foreground/70"
            >
              {{ item.shortcut }}
            </span>
          </button>
          <button v-else-if="item.action === 'command'" type="button" class="flex w-full items-center gap-2" @click="$emit('command')">
            <component :is="item.icon" v-if="item.icon" />
            <span>{{ item.title }}</span>
            <span
              v-if="item.shortcut"
              class="ml-auto inline-flex items-center rounded-full border border-sidebar-border bg-sidebar/50 px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.16em] text-sidebar-foreground/70"
            >
              {{ item.shortcut }}
            </span>
          </button>
          <a v-else class="flex w-full items-center gap-2" :href="item.url" target="_blank" rel="noreferrer">
            <component :is="item.icon" v-if="item.icon" />
            <span>{{ item.title }}</span>
            <span
              v-if="item.shortcut"
              class="ml-auto inline-flex items-center rounded-full border border-sidebar-border bg-sidebar/50 px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.16em] text-sidebar-foreground/70"
            >
              {{ item.shortcut }}
            </span>
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
  shortcut?: string;
  action?: "logout" | "command";
};

defineProps<{
  items: NavItem[];
}>();

defineEmits<{
  (event: "logout"): void;
  (event: "command"): void;
}>();
</script>
