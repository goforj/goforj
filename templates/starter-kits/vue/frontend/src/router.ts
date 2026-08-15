import { createRouter, createWebHistory } from 'vue-router'
import { appName } from '@/lib/app'
import { authState, loadCurrentUser } from '@/lib/auth'

const DashboardView = () => import('@/views/DashboardView.vue')
const ComponentsOverviewView = () => import('@/views/components/ComponentsOverviewView.vue')
const ComponentsFormsView = () => import('@/views/components/ComponentsFormsView.vue')
const ComponentsNavigationView = () => import('@/views/components/ComponentsNavigationView.vue')
const ComponentsOverlaysView = () => import('@/views/components/ComponentsOverlaysView.vue')
const ComponentsDataView = () => import('@/views/components/ComponentsDataView.vue')
const LoginView = () => import('@/views/LoginView.vue')
const ForgotPasswordView = () => import('@/views/ForgotPasswordView.vue')
const RegisterView = () => import('@/views/RegisterView.vue')
const ResetPasswordView = () => import('@/views/ResetPasswordView.vue')
const VerifyEmailView = () => import('@/views/VerifyEmailView.vue')
const SettingsLayoutView = () => import('@/views/settings/SettingsLayoutView.vue')
const SettingsProfileView = () => import('@/views/settings/SettingsProfileView.vue')
const SettingsPasswordView = () => import('@/views/settings/SettingsPasswordView.vue')
const SettingsAppearanceView = () => import('@/views/settings/SettingsAppearanceView.vue')
const NotFoundView = () => import('@/views/NotFoundView.vue')

const router = createRouter({
  history: createWebHistory(),
  scrollBehavior(_to, _from, savedPosition) {
    if (savedPosition) {
      return savedPosition
    }
    return { top: 0 }
  },
  routes: [
    { path: '/', name: 'dashboard', component: DashboardView, meta: { title: 'Dashboard' } },
    { path: '/components', redirect: '/components/overview' },
    { path: '/components/overview', name: 'components-overview', component: ComponentsOverviewView, meta: { title: 'Components Overview' } },
    { path: '/components/forms', name: 'components-forms', component: ComponentsFormsView, meta: { title: 'Components Forms' } },
    { path: '/components/navigation', name: 'components-navigation', component: ComponentsNavigationView, meta: { title: 'Components Navigation' } },
    { path: '/components/overlays', name: 'components-overlays', component: ComponentsOverlaysView, meta: { title: 'Components Overlays' } },
    { path: '/components/data', name: 'components-data', component: ComponentsDataView, meta: { title: 'Components Data' } },
    { path: '/login', name: 'login', component: LoginView, meta: { title: 'Sign in', publicShell: true } },
    { path: '/forgot-password', name: 'forgot-password', component: ForgotPasswordView, meta: { title: 'Forgot password', publicShell: true } },
    { path: '/register', name: 'register', component: RegisterView, meta: { title: 'Create account', publicShell: true } },
    { path: '/reset-password', name: 'reset-password', component: ResetPasswordView, meta: { title: 'Reset password', publicShell: true } },
    { path: '/verify-email', name: 'verify-email', component: VerifyEmailView, meta: { title: 'Verify email', publicShell: true } },
    {
      path: '/settings',
      component: SettingsLayoutView,
      meta: { title: 'Settings' },
      children: [
        { path: '', redirect: '/settings/profile' },
        { path: 'profile', name: 'settings-profile', component: SettingsProfileView, meta: { title: 'Profile settings' } },
        { path: 'password', name: 'settings-password', component: SettingsPasswordView, meta: { title: 'Password settings' } },
        { path: 'appearance', name: 'settings-appearance', component: SettingsAppearanceView, meta: { title: 'Appearance settings' } },
      ],
    },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView, meta: { title: 'Page not found' } },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.publicShell) {
    return true
  }
  if (authState.user) {
    return true
  }
  await loadCurrentUser()
  if (authState.user) {
    return true
  }
  return { name: 'login' }
})

router.afterEach((to) => {
  const title = typeof to.meta.title === 'string' ? to.meta.title : 'App'
  document.title = `${title} · ${appName}`
})

// Every route is lazily imported, so its chunk is fetched on first visit. After
// a deploy the old filenames are gone, and anyone still holding the previous
// page gets a rejected import and a blank screen on their next navigation.
// Reload once onto the new build instead, guarding against a loop when the
// failure is something other than a stale chunk.
const reloadedKey = 'route-chunk-reloaded'

router.onError((error, to) => {
  const message = error instanceof Error ? error.message : String(error)
  const staleChunk = /dynamically imported module|Importing a module script failed|Failed to fetch/i.test(message)

  if (!staleChunk || !to?.fullPath) {
    return
  }

  if (sessionStorage.getItem(reloadedKey) === to.fullPath) {
    return
  }

  sessionStorage.setItem(reloadedKey, to.fullPath)
  window.location.assign(to.fullPath)
})

router.afterEach((to) => {
  if (sessionStorage.getItem(reloadedKey) === to.fullPath) {
    sessionStorage.removeItem(reloadedKey)
  }
})

export default router
