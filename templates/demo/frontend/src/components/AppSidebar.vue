<script setup lang="ts">
import {
  IconAlertTriangle,
  IconActivityHeartbeat,
  IconHeartbeat,
  IconLink,
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
              <img
                :src="uptimeGopherIcon"
                alt="Uptime Gopher"
                class="size-7 rounded-sm object-cover"
              />
              <span class="text-base font-semibold">Uptime Gopher</span>
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
