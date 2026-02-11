<script setup lang="ts">
import { computed } from 'vue'
import {
  IconAlertTriangle,
  IconActivityHeartbeat,
  IconHeartbeat,
  IconLink,
  IconSettings,
  IconBellRinging,
} from "@tabler/icons-vue"
import { useI18n } from 'vue-i18n'

import NavMain from '@/components/NavMain.vue'
import NavMonitors from '@/components/NavMonitors.vue'
import NavUser from '@/components/NavUser.vue'
import uptimeGopherIcon from '@/assets/uptime-gopher-icon.png'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'

const { t } = useI18n()

const user = {
  name: "GoForj",
  email: "ops@example.com",
  avatar: "/favicon.png",
}

const navMain = computed(() => [
  {
    title: t('nav.monitors'),
    url: "/monitors",
    icon: IconHeartbeat,
  },
  {
    title: t('nav.incidents'),
    url: "/incidents",
    icon: IconAlertTriangle,
  },
  {
    title: t('nav.statusPages'),
    url: "/status-pages",
    icon: IconLink,
  },
  {
    title: t('nav.diagnostics'),
    url: "/diagnostics",
    icon: IconActivityHeartbeat,
  },
  {
    title: t('nav.settings'),
    url: "/settings",
    icon: IconSettings,
  },
  {
    title: t('nav.notificationChannels'),
    url: "/settings/notification-channels",
    icon: IconBellRinging,
  },
])

const appName = computed(() => t('app.name'))
</script>

<template>
  <Sidebar collapsible="offcanvas">
    <SidebarHeader>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="data-[slot=sidebar-menu-button]:!h-auto data-[slot=sidebar-menu-button]:!p-2"
          >
            <RouterLink to="/monitors">
              <img
                :src="uptimeGopherIcon"
                :alt="appName"
                class="h-8 w-auto shrink-0 object-contain"
              />
              <span class="text-base font-semibold tracking-tight">{{ appName }}</span>
            </RouterLink>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>
    <SidebarContent>
      <NavMain :items="navMain" />
      <NavMonitors />
    </SidebarContent>
    <SidebarFooter>
      <NavUser :user="user" />
    </SidebarFooter>
  </Sidebar>
</template>
