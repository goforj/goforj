<template>
  <SidebarGroup>
    <SidebarGroupLabel>Platform</SidebarGroupLabel>
    <SidebarMenu>
      <SidebarMenuItem v-for="item in items" :key="item.title">
        <DropdownMenu v-if="item.items?.length && !showExpandedContent" :modal="false">
          <DropdownMenuTrigger as-child>
            <SidebarMenuButton :is-active="isRouteActive(item.url)" :tooltip="item.title">
              <component :is="item.icon" v-if="item.icon" />
              <span>{{ item.title }}</span>
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="right" align="start" class="z-[100] min-w-52">
            <DropdownMenuLabel class="text-xs text-muted-foreground">{{ item.title }}</DropdownMenuLabel>
            <DropdownMenuItem v-for="child in item.items" :key="child.url" as-child>
              <RouterLink :to="child.url">{{ child.title }}</RouterLink>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <SidebarMenuButton v-else as-child :is-active="isRouteActive(item.url)" :tooltip="item.title">
          <RouterLink :to="item.url">
            <component :is="item.icon" v-if="item.icon" />
            <span>{{ item.title }}</span>
          </RouterLink>
        </SidebarMenuButton>

        <SidebarMenuSub v-if="item.items?.length && isRouteActive(item.url)">
          <SidebarMenuSubItem v-for="child in item.items" :key="child.url">
            <SidebarMenuSubButton as-child :is-active="isRouteActive(child.url)">
              <RouterLink :to="child.url">
                <span>{{ child.title }}</span>
              </RouterLink>
            </SidebarMenuSubButton>
          </SidebarMenuSubItem>
        </SidebarMenuSub>
      </SidebarMenuItem>
    </SidebarMenu>
  </SidebarGroup>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "./ui/dropdown-menu";
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  useSidebar,
} from "./ui/sidebar";

type NavItem = {
  title: string;
  url: string;
  icon: any;
  items?: Array<{
    title: string;
    url: string;
  }>;
};

defineProps<{
  items: NavItem[];
}>();

const route = useRoute();
const router = useRouter();
const routerReady = ref(false);
const { isMobile, state } = useSidebar();
const showExpandedContent = computed(() => isMobile.value || state.value === "expanded");

onMounted(async () => {
  await router.isReady();
  routerReady.value = true;
});

const isRouteActive = (url: string) => {
  if (!routerReady.value) {
    return false;
  }
  if (url === "/") {
    return route.path === "/";
  }
  return route.path.startsWith(url);
};
</script>
