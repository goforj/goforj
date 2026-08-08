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
        <Collapsible
          v-else-if="item.items?.length"
          :open="isOpen(item)"
          @update:open="(value) => toggle(item, value)"
        >
          <div class="flex items-center">
            <SidebarMenuButton as-child :is-active="isRouteActive(item.url)" :tooltip="item.title" class="flex-1">
              <RouterLink :to="item.url">
                <component :is="item.icon" v-if="item.icon" />
                <span>{{ item.title }}</span>
              </RouterLink>
            </SidebarMenuButton>
            <CollapsibleTrigger as-child>
              <SidebarMenuAction :aria-label="`Toggle ${item.title}`">
                <ChevronRight class="transition-transform duration-200" :class="isOpen(item) && 'rotate-90'" />
              </SidebarMenuAction>
            </CollapsibleTrigger>
          </div>

          <CollapsibleContent>
            <SidebarMenuSub>
              <SidebarMenuSubItem v-for="child in item.items" :key="child.url">
                <SidebarMenuSubButton as-child :is-active="isRouteActive(child.url)">
                  <RouterLink :to="child.url">
                    <span>{{ child.title }}</span>
                  </RouterLink>
                </SidebarMenuSubButton>
              </SidebarMenuSubItem>
            </SidebarMenuSub>
          </CollapsibleContent>
        </Collapsible>

        <SidebarMenuButton v-else as-child :is-active="isRouteActive(item.url)" :tooltip="item.title">
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
import { computed, onMounted, ref } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { ChevronRight } from "@lucide/vue";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "./ui/collapsible";
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
  SidebarMenuAction,
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

// Sections start open when you are inside them, and remember an explicit
// toggle for the rest of the session.
const manuallyToggled = ref<Record<string, boolean>>({});

const isOpen = (item: NavItem) => manuallyToggled.value[item.url] ?? isRouteActive(item.url);

const toggle = (item: NavItem, value: boolean) => {
  manuallyToggled.value = { ...manuallyToggled.value, [item.url]: value };
};

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
