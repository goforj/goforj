import { createRouter, createWebHistory } from "vue-router";
import DashboardView from "./views/DashboardView.vue";
import LogsView from "./views/LogsView.vue";
import LoginView from "./views/LoginView.vue";
import RoutesView from "./views/RoutesView.vue";
import SchedulesView from "./views/SchedulesView.vue";
import CommandsView from "./views/CommandsView.vue";
import EnvView from "./views/EnvView.vue";
import TracesView from "./views/TracesView.vue";
import { useLighthouseStore } from "./stores/lighthouse";
import QueuesView from "./views/QueuesView.vue";
import StorageView from "./views/StorageView.vue";
import CacheView from "./views/CacheView.vue";
import BenchmarksView from "./views/BenchmarksView.vue";
import DevWatcherView from "./views/DevWatcherView.vue";
import ProjectConfigView from "./views/ProjectConfigView.vue";
import ComponentsView from "./views/ComponentsView.vue";
import { findAppNavItem } from "./lib/navigation";
import { lighthouseBasePath } from "./lib/base-path";

const navTitle = (path: string, fallback: string) => findAppNavItem(path)?.title || fallback;

const inspectMeta = (path: string, fallback: string, source: string) => ({
  title: navTitle(path, fallback),
  inspectTitle: navTitle(path, fallback),
  inspectSource: source,
});

const router = createRouter({
  history: createWebHistory(`${lighthouseBasePath}/`),
  routes: [
    { path: "/login", component: LoginView, meta: { public: true, title: "Sign In" } },
    { path: "/", component: DashboardView, meta: { title: navTitle("/", "Dashboard") } },
    { path: "/routes", component: RoutesView, meta: { title: navTitle("/routes", "Routes") } },
    { path: "/schedules", component: SchedulesView, meta: { title: navTitle("/schedules", "Schedules") } },
    { path: "/queues", component: QueuesView, meta: { title: navTitle("/queues", "Job Queues") } },
    { path: "/cache", component: CacheView, meta: { title: navTitle("/cache", "Cache") } },
    { path: "/storage", component: StorageView, meta: { title: navTitle("/storage", "Storage") } },
    { path: "/benchmarks", component: BenchmarksView, meta: { title: navTitle("/benchmarks", "Benchmarks") } },
    { path: "/devwatch", component: DevWatcherView, meta: { title: navTitle("/devwatch", "Dev Watcher") } },
    { path: "/config", component: ProjectConfigView, meta: { title: navTitle("/config", "Project Config") } },
    { path: "/commands", component: CommandsView, meta: { title: navTitle("/commands", "Commands") } },
    { path: "/env", component: EnvView, meta: { title: navTitle("/env", "Env") } },
    { path: "/traces", redirect: "/inspect/requests" },
    { path: "/inspect/requests", component: TracesView, meta: inspectMeta("/inspect/requests", "Requests", "http") },
    { path: "/inspect/commands", component: TracesView, meta: inspectMeta("/inspect/commands", "Commands", "cli") },
    { path: "/inspect/jobs", component: TracesView, meta: inspectMeta("/inspect/jobs", "Jobs", "jobs") },
    { path: "/inspect/schedule", component: TracesView, meta: inspectMeta("/inspect/schedule", "Schedule", "scheduler") },
    { path: "/logs", component: LogsView, meta: { title: navTitle("/logs", "Logs") } },
    { path: "/components", component: ComponentsView, meta: { title: "Components" } },
  ],
});

router.beforeEach(async (to, from) => {
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
  if (store.state.reconnecting && to.fullPath !== from.fullPath) {
    return false;
  }
  return true;
});

export default router;
