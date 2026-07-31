<script setup lang="ts">
import type { SidebarProps } from "./ui/sidebar";
import { computed } from "vue";
import {
  BookOpen,
  Github,
  Command,
} from "lucide-vue-next";
import TeamSwitcher from "./TeamSwitcher.vue";
import NavDocuments from "./NavDocuments.vue";
import NavMain from "./NavMain.vue";
import NavSecondary from "./NavSecondary.vue";
import NavUser from "./NavUser.vue";
import LighthouseMark from "./LighthouseMark.vue";
import goforjMarkDark from "../assets/goforj-mark-dark.svg";
import goforjMarkLight from "../assets/goforj-mark-light.svg";
import { appNavSections } from "../lib/navigation";
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

const navSections = appNavSections;

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
      { title: "Command Menu", url: "#", icon: Command, action: "command", shortcut: commandShortcut.value },
    ] as Array<{ title: string; url: string; icon: any; action?: "logout" | "command"; shortcut?: string }>
);

const teams = [
  {
    name: "GoForj Lighthouse",
    logo: LighthouseMark,
    plan: "GoForj Lighthouse",
  },
];

const user = {
  name: "goforj",
  email: "lighthouse",
  avatarDark: goforjMarkDark,
  avatarLight: goforjMarkLight,
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
      <NavMain :sections="navSections" />
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
