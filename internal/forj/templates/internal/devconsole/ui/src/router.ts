import { createRouter, createWebHistory } from "vue-router";
import DashboardView from "./views/DashboardView.vue";
import LogsView from "./views/LogsView.vue";
import LoginView from "./views/LoginView.vue";
import { useDevconsoleStore } from "./stores/devconsole";

const router = createRouter({
  history: createWebHistory("/__devconsole/"),
  routes: [
    { path: "/login", component: LoginView, meta: { public: true } },
    { path: "/", component: DashboardView },
    { path: "/logs", component: LogsView },
  ],
});

router.beforeEach(async (to) => {
  const store = useDevconsoleStore();
  await store.bootstrap();
  if (to.meta.public) {
    if (store.state.authenticated) {
      return "/";
    }
    return true;
  }
  if (!store.state.authenticated) {
    return "/login";
  }
  return true;
});

export default router;
