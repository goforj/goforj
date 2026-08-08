<script setup lang="ts">
import type { SidebarProps } from './ui/sidebar'
import { computed } from 'vue'
import { BookOpen, Command, GitFork } from '@lucide/vue'
import AppBrand from './AppBrand.vue'
import NavDocuments from './NavDocuments.vue'
import NavMain from './NavMain.vue'
import NavSecondary from './NavSecondary.vue'
import NavUser from './NavUser.vue'
import goforjLogo from '../assets/goforj-logo.png'
import { appName, resourceLinks } from '@/lib/app'
import { appNavMain } from '@/lib/navigation'
import { Sidebar, SidebarContent, SidebarFooter, SidebarHeader, SidebarRail } from './ui/sidebar'

const props = withDefaults(defineProps<SidebarProps & {
  user: {
    name: string
    email: string
    avatar?: string
  } | null
}>(), {
  collapsible: 'icon',
})

const sidebarProps = computed<SidebarProps>(() => ({
  side: props.side,
  variant: props.variant,
  collapsible: props.collapsible,
  class: props.class,
}))

const documentIcons = [GitFork, BookOpen]
const navDocuments = resourceLinks.map((link, index) => ({ ...link, icon: documentIcons[index] ?? BookOpen }))

// macOS reports the Command key as metaKey; App.vue accepts either.
const isApple = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)

const navSecondary = [
  {
    title: 'Command Menu',
    icon: Command,
    action: 'command' as const,
    shortcut: isApple ? '⌘ K' : 'Ctrl + K',
  },
]

const brand = { name: appName, logoSrc: goforjLogo, logoCollapsedSrc: goforjLogo }

defineEmits<{
  (event: 'logout'): void
  (event: 'command'): void
}>()
</script>

<template>
  <Sidebar v-bind="sidebarProps">
    <SidebarHeader>
      <AppBrand v-bind="brand" />
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
