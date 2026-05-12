<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  IconAlertTriangle,
  IconActivityHeartbeat,
  IconHeartbeat,
  IconLink,
  IconSettings,
  IconBellRinging,
} from "@tabler/icons-vue"
import { ChevronDown, ChevronRight } from 'lucide-vue-next'
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
const route = useRoute()
const router = useRouter()
const { currentUser } = useAuthState()
const navigationExpandedStorageKey = 'uptime-gopher:sidebar:navigation-expanded'

const user = computed(() => ({
  name: currentUser.value?.display_name || currentUser.value?.username || 'Operator',
  email: currentUser.value?.email || '',
  avatar: appMark,
}))
const pagesExpanded = ref(true)
const pagesExpandedHasStoredPreference = ref(false)
const pagesExpandedLoaded = ref(false)

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
const navigationExpanded = computed(() =>
  navMain.value
    .filter((item) => item.url !== '/monitors')
    .some((item) => route.path === item.url || route.path.startsWith(`${item.url}/`)),
)

watch(
  navigationExpanded,
  (expanded) => {
    if (pagesExpandedHasStoredPreference.value) return
    pagesExpanded.value = expanded
  },
  { immediate: true },
)

watch(pagesExpanded, (expanded) => {
  if (!pagesExpandedLoaded.value || typeof window === 'undefined') return
  window.localStorage.setItem(navigationExpandedStorageKey, expanded ? 'true' : 'false')
})

onMounted(() => {
  if (typeof window === 'undefined') return
  const stored = window.localStorage.getItem(navigationExpandedStorageKey)
  if (stored === 'true' || stored === 'false') {
    pagesExpanded.value = stored === 'true'
    pagesExpandedHasStoredPreference.value = true
  }
  pagesExpandedLoaded.value = true
})

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
            class="data-[slot=sidebar-menu-button]:!h-auto data-[slot=sidebar-menu-button]:!gap-2 data-[slot=sidebar-menu-button]:!p-1.5"
          >
            <RouterLink to="/monitors">
              <img
                :src="appMark"
                :alt="appName"
                class="h-7 w-auto shrink-0 object-contain"
              />
              <span class="text-[15px] font-semibold tracking-tight">{{ appName }}</span>
            </RouterLink>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>
    <SidebarContent class="overflow-hidden">
      <div class="px-2 pt-1">
        <button
          type="button"
          class="flex h-7 w-full items-center justify-between rounded-md px-2 text-[11px] font-medium tracking-[0.08em] text-muted-foreground uppercase hover:bg-muted/40 hover:text-foreground"
          :aria-expanded="pagesExpanded"
          @click="pagesExpanded = !pagesExpanded"
        >
          <span>Navigation</span>
          <ChevronDown v-if="pagesExpanded" class="size-3.5" />
          <ChevronRight v-else class="size-3.5" />
        </button>
      </div>
      <NavMain v-if="pagesExpanded" :items="navMain" compact />
      <NavMonitors />
    </SidebarContent>
    <SidebarFooter>
      <NavUser :user="user" @logout="handleLogout" />
    </SidebarFooter>
    <SidebarRail />
  </Sidebar>
</template>
