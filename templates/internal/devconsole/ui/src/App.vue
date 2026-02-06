<template>
  <div class="min-h-screen w-full bg-background text-foreground">
    <SidebarProvider>
      <AppSidebar
        v-if="!isLogin"
        @logout="handleLogout"
        @command="commandOpen = true"
      />

      <SidebarInset :class="isLogin ? 'main-surface-login' : 'main-surface'">
        <header
          v-if="!isLogin"
          data-slot="app-header"
          class="flex min-h-16 items-center gap-2 transition-[width,height] ease-linear"
        >
          <div class="flex w-full flex-wrap items-center gap-2 px-4 py-2 md:py-0">
            <SidebarTrigger class="-ml-1" />
            <Separator orientation="vertical" class="mr-2 data-[orientation=vertical]:h-4" />
            <Breadcrumb>
              <BreadcrumbList>
                <BreadcrumbItem class="hidden md:block">
                  <BreadcrumbLink href="#">Platform</BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator class="hidden md:block" />
                <BreadcrumbItem>
                  <BreadcrumbPage>{{ pageTitle }}</BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>
            <div class="ml-auto flex min-w-0 items-center gap-3">
              <div class="flex min-w-0 items-center gap-2 overflow-x-auto hide-scrollbar sm:overflow-visible">
                <AgentPills />
                <LivePill />
              </div>
              <ThemeSelector v-if="isDark" v-model="themeId" />
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-md border border-input bg-background px-3 py-1.5 text-sm font-medium text-foreground shadow-sm transition hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                :aria-pressed="isDark"
                aria-label="Toggle theme"
                @click="toggleTheme"
              >
                <span class="hidden sm:inline">{{ isDark ? "Dark" : "Light" }}</span>
                <Sun v-if="!isDark" class="h-4 w-4" aria-hidden="true" />
                <Moon v-else class="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
          </div>
        </header>
        <div :class="isLogin ? '' : 'flex flex-1 flex-col gap-4 p-4 pt-4'">
          <RouterView v-if="isLogin || (ready && authenticated)" />
        </div>
      </SidebarInset>
    </SidebarProvider>
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
import { useDevconsoleStore } from "./stores/devconsole";
import AppSidebar from "./components/AppSidebar.vue";
import AgentPills from "./components/AgentPills.vue";
import LivePill from "./components/LivePill.vue";
import CommandMenu from "./components/CommandMenu.vue";
import ThemeSelector from "./components/ThemeSelector.vue";
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

const store = useDevconsoleStore();
const route = useRoute();
const router = useRouter();
const isLogin = computed(() => route.path === "/login");
const ready = computed(() => store.state.bootstrapped);
const authenticated = computed(() => store.state.authenticated);
const pageTitle = computed(() => (route.meta?.title as string) || "Dashboard");

const isDark = ref(true);
const themeId = ref("discord");
const commandOpen = ref(false);
let keydownHandler: ((event: KeyboardEvent) => void) | null = null;

const applyTheme = (value: boolean) => {
  document.documentElement.classList.toggle("dark", value);
  document.documentElement.classList.remove("glass-v2");
  if (themeId.value === "default") {
    delete document.documentElement.dataset.theme;
  } else {
    document.documentElement.dataset.theme = themeId.value;
  }
  localStorage.setItem("theme", value ? "dark" : "light");
  localStorage.setItem("theme-id", themeId.value);
};

const toggleTheme = () => {
  isDark.value = !isDark.value;
  applyTheme(isDark.value);
};

onMounted(() => {
  const stored = localStorage.getItem("theme");
  const next = stored ? stored === "dark" : true;
  isDark.value = next;
  const storedTheme = localStorage.getItem("theme-id");
  themeId.value = storedTheme || "default";
  applyTheme(next);
});

watch(themeId, () => {
  applyTheme(isDark.value);
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

const handleLogout = async () => {
  await store.logout();
  router.replace("/login");
};

</script>
