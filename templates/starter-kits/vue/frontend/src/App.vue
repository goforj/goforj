<template>
  <div class="app-shell min-h-screen w-full bg-background text-foreground">
    <SidebarProvider
      :style="{
        '--sidebar-width': 'calc(var(--spacing) * 72)',
        '--header-height': 'calc(var(--spacing) * 16)',
      }"
    >
      <AppSidebar v-if="routeReady && !isPublicShell" :user="sidebarUser" @logout="handleLogout" @command="commandOpen = true" />

      <SidebarInset :class="showLoginLayout ? 'main-surface-login' : 'main-surface'">
        <header
          v-show="routeReady && !isPublicShell"
          data-slot="app-header"
          class="shrink-0 border-b border-border transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-(--header-height)"
        >
          <div class="flex h-(--header-height) w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
            <SidebarTrigger class="-ml-1" />
            <Separator orientation="vertical" class="mx-2 data-[orientation=vertical]:h-4" />
            <Breadcrumb>
              <BreadcrumbList>
                <template v-for="(crumb, index) in breadcrumbs" :key="crumb.url ?? crumb.title">
                  <BreadcrumbItem :class="index < breadcrumbs.length - 1 ? 'hidden md:block' : undefined">
                    <BreadcrumbLink v-if="crumb.url" as-child>
                      <RouterLink :to="crumb.url">{{ crumb.title }}</RouterLink>
                    </BreadcrumbLink>
                    <BreadcrumbPage v-else>{{ crumb.title }}</BreadcrumbPage>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator v-if="index < breadcrumbs.length - 1" class="hidden md:block" />
                </template>
              </BreadcrumbList>
            </Breadcrumb>
          </div>
        </header>

        <div :class="showLoginLayout ? 'flex flex-1' : 'main-content-area flex flex-1 flex-col gap-4 p-4 pt-4'">
          <RouterView />
        </div>
      </SidebarInset>
    </SidebarProvider>
  </div>
  <CommandMenu
    v-if="routeReady && !isPublicShell"
    :open="commandOpen"
    @update:open="(value) => (commandOpen = value)"
    @logout="handleLogout"
  />
  <Toaster />
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import AppSidebar from './components/AppSidebar.vue'
import CommandMenu from './components/CommandMenu.vue'
import { authState, loadCurrentUser, logout } from './lib/auth'
import { findAppNavItem } from './lib/navigation'
import { applyTheme, themePreference } from './lib/theme'
import { toast } from 'vue-sonner'
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
const isPublicShell = computed(() => Boolean(route.meta.publicShell))
const showLoginLayout = computed(() => !routeReady.value || isPublicShell.value)
const pageTitle = computed(() => (route.meta?.title as string) || 'Dashboard')

// Build the trail from the nav definition so a section and its child page read
// as "Components / Forms" rather than one flattened "Components Forms" label.
const breadcrumbs = computed<Array<{ title: string, url?: string }>>(() => {
  const section = findAppNavItem(route.path)
  if (!section) {
    return [{ title: pageTitle.value }]
  }

  const child = section.items?.find(item => route.path.startsWith(item.url))
  if (child) {
    return [{ title: section.title, url: section.url }, { title: child.title }]
  }

  return [{ title: section.title }]
})
const commandOpen = ref(false)
const routeReady = ref(false)
let keydownHandler: ((event: KeyboardEvent) => void) | null = null
let focusHandler: (() => void) | null = null
let visibilityHandler: (() => void) | null = null
let sessionRecheckInFlight: Promise<void> | null = null

// Null while the session is still resolving. NavUser renders a skeleton for
// this rather than a row that reads as a user named "Signed out".
const sidebarUser = computed(() => {
  const user = authState.user
  if (!user) {
    return null
  }
  return {
    name: user.display_name || user.username,
    email: user.email,
    avatar: user.avatar_url || '',
  }
})

async function handleLogout() {
  commandOpen.value = false
  await nextTick()
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

function hasActiveOverlay() {
  return Boolean(
    document.querySelector(
      [
        'dialog[open]',
        '[data-dismissable-layer][data-state="open"]',
        '[data-slot="dropdown-menu-content"][data-state="open"]',
        '[data-slot="dialog-content"][data-state="open"]',
        '[data-slot="sheet-content"][data-state="open"]',
      ].join(','),
    ),
  )
}

function releaseStaleInteractionLocks() {
  if (!isPublicShell.value || hasActiveOverlay()) {
    return
  }

  if (document.body.style.pointerEvents === 'none') {
    document.body.style.pointerEvents = ''
  }
  if (document.body.style.overflow === 'hidden') {
    document.body.style.overflow = ''
  }

  document.querySelectorAll<HTMLElement>('[data-aria-hidden="true"][aria-hidden="true"]').forEach((element) => {
    if (element.hasAttribute('data-reka-focus-guard')) {
      return
    }
    element.removeAttribute('data-aria-hidden')
    element.removeAttribute('aria-hidden')
  })
}

async function releasePublicShellLocks() {
  await nextTick()
  requestAnimationFrame(() => {
    releaseStaleInteractionLocks()
  })
}

async function revalidateSessionOnResume() {
  if (!routeReady.value || isPublicShell.value || authState.loading) {
    return
  }
  if (sessionRecheckInFlight) {
    return sessionRecheckInFlight
  }
  sessionRecheckInFlight = (async () => {
    await loadCurrentUser()
    if (!authState.user && !isPublicShell.value) {
      await router.replace('/login')
    }
  })().finally(() => {
    sessionRecheckInFlight = null
  })
  return sessionRecheckInFlight
}

onMounted(async () => {
  applyTheme(themePreference())
  await router.isReady()
  routeReady.value = true
  if (isPublicShell.value) {
    void releasePublicShellLocks()
  }
  keydownHandler = (event: KeyboardEvent) => {
    if (!routeReady.value) {
      return
    }
    if (isPublicShell.value) {
      return
    }
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
    if (!routeReady.value) {
      return
    }
    if (isPublicShell.value) {
      commandOpen.value = false
      void releasePublicShellLocks()
    }
  },
)
</script>
