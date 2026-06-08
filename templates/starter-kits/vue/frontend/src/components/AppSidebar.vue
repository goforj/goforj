<script setup lang="ts">
import type { SidebarProps } from './ui/sidebar'
import { computed } from 'vue'
import { BookOpen, Command, Github } from 'lucide-vue-next'
import TeamSwitcher from './TeamSwitcher.vue'
import NavDocuments from './NavDocuments.vue'
import NavMain from './NavMain.vue'
import NavSecondary from './NavSecondary.vue'
import NavUser from './NavUser.vue'
import goforjLogo from '../assets/goforj-logo.png'
import { appNavMain } from '@/lib/navigation'
import { Sidebar, SidebarContent, SidebarFooter, SidebarHeader, SidebarRail } from './ui/sidebar'

const props = withDefaults(defineProps<SidebarProps & {
  user: {
    name: string
    email: string
    avatar?: string
  }
}>(), {
  collapsible: 'icon',
})

const sidebarProps = computed<SidebarProps>(() => ({
  side: props.side,
  variant: props.variant,
  collapsible: props.collapsible,
  class: props.class,
}))

const navDocuments = [
  { title: 'Repository', url: 'https://github.com/goforj/goforj', icon: Github },
  { title: 'Documentation', url: 'https://goforj.dev', icon: BookOpen },
]

const navSecondary = [
  { title: 'Command Menu', url: '#', icon: Command, action: 'command' as const, shortcut: 'Ctrl + K' },
]

const teams = [
  { name: 'GoForj Starter Kit', logoSrc: goforjLogo, logoCollapsedSrc: goforjLogo, plan: 'GoForj Starter Kit' },
]

defineEmits<{
  (event: 'logout'): void
  (event: 'command'): void
}>()
</script>

<template>
  <Sidebar v-bind="sidebarProps">
    <SidebarHeader>
      <TeamSwitcher :teams="teams" />
    </SidebarHeader>
    <SidebarContent>
      <NavMain :items="appNavMain" />
      <NavDocuments :items="navDocuments" />
      <NavSecondary :items="navSecondary" class="mt-auto" @command="$emit('command')" />
    </SidebarContent>
    <SidebarFooter>
      <NavUser :user="user" @logout="$emit('logout')" />
    </SidebarFooter>
    <SidebarRail />
  </Sidebar>
</template>
