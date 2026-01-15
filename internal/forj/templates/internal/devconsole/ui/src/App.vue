<template>
  <div class="min-h-screen app-bg">
    <div class="app-shell">
      <aside v-if="!isLogin" class="sidebar-surface">
        <div class="px-6 pt-6">
          <img :src="logoFull" alt="GoForj" class="h-7" />
          <p class="mt-2 text-xs uppercase tracking-[0.35em] text-muted">Developer Console</p>
        </div>
        <nav class="mt-8 px-4">
          <RouterLink class="nav-item" active-class="nav-item-active" to="/">
            Dashboard
          </RouterLink>
          <RouterLink class="nav-item" active-class="nav-item-active" to="/logs">
            Logs
          </RouterLink>
        </nav>
        <div class="mt-8 px-6">
          <p class="text-xs uppercase tracking-[0.3em] text-muted">Agents</p>
          <div class="mt-4 space-y-3">
            <div v-if="agents.length === 0" class="text-xs text-muted">
              No agents connected.
            </div>
            <div
              v-for="agent in agents"
              :key="agent.id + agent.source"
              class="rounded-xl border border-border/60 bg-white/5 p-3"
            >
              <p class="text-sm text-white">{{ agent.source }}</p>
              <p class="mt-1 text-xs text-muted">
                {{ agent.env || "unknown" }} · {{ agent.capabilities.join(", ") }}
              </p>
            </div>
          </div>
        </div>
        <div class="mt-auto px-6 pb-6">
          <div class="text-xs text-muted">
            <div class="mb-2">Repository</div>
            <div>Documentation</div>
          </div>
          <button
            class="mt-6 flex w-full items-center justify-between rounded-xl border border-border/70 bg-white/5 px-3 py-2 text-xs text-white/80 hover:text-white"
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
const agents = computed(() => store.state.agents);
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
</script>
