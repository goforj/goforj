import { createRouter, createWebHistory } from 'vue-router'
import { watch } from 'vue'
import MonitoringView from '@/views/MonitoringView.vue'
import LoginView from '@/views/LoginView.vue'
import MonitorEditView from '@/views/MonitorEditView.vue'
import IncidentsView from '@/views/IncidentsView.vue'
import StatusPagesView from '@/views/StatusPagesView.vue'
import StatusPublicView from '@/views/StatusPublicView.vue'
import DiagnosticsView from '@/views/DiagnosticsView.vue'
import SettingsView from '@/views/SettingsView.vue'
import { i18n } from '@/i18n'
import { ensureAuthenticated } from '@/lib/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/monitors' },
    { path: '/login', name: 'login', component: LoginView, meta: { publicShell: true, publicRoute: true, title: 'Sign In' } },
    { path: '/monitors/new', name: 'monitor-new', component: MonitorEditView, meta: { titleKey: 'routes.newMonitor' } },
    { path: '/monitors/:id/edit', name: 'monitor-edit', component: MonitorEditView, meta: { titleKey: 'routes.editMonitor' } },
    { path: '/monitors/:id?', name: 'monitoring', component: MonitoringView, meta: { titleKey: 'routes.monitoring' } },
    { path: '/incidents', name: 'incidents', component: IncidentsView, meta: { titleKey: 'routes.incidents' } },
    { path: '/status-pages', name: 'status-pages', component: StatusPagesView, meta: { titleKey: 'routes.statusPages' } },
    { path: '/diagnostics', name: 'diagnostics', component: DiagnosticsView, meta: { titleKey: 'routes.diagnostics' } },
    { path: '/settings', name: 'settings', component: SettingsView, meta: { titleKey: 'routes.settings' } },
    { path: '/settings/notification-channels', name: 'notification-channels', component: SettingsView, meta: { titleKey: 'routes.notificationChannels' } },
    { path: '/status', name: 'status-public', component: StatusPublicView, meta: { publicShell: true, publicRoute: true, titleKey: 'routes.publicStatus' } },
  ],
})

router.beforeEach(async (to) => {
  const isPublicRoute = to.meta?.publicRoute === true
  if (to.name === 'login') {
    const authenticated = await ensureAuthenticated()
    if (authenticated) {
      const redirect = typeof to.query.redirect === 'string' && to.query.redirect.trim() ? to.query.redirect : '/monitors'
      return redirect
    }
    return true
  }
  if (isPublicRoute) {
    return true
  }
  const authenticated = await ensureAuthenticated()
  if (authenticated) {
    return true
  }
  return { name: 'login', query: { redirect: to.fullPath } }
})

function updateDocumentTitle(to: { name?: unknown; params?: Record<string, unknown>; meta?: Record<string, unknown> }) {
  const pageTitle = typeof to.meta?.title === 'string'
    ? to.meta.title
    : i18n.global.t(
      to.name === 'monitoring' && typeof to.params?.id === 'string'
        ? 'routes.monitorDetail'
        : (typeof to.meta?.titleKey === 'string' ? to.meta.titleKey : 'routes.dashboard'),
    )
  const appName = i18n.global.t('app.name')
  document.title = `${pageTitle} · ${appName}`
}

router.afterEach((to) => {
  updateDocumentTitle(to)
})

watch(i18n.global.locale, () => {
  updateDocumentTitle(router.currentRoute.value)
})

export default router
