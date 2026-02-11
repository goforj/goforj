<script setup lang="ts">
import {
  IconAlertTriangle,
  IconActivityHeartbeat,
  IconHeartbeat,
  IconLink,
  IconSettings,
  IconBellRinging,
} from "@tabler/icons-vue"

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

const data = {
  user: {
    name: "GoForj",
    email: "ops@example.com",
    avatar: "/favicon.png",
  },
  navMain: [
    {
      title: "Monitors",
      url: "/monitors",
      icon: IconHeartbeat,
    },
    {
      title: "Incidents",
      url: "/incidents",
      icon: IconAlertTriangle,
    },
    {
      title: "Status Pages",
      url: "/status-pages",
      icon: IconLink,
    },
    {
      title: "Diagnostics",
      url: "/diagnostics",
      icon: IconActivityHeartbeat,
    },
    {
      title: "Settings",
      url: "/settings",
      icon: IconSettings,
    },
    {
      title: "Notification Channels",
      url: "/settings/notification-channels",
      icon: IconBellRinging,
    },
  ],
}
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
                alt="Uptime Gopher"
                class="h-8 w-auto shrink-0 object-contain"
              />
              <span class="text-base font-semibold tracking-tight">Uptime Gopher</span>
            </RouterLink>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>
    <SidebarContent>
      <NavMain :items="data.navMain" />
      <NavMonitors />
    </SidebarContent>
    <SidebarFooter>
      <NavUser :user="data.user" />
    </SidebarFooter>
  </Sidebar>
</template>
