<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
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
import appMark from '@/assets/favicons/favicon-96x96.png'
import { signOut, useAuthState } from '@/lib/auth'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarRail,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'

const { t } = useI18n()
const router = useRouter()
const { currentUser } = useAuthState()

const user = computed(() => ({
  name: currentUser.value?.display_name || currentUser.value?.username || 'Operator',
  email: currentUser.value?.email || '',
  avatar: appMark,
}))

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

async function handleLogout() {
  await signOut()
  await router.replace('/login')
}
</script>

<template>
  <Sidebar collapsible="icon">
    <SidebarHeader>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="data-[slot=sidebar-menu-button]:!h-auto data-[slot=sidebar-menu-button]:!p-2"
          >
            <RouterLink to="/monitors">
              <img
                :src="appMark"
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
      <NavUser :user="user" @logout="handleLogout" />
    </SidebarFooter>
    <SidebarRail />
  </Sidebar>
</template>
