<template>
  <div class="min-h-screen w-full bg-background text-foreground">
    <SidebarProvider>
      <AppSidebar v-if="!isLogin" @logout="handleLogout" />

      <SidebarInset :class="isLogin ? 'main-surface-login' : 'main-surface'">
        <header
          v-if="!isLogin"
          class="flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear"
        >
          <div class="flex w-full items-center gap-2 px-4">
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
            <div class="ml-auto flex items-center gap-2">
              <AgentPills />
              <LivePill />
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
        <div :class="isLogin ? '' : 'flex flex-1 flex-col gap-4 p-4 pt-0'">
          <RouterView v-if="isLogin || (ready && authenticated)" />
        </div>
      </SidebarInset>
    </SidebarProvider>
  </div>
  <Toaster />
</template>

<script setup lang="ts">
import { RouterView, useRoute, useRouter } from "vue-router";
import { computed, onMounted, ref, watch } from "vue";
import { Moon, Sun } from "lucide-vue-next";
import { useDevconsoleStore } from "./stores/devconsole";
import AppSidebar from "./components/AppSidebar.vue";
import AgentPills from "./components/AgentPills.vue";
import LivePill from "./components/LivePill.vue";
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

watch(
  () => store.state.authenticated,
  (authenticated) => {
    if (authenticated) {
      store.connectSocket();
    }
    if (!authenticated && route.path !== "/login") {
      router.replace("/login");
    }
  }
);

const handleLogout = async () => {
  await store.logout();
  router.replace("/login");
};

</script>
