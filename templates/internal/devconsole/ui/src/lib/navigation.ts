import {
  Activity,
  CalendarClock,
  FileText,
  LayoutDashboard,
  ListChecks,
  Route,
  ScrollText,
  Settings,
  Terminal,
} from "lucide-vue-next";

export type AppNavItem = {
  title: string;
  url: string;
  icon: any;
};

export const appNavMain: AppNavItem[] = [
  { title: "Dashboard", url: "/", icon: LayoutDashboard },
  { title: "Routes", url: "/routes", icon: Route },
  { title: "Schedules", url: "/schedules", icon: CalendarClock },
  { title: "Job Queues", url: "/queues", icon: ListChecks },
  { title: "Dev Watcher", url: "/devwatch", icon: Activity },
  { title: "Project Config", url: "/config", icon: Settings },
  { title: "Commands", url: "/commands", icon: Terminal },
  { title: "Env", url: "/env", icon: FileText },
  { title: "Logs", url: "/logs", icon: ScrollText },
];

export function findAppNavItem(path: string): AppNavItem | undefined {
  return appNavMain.find((item) => item.url === path);
}

