import { createRouter, createWebHistory } from "vue-router";
import DashboardView from "./views/DashboardView.vue";
import LogsView from "./views/LogsView.vue";
import LoginView from "./views/LoginView.vue";
import RoutesView from "./views/RoutesView.vue";
import SchedulesView from "./views/SchedulesView.vue";
import CommandsView from "./views/CommandsView.vue";
import EnvView from "./views/EnvView.vue";
import { useLighthouseStore } from "./stores/lighthouse";
import QueuesView from "./views/QueuesView.vue";
import BenchmarksView from "./views/BenchmarksView.vue";
import DevWatcherView from "./views/DevWatcherView.vue";
import ProjectConfigView from "./views/ProjectConfigView.vue";
import ComponentsView from "./views/ComponentsView.vue";
import { findAppNavItem } from "./lib/navigation";
import { lighthouseBasePath } from "./lib/base-path";

const navTitle = (path: string, fallback: string) => findAppNavItem(path)?.title || fallback;

const router = createRouter({
  history: createWebHistory(`${lighthouseBasePath}/`),
  routes: [
    { path: "/login", component: LoginView, meta: { public: true, title: "Sign In" } },
    { path: "/", component: DashboardView, meta: { title: navTitle("/", "Dashboard") } },
    { path: "/routes", component: RoutesView, meta: { title: navTitle("/routes", "Routes") } },
    { path: "/schedules", component: SchedulesView, meta: { title: navTitle("/schedules", "Schedules") } },
    { path: "/queues", component: QueuesView, meta: { title: navTitle("/queues", "Job Queues") } },
    { path: "/benchmarks", component: BenchmarksView, meta: { title: navTitle("/benchmarks", "Benchmarks") } },
    { path: "/devwatch", component: DevWatcherView, meta: { title: navTitle("/devwatch", "Dev Watcher") } },
    { path: "/config", component: ProjectConfigView, meta: { title: navTitle("/config", "Project Config") } },
    { path: "/commands", component: CommandsView, meta: { title: navTitle("/commands", "Commands") } },
    { path: "/env", component: EnvView, meta: { title: navTitle("/env", "Env") } },
    { path: "/logs", component: LogsView, meta: { title: navTitle("/logs", "Logs") } },
    { path: "/components", component: ComponentsView, meta: { title: "Components" } },
  ],
});

router.beforeEach(async (to) => {
  const store = useLighthouseStore();
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
