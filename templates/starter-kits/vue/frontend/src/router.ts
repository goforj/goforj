import { createRouter, createWebHistory } from 'vue-router'

const DashboardView = () => import('@/views/DashboardView.vue')
const ComponentsOverviewView = () => import('@/views/components/ComponentsOverviewView.vue')
const ComponentsFormsView = () => import('@/views/components/ComponentsFormsView.vue')
const ComponentsNavigationView = () => import('@/views/components/ComponentsNavigationView.vue')
const ComponentsOverlaysView = () => import('@/views/components/ComponentsOverlaysView.vue')
const ComponentsDataView = () => import('@/views/components/ComponentsDataView.vue')
const LoginView = () => import('@/views/LoginView.vue')
const SettingsLayoutView = () => import('@/views/settings/SettingsLayoutView.vue')
const SettingsProfileView = () => import('@/views/settings/SettingsProfileView.vue')
const SettingsPasswordView = () => import('@/views/settings/SettingsPasswordView.vue')
const SettingsAppearanceView = () => import('@/views/settings/SettingsAppearanceView.vue')

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

router.afterEach((to) => {
  const title = typeof to.meta.title === 'string' ? to.meta.title : 'App'
  document.title = `${title} · GoForj`
})

export default router
