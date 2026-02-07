<script setup lang="ts">
import {
  IconAlertTriangle,
  IconActivityHeartbeat,
  IconHeartbeat,
  IconHelp,
  IconInnerShadowTop,
  IconLink,
  IconListDetails,
  IconSearch,
  IconSettings,
} from "@tabler/icons-vue"

import NavMain from '@/components/NavMain.vue'
import NavMonitors from '@/components/NavMonitors.vue'
import NavSecondary from '@/components/NavSecondary.vue'
import NavUser from '@/components/NavUser.vue'
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
    avatar: "/favicon.ico",
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
      title: "Check History",
      url: "/check-history",
      icon: IconListDetails,
    },
    {
      title: "Metrics",
      url: "/metrics",
      icon: IconActivityHeartbeat,
    },
  ],
  navSecondary: [
    {
      title: "Settings",
      url: "/status-pages",
      icon: IconSettings,
    },
    {
      title: "Get Help",
      url: "/incidents",
      icon: IconHelp,
    },
    {
      title: "Search",
      url: "/check-history",
      icon: IconSearch,
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
            class="data-[slot=sidebar-menu-button]:!p-1.5"
          >
            <RouterLink to="/monitors">
              <IconInnerShadowTop class="!size-5" />
              <span class="text-base font-semibold">GoForj Uptime</span>
            </RouterLink>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>
    <SidebarContent>
      <NavMain :items="data.navMain" />
      <NavMonitors />
      <NavSecondary :items="data.navSecondary" class="mt-auto" />
    </SidebarContent>
    <SidebarFooter>
      <NavUser :user="data.user" />
    </SidebarFooter>
  </Sidebar>
</template>
