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
    { path: "/login", component: LoginView, meta: { public: true, title: "Sign In" } },
    { path: "/", component: DashboardView, meta: { title: "Dashboard" } },
    { path: "/routes", component: RoutesView, meta: { title: "Routes" } },
    { path: "/schedules", component: SchedulesView, meta: { title: "Schedules" } },
    { path: "/queues", component: QueuesView, meta: { title: "Job Queues" } },
    { path: "/devwatch", component: DevWatcherView, meta: { title: "Dev Watcher" } },
    { path: "/config", component: ProjectConfigView, meta: { title: "Project Config" } },
    { path: "/commands", component: CommandsView, meta: { title: "Commands" } },
    { path: "/env", component: EnvView, meta: { title: "Env" } },
    { path: "/logs", component: LogsView, meta: { title: "Logs" } },
    { path: "/components", component: ComponentsView, meta: { title: "Components" } },
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
