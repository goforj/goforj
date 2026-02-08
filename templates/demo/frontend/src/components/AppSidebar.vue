<script setup lang="ts">
import {
  IconAlertTriangle,
  IconActivityHeartbeat,
  IconHeartbeat,
  IconInnerShadowTop,
  IconLink,
} from "@tabler/icons-vue"

import NavMain from '@/components/NavMain.vue'
import NavMonitors from '@/components/NavMonitors.vue'
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
      title: "Diagnostics",
      url: "/diagnostics",
      icon: IconActivityHeartbeat,
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
    </SidebarContent>
    <SidebarFooter>
      <NavUser :user="data.user" />
    </SidebarFooter>
  </Sidebar>
</template>
