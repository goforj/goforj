import { createRouter, createWebHistory } from "vue-router";
import DashboardView from "./views/DashboardView.vue";
import LogsView from "./views/LogsView.vue";
import LoginView from "./views/LoginView.vue";
import RoutesView from "./views/RoutesView.vue";
import SchedulesView from "./views/SchedulesView.vue";
import CommandsView from "./views/CommandsView.vue";
import EnvView from "./views/EnvView.vue";
import { useDevconsoleStore } from "./stores/devconsole";
import QueuesView from "./views/QueuesView.vue";
import DevWatcherView from "./views/DevWatcherView.vue";
import ProjectConfigView from "./views/ProjectConfigView.vue";
import ComponentsView from "./views/ComponentsView.vue";

const router = createRouter({
  history: createWebHistory("/__devconsole/"),
  routes: [
    { path: "/login", component: LoginView, meta: { public: true } },
    { path: "/", component: DashboardView },
    { path: "/routes", component: RoutesView },
    { path: "/schedules", component: SchedulesView },
    { path: "/queues", component: QueuesView },
    { path: "/devwatch", component: DevWatcherView },
    { path: "/config", component: ProjectConfigView },
    { path: "/commands", component: CommandsView },
    { path: "/env", component: EnvView },
    { path: "/logs", component: LogsView },
    { path: "/components", component: ComponentsView },
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
