<script setup lang="ts">
import { h, type Component } from "vue"
import { useI18n } from 'vue-i18n'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'

interface NavItem {
  title: string
  url: string
  icon?: Component
}

defineProps<{
  items: NavItem[]
}>()

const { t } = useI18n()

function tooltipForItem(item: NavItem): Component {
  return {
    name: 'NavMainTooltip',
    render() {
      return h('div', { class: 'flex items-center gap-2' }, [
        item.icon ? h(item.icon, { class: 'size-4 shrink-0' }) : null,
        h('span', item.title),
      ])
    },
  }
}
</script>

<template>
  <SidebarGroup>
    <SidebarGroupLabel>{{ t('nav.pages') }}</SidebarGroupLabel>
    <SidebarGroupContent>
      <SidebarMenu>
        <SidebarMenuItem v-for="item in items" :key="item.title">
          <SidebarMenuButton as-child :tooltip="tooltipForItem(item)">
            <RouterLink :to="item.url">
              <component :is="item.icon" v-if="item.icon" />
              <span>{{ item.title }}</span>
            </RouterLink>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarGroupContent>
  </SidebarGroup>
</template>
