<template>
  <div class="lighthouse-app min-h-screen w-full bg-background text-foreground">
    <SidebarProvider
      :style="{
        '--sidebar-width': 'calc(var(--spacing) * 72)',
        '--header-height': 'calc(var(--spacing) * 12)',
      }"
    >
      <AppSidebar
        v-if="!isLogin"
        @logout="handleLogout"
        @command="commandOpen = true"
      />

      <SidebarInset :class="isLogin ? 'main-surface-login' : 'main-surface'">
        <header
          v-if="!isLogin"
          data-slot="app-header"
          class="shrink-0 border-b border-border transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-(--header-height)"
        >
          <div class="flex h-(--header-height) w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
            <SidebarTrigger class="-ml-1" />
            <Separator orientation="vertical" class="mx-2 data-[orientation=vertical]:h-4" />
            <Breadcrumb>
              <BreadcrumbList>
                <BreadcrumbItem class="hidden md:block">
                  <BreadcrumbLink href="#">Platform</BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator class="hidden md:block" />
                <BreadcrumbItem>
                  <BreadcrumbPage class="inline-flex items-center gap-1.5">
                    <component :is="pageIcon" v-if="pageIcon" class="size-4 text-chart-2" />
                    {{ pageTitle }}
                  </BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>
            <div class="ml-auto flex items-center gap-2 pr-2">
              <div class="hidden items-center gap-2 overflow-x-auto md:flex">
                <AgentPills />
                <LivePill />
              </div>
              <button
                type="button"
                class="goforj-mode-switch"
                :aria-pressed="isDark"
                role="switch"
                :aria-checked="isDark"
                :aria-label="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
                @click="toggleTheme"
              >
                <span class="goforj-mode-switch__thumb">
                  <Moon v-if="isDark" aria-hidden="true" />
                  <Sun v-else aria-hidden="true" />
                </span>
              </button>
            </div>
          </div>
        </header>
        <div :class="isLogin ? 'flex flex-1' : 'main-content-area flex flex-1 flex-col gap-4 p-4 pt-4'">
          <RouterView v-if="isLogin || (ready && authenticated)" />
        </div>
      </SidebarInset>
    </SidebarProvider>
    <div
      v-if="showReconnectOverlay"
      class="reconnect-overlay"
      aria-live="polite"
      aria-busy="true"
    >
      <div class="reconnect-overlay__panel">
        <div class="reconnect-overlay__pulse" />
        <p class="reconnect-overlay__eyebrow">Live connection lost</p>
        <h2 class="reconnect-overlay__title">Reconnecting to Lighthouse</h2>
        <p class="reconnect-overlay__copy">
          Waiting for the source websocket and connected agents to come back online.
        </p>
      </div>
    </div>
    <CommandMenu
      v-if="!isLogin && authenticated"
      :open="commandOpen"
      @update:open="(value) => (commandOpen = value)"
    />
  </div>
  <Toaster />
</template>

<script setup lang="ts">
import { RouterView, useRoute, useRouter } from "vue-router";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Moon, Sun } from "lucide-vue-next";
import { useLighthouseStore } from "./stores/lighthouse";
import { findAppNavItem } from "./lib/navigation";
import AppSidebar from "./components/AppSidebar.vue";
import AgentPills from "./components/AgentPills.vue";
import LivePill from "./components/LivePill.vue";
import CommandMenu from "./components/CommandMenu.vue";
import { Toaster } from "./components/ui/sonner";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "./components/ui/breadcrumb";
import { Separator } from "./components/ui/separator";
import {
  SidebarProvider,
  SidebarInset,
  SidebarTrigger,
} from "./components/ui/sidebar";

const store = useLighthouseStore();
const route = useRoute();
const router = useRouter();
const isLogin = computed(() => route.path === "/login");
const ready = computed(() => store.state.bootstrapped);
const authenticated = computed(() => store.state.authenticated);
const pageTitle = computed(() => (route.meta?.title as string) || "Dashboard");
const pageIcon = computed(() => findAppNavItem(route.path)?.icon);
const showReconnectOverlay = computed(
  () => !isLogin.value && ready.value && authenticated.value && store.state.reconnecting
);

const isDark = ref(true);
const commandOpen = ref(false);
let keydownHandler: ((event: KeyboardEvent) => void) | null = null;

const applyTheme = (value: boolean) => {
  document.documentElement.classList.toggle("dark", value);
  localStorage.setItem("theme", value ? "dark" : "light");
};

const toggleTheme = () => {
  isDark.value = !isDark.value;
  applyTheme(isDark.value);
};

onMounted(() => {
  const stored = localStorage.getItem("theme");
  const next = stored ? stored === "dark" : true;
  isDark.value = next;
  applyTheme(next);
});

onMounted(() => {
  keydownHandler = (event: KeyboardEvent) => {
    const target = event.target as HTMLElement | null;
    const tag = target?.tagName?.toLowerCase();
    if (tag === "input" || tag === "textarea" || tag === "select" || target?.isContentEditable) {
      return;
    }
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
      event.preventDefault();
      commandOpen.value = !commandOpen.value;
    }
  };
  window.addEventListener("keydown", keydownHandler);
});

onBeforeUnmount(() => {
  if (keydownHandler) {
    window.removeEventListener("keydown", keydownHandler);
  }
});

watch(
  () => store.state.authenticated,
  (authenticated) => {
    if (authenticated) {
      store.connectSocket();
    }
    if (!authenticated && route.path !== "/login") {
      commandOpen.value = false;
      router.replace("/login");
    }
  }
);

watch(
  () => route.path,
  () => {
    commandOpen.value = false;
  }
);

watch(
  pageTitle,
  (title) => {
    document.title = `GoForj Lighthouse | ${title || "Dashboard"}`;
  },
  { immediate: true }
);

const handleLogout = async () => {
  await store.logout();
  router.replace("/login");
};

</script>
