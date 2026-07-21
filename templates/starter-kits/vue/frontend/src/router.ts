import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { authState, loadCurrentUser } from '@/lib/auth'

const DashboardView = () => import('@/views/DashboardView.vue')
const LoginView = () => import('@/views/LoginView.vue')
const ForgotPasswordView = () => import('@/views/ForgotPasswordView.vue')
const RegisterView = () => import('@/views/RegisterView.vue')
const ResetPasswordView = () => import('@/views/ResetPasswordView.vue')
const VerifyEmailView = () => import('@/views/VerifyEmailView.vue')
const SettingsLayoutView = () => import('@/views/settings/SettingsLayoutView.vue')
const SettingsProfileView = () => import('@/views/settings/SettingsProfileView.vue')
const SettingsPasswordView = () => import('@/views/settings/SettingsPasswordView.vue')
const SettingsAppearanceView = () => import('@/views/settings/SettingsAppearanceView.vue')

const componentShowroomRoutes: RouteRecordRaw[] = import.meta.env.DEV
  ? [
      { path: '/components', redirect: '/components/overview' },
      { path: '/components/overview', name: 'components-overview', component: () => import('@/views/components/ComponentsOverviewView.vue'), meta: { title: 'Components Overview' } },
      { path: '/components/forms', name: 'components-forms', component: () => import('@/views/components/ComponentsFormsView.vue'), meta: { title: 'Components Forms' } },
      { path: '/components/navigation', name: 'components-navigation', component: () => import('@/views/components/ComponentsNavigationView.vue'), meta: { title: 'Components Navigation' } },
      { path: '/components/overlays', name: 'components-overlays', component: () => import('@/views/components/ComponentsOverlaysView.vue'), meta: { title: 'Components Overlays' } },
      { path: '/components/data', name: 'components-data', component: () => import('@/views/components/ComponentsDataView.vue'), meta: { title: 'Components Data' } },
    ]
  : []

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
    ...componentShowroomRoutes,
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
  document.title = `${title} · GoForj`
})

export default router
