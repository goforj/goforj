import { createRouter, createWebHistory } from 'vue-router'
import MonitoringView from '@/views/MonitoringView.vue'
import MonitorEditView from '@/views/MonitorEditView.vue'
import IncidentsView from '@/views/IncidentsView.vue'
import StatusPagesView from '@/views/StatusPagesView.vue'
import StatusPublicView from '@/views/StatusPublicView.vue'
import DiagnosticsView from '@/views/DiagnosticsView.vue'
import SettingsView from '@/views/SettingsView.vue'

const APP_NAME = 'Uptime Gopher'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/monitors' },
    { path: '/monitors/new', name: 'monitor-new', component: MonitorEditView, meta: { title: 'New Monitor' } },
    { path: '/monitors/:id/edit', name: 'monitor-edit', component: MonitorEditView, meta: { title: 'Edit Monitor' } },
    { path: '/monitors/:id?', name: 'monitoring', component: MonitoringView, meta: { title: 'Monitoring' } },
    { path: '/incidents', name: 'incidents', component: IncidentsView, meta: { title: 'Incidents' } },
    { path: '/status-pages', name: 'status-pages', component: StatusPagesView, meta: { title: 'Status Pages' } },
    { path: '/diagnostics', name: 'diagnostics', component: DiagnosticsView, meta: { title: 'Diagnostics' } },
    { path: '/settings', name: 'settings', component: SettingsView, meta: { title: 'Settings' } },
    { path: '/status', name: 'status-public', component: StatusPublicView, meta: { publicShell: true, title: 'Public Status' } },
  ],
})

router.afterEach((to) => {
  const pageTitle = to.name === 'monitoring' && typeof to.params?.id === 'string'
    ? 'Monitor Detail'
    : (typeof to.meta?.title === 'string' ? to.meta.title : 'Dashboard')
  document.title = `${pageTitle} · ${APP_NAME}`
})

export default router
