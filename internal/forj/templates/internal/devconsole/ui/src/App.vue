<template>
  <div class="min-h-screen app-bg">
    <div class="app-shell">
      <Sidebar v-if="!isLogin">
        <SidebarHeader>
          <div class="sidebar-brand">
            <div class="sidebar-brand-icon">
              <img :src="logoFull" alt="GoForj" />
            </div>
            <div>
              <div class="text-sm font-semibold text-white">GoForj</div>
              <div class="text-xs text-muted">Developer Console</div>
            </div>
          </div>
        </SidebarHeader>
        <SidebarContent>
          <NavMain :items="navMain" />
          <NavDocuments :items="navDocuments" />
          <NavSecondary :items="navSecondary" class="mt-auto" @logout="handleLogout" />
        </SidebarContent>
        <SidebarFooter>
          <NavUser @logout="handleLogout" />
        </SidebarFooter>
      </Sidebar>

      <main :class="isLogin ? 'main-surface-login' : 'main-surface'">
        <RouterView v-if="isLogin || (ready && authenticated)" />
      </main>
    </div>
  </div>
  <Toaster />
</template>

<script setup lang="ts">
import { RouterView, useRoute, useRouter } from "vue-router";
import { computed, watch } from "vue";
import {
  LayoutDashboard,
  Route,
  CalendarClock,
  ListChecks,
  Terminal,
  Activity,
  FileText,
  ScrollText,
  BookOpen,
  Github,
} from "lucide-vue-next";
import { useDevconsoleStore } from "./stores/devconsole";
import NavDocuments from "./components/NavDocuments.vue";
import NavMain from "./components/NavMain.vue";
import NavSecondary from "./components/NavSecondary.vue";
import NavUser from "./components/NavUser.vue";
import { Toaster } from "./components/ui/sonner";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "./components/ui/sidebar";
import logoFull from "./assets/goforj-full.png";

const store = useDevconsoleStore();
const route = useRoute();
const router = useRouter();
const isLogin = computed(() => route.path === "/login");
const ready = computed(() => store.state.bootstrapped);
const authenticated = computed(() => store.state.authenticated);

const navMain = [
  { title: "Dashboard", url: "/", icon: LayoutDashboard },
  { title: "Routes", url: "/routes", icon: Route },
  { title: "Schedules", url: "/schedules", icon: CalendarClock },
  { title: "Job Queues (Asynq)", url: "/queues", icon: ListChecks },
  { title: "Dev Watcher", url: "/devwatch", icon: Activity },
  { title: "Commands", url: "/commands", icon: Terminal },
  { title: "Env", url: "/env", icon: FileText },
  { title: "Logs", url: "/logs", icon: ScrollText },
];

const navDocuments = [
  { title: "Repository", url: "https://github.com/goforj/goforj", icon: Github, external: true },
  { title: "Documentation", url: "https://goforj.dev", icon: BookOpen, external: true },
];

const navSecondary = [];

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
