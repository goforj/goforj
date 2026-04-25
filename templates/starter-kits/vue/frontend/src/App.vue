<template>
  <div class="app-shell min-h-screen w-full bg-background text-foreground">
    <SidebarProvider
      :style="{
        '--sidebar-width': 'calc(var(--spacing) * 72)',
        '--header-height': 'calc(var(--spacing) * 12)',
      }"
    >
      <AppSidebar v-if="!isLogin" :user="sidebarUser" @logout="handleLogout" @command="commandOpen = true" />

      <SidebarInset :class="isLogin ? 'main-surface-login' : 'main-surface'">
        <header
          v-if="!isLogin"
          data-slot="app-header"
          class="shrink-0 border-b border-border transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-(--header-height)"
        >
          <div class="flex h-(--header-height) w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
            <SidebarTrigger class="-ml-1" />
            <Separator orientation="vertical" class="mx-2 data-[orientation=vertical]:h-4" />
            <Breadcrumb>
              <BreadcrumbList>
                <BreadcrumbItem class="hidden md:block">
                  <BreadcrumbLink href="#">Application</BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator class="hidden md:block" />
                <BreadcrumbItem>
                  <BreadcrumbPage class="inline-flex items-center gap-1.5">
                    <component :is="pageIcon" v-if="pageIcon" class="size-4 text-muted-foreground" />
                    {{ pageTitle }}
                  </BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>
            <div class="ml-auto flex items-center gap-2 pr-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                class="gap-2"
                :aria-pressed="isDark"
                aria-label="Toggle theme"
                @click="toggleTheme"
              >
                <span class="hidden sm:inline">{{ isDark ? 'Dark' : 'Light' }}</span>
                <Sun v-if="!isDark" class="size-4" aria-hidden="true" />
                <Moon v-else class="size-4" aria-hidden="true" />
              </Button>
            </div>
          </div>
        </header>

        <div :class="isLogin ? 'flex flex-1' : 'main-content-area flex flex-1 flex-col gap-4 p-4 pt-4'">
          <RouterView />
        </div>
      </SidebarInset>
    </SidebarProvider>
  </div>
  <Teleport to="body">
    <CommandMenu
      v-if="!isLogin"
      :open="commandOpen"
      @update:open="(value) => (commandOpen = value)"
      @logout="handleLogout"
    />
  </Teleport>
  <Toaster />
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import { Moon, Sun } from 'lucide-vue-next'
import AppSidebar from './components/AppSidebar.vue'
import CommandMenu from './components/CommandMenu.vue'
import { authState, loadCurrentUser, logout } from './lib/auth'
import { findAppNavItem } from './lib/navigation'
import { applyTheme, setThemePreference, themePreference } from './lib/theme'
import { toast } from 'vue-sonner'
import { Button } from './components/ui/button'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from './components/ui/breadcrumb'
import { Separator } from './components/ui/separator'
import { SidebarInset, SidebarProvider, SidebarTrigger } from './components/ui/sidebar'
import { Toaster } from './components/ui/sonner'

const route = useRoute()
const router = useRouter()
const isLogin = computed(() => route.path === '/login')
const pageTitle = computed(() => (route.meta?.title as string) || 'Dashboard')
const pageIcon = computed(() => findAppNavItem(route.path)?.icon)
const isDark = ref(document.documentElement.classList.contains('dark'))
const commandOpen = ref(false)
let keydownHandler: ((event: KeyboardEvent) => void) | null = null
let focusHandler: (() => void) | null = null
let visibilityHandler: (() => void) | null = null
let sessionRecheckInFlight: Promise<void> | null = null

const sidebarUser = computed(() => {
  const user = authState.user
  return {
    name: user?.display_name || user?.username || 'Signed out',
    email: user?.email || 'No active session',
    avatar: user?.avatar_url || '',
  }
})

function toggleTheme() {
  isDark.value = !isDark.value
  setThemePreference(isDark.value ? 'dark' : 'light')
}

async function handleLogout() {
  try {
    await logout()
  } catch (error) {
    toast.error('Sign out failed', {
      description: error instanceof Error ? error.message : 'Unable to log out.',
    })
  } finally {
    await router.replace('/login')
  }
}

async function requireSession() {
  if (isLogin.value) {
    return
  }
  if (!authState.user) {
    await loadCurrentUser()
  }
  if (!authState.user) {
    await router.replace('/login')
  }
}

async function revalidateSessionOnResume() {
  if (isLogin.value || authState.loading) {
    return
  }
  if (sessionRecheckInFlight) {
    return sessionRecheckInFlight
  }
  sessionRecheckInFlight = (async () => {
    await loadCurrentUser()
    if (!authState.user && !isLogin.value) {
      await router.replace('/login')
    }
  })().finally(() => {
    sessionRecheckInFlight = null
  })
  return sessionRecheckInFlight
}

onMounted(() => {
  applyTheme(themePreference())
  isDark.value = document.documentElement.classList.contains('dark')
  void requireSession()
  keydownHandler = (event: KeyboardEvent) => {
    const target = event.target as HTMLElement | null
    const tag = target?.tagName?.toLowerCase()
    if (tag === 'input' || tag === 'textarea' || tag === 'select' || target?.isContentEditable) {
      return
    }
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault()
      commandOpen.value = !commandOpen.value
    }
  }
  window.addEventListener('keydown', keydownHandler)
  focusHandler = () => {
    void revalidateSessionOnResume()
  }
  visibilityHandler = () => {
    if (document.visibilityState === 'visible') {
      void revalidateSessionOnResume()
    }
  }
  window.addEventListener('focus', focusHandler)
  document.addEventListener('visibilitychange', visibilityHandler)
})

onBeforeUnmount(() => {
  if (keydownHandler) {
    window.removeEventListener('keydown', keydownHandler)
  }
  if (focusHandler) {
    window.removeEventListener('focus', focusHandler)
  }
  if (visibilityHandler) {
    document.removeEventListener('visibilitychange', visibilityHandler)
  }
})

watch(
  () => route.path,
  () => {
    void requireSession()
  },
)
</script>
