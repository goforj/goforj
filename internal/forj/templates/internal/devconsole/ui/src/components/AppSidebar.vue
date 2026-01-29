<script setup lang="ts">
import type { SidebarProps } from "./ui/sidebar";
import { computed } from "vue";
import {
  Activity,
  BookOpen,
  CalendarClock,
  FileText,
  Github,
  LayoutDashboard,
  ListChecks,
  Route,
  ScrollText,
  Settings,
  Terminal,
  Command,
} from "lucide-vue-next";
import TeamSwitcher from "./TeamSwitcher.vue";
import NavDocuments from "./NavDocuments.vue";
import NavMain from "./NavMain.vue";
import NavSecondary from "./NavSecondary.vue";
import NavUser from "./NavUser.vue";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarRail,
} from "./ui/sidebar";

const props = withDefaults(defineProps<SidebarProps>(), {
  collapsible: "icon",
});

const navMain = [
  { title: "Dashboard", url: "/", icon: LayoutDashboard },
  { title: "Routes", url: "/routes", icon: Route },
  { title: "Schedules", url: "/schedules", icon: CalendarClock },
  { title: "Job Queues (Asynq)", url: "/queues", icon: ListChecks },
  { title: "Dev Watcher", url: "/devwatch", icon: Activity },
  { title: "Project Config", url: "/config", icon: Settings },
  { title: "Commands", url: "/commands", icon: Terminal },
  { title: "Env", url: "/env", icon: FileText },
  { title: "Logs", url: "/logs", icon: ScrollText },
];

const navDocuments = [
  { title: "Repository", url: "https://github.com/goforj/goforj", icon: Github, external: true },
  { title: "Documentation", url: "https://goforj.dev", icon: BookOpen, external: true },
];

const isMac = computed(() => {
  if (typeof navigator === "undefined") return false;
  return /Mac|iPhone|iPad|iPod/.test(navigator.platform);
});

const commandShortcut = computed(() => (isMac.value ? "⌘ + K" : "Ctrl + K"));

const navSecondary = computed(
  () =>
    [
      { title: "Command Palette", url: "#", icon: Command, action: "command", shortcut: commandShortcut.value },
    ] as Array<{ title: string; url: string; icon: any; action?: "logout" | "command"; shortcut?: string }>
);

const teams = [
  { name: "GoForj", logo: LayoutDashboard, plan: "Developer Console" },
];

const user = {
  name: "goforj",
  email: "devconsole",
  avatar: "",
};

defineEmits<{
  (event: "logout"): void;
  (event: "command"): void;
}>();
</script>

<template>
  <Sidebar v-bind="props">
    <SidebarHeader>
      <TeamSwitcher :teams="teams" />
    </SidebarHeader>
    <SidebarContent>
      <NavMain :items="navMain" />
      <NavDocuments :items="navDocuments" />
      <NavSecondary
        :items="navSecondary"
        class="mt-auto"
        @command="$emit('command')"
      />
    </SidebarContent>
    <SidebarFooter>
      <NavUser :user="user" @logout="$emit('logout')" />
    </SidebarFooter>
    <SidebarRail />
  </Sidebar>
</template>
