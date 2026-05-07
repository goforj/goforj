import {
  Activity,
  Bot,
  CalendarClock,
  ClipboardList,
  FileText,
  FolderTree,
  Gauge,
  KeyRound,
  LayoutDashboard,
  ListChecks,
  Route,
  ScrollText,
  Settings,
  Terminal,
  Workflow,
} from "lucide-vue-next";

export type AppNavItem = {
  title: string;
  url: string;
  icon: any;
};

export type AppNavSection = {
  title: string;
  items: AppNavItem[];
};

export const appNavSections: AppNavSection[] = [
  {
    title: "Platform",
    items: [
      { title: "Dashboard", url: "/", icon: LayoutDashboard },
      { title: "Routes", url: "/routes", icon: Route },
      { title: "Schedules", url: "/schedules", icon: CalendarClock },
      { title: "Job Queues", url: "/queues", icon: ListChecks },
      { title: "Cache", url: "/cache", icon: KeyRound },
      { title: "Storage", url: "/storage", icon: FolderTree },
      { title: "Benchmarks", url: "/benchmarks", icon: Gauge },
      { title: "Dev Watcher", url: "/devwatch", icon: Activity },
      { title: "Project Config", url: "/config", icon: Settings },
      { title: "CLI Commands", url: "/commands", icon: Terminal },
      { title: "Env", url: "/env", icon: FileText },
      { title: "Logs", url: "/logs", icon: ScrollText },
    ],
  },
  {
    title: "Inspect",
    items: [
      { title: "Requests", url: "/inspect/requests", icon: Route },
      { title: "Commands", url: "/inspect/commands", icon: Terminal },
      { title: "Jobs", url: "/inspect/jobs", icon: Bot },
      { title: "Schedule", url: "/inspect/schedule", icon: ClipboardList },
    ],
  },
];

export const appNavMain: AppNavItem[] = appNavSections.flatMap((section) => section.items);

export function findAppNavItem(path: string): AppNavItem | undefined {
  return appNavMain.find((item) => item.url === path);
}
