<template>
  <div class="min-h-screen app-bg">
    <div class="app-shell">
      <aside v-if="!isLogin" class="sidebar-surface">
        <div class="px-6 pt-6">
          <img :src="logoFull" alt="GoForj" class="h-12" />
          <p class="mt-2 text-[10px] uppercase tracking-[0.35em] text-muted">Developer Console</p>
        </div>
        <div class="mt-8 px-4">
          <p class="px-2 text-[10px] uppercase tracking-[0.3em] text-muted">Platform</p>
        </div>
        <nav class="mt-3 px-4">
          <RouterLink class="nav-item" active-class="nav-item-active" to="/">
            Dashboard
          </RouterLink>
          <RouterLink class="nav-item" active-class="nav-item-active" to="/routes">
            Routes
          </RouterLink>
          <RouterLink class="nav-item" active-class="nav-item-active" to="/schedules">
            Schedules
          </RouterLink>
          <RouterLink class="nav-item" active-class="nav-item-active" to="/queues">
            Job Queues (Asynq)
          </RouterLink>
          <RouterLink class="nav-item" active-class="nav-item-active" to="/commands">
            Commands
          </RouterLink>
          <RouterLink class="nav-item" active-class="nav-item-active" to="/env">
            Env
          </RouterLink>
          <RouterLink class="nav-item" active-class="nav-item-active" to="/logs">
            Logs
          </RouterLink>
        </nav>
        <div class="mt-auto px-6 pb-6">
          <div class="text-xs text-muted">
            <div class="mb-2">Repository</div>
            <div>Documentation</div>
          </div>
          <button
            class="mt-6 flex w-full items-center justify-between rounded-xl border border-border/70 bg-white/5 px-3 py-2 text-xs text-white/80 transition hover:border-white/20 hover:text-white"
            @click="handleLogout"
          >
            Logout
          </button>
        </div>
      </aside>

      <main :class="isLogin ? 'main-surface-login' : 'main-surface'">
        <RouterView v-if="isLogin || (ready && authenticated)" />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { RouterLink, RouterView, useRoute, useRouter } from "vue-router";
import { computed, watch } from "vue";
import { useDevconsoleStore } from "./stores/devconsole";
import logoFull from "./assets/goforj-full.png";

const store = useDevconsoleStore();
const selectedAgent = computed(() => store.state.selectedAgent);
const route = useRoute();
const router = useRouter();
const isLogin = computed(() => route.path === "/login");
const ready = computed(() => store.state.bootstrapped);
const authenticated = computed(() => store.state.authenticated);

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

const handleSelectAgent = (source: string) => {
  store.selectAgent(source);
};

</script>
