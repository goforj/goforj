import { createRouter, createWebHistory } from 'vue-router'
import MonitoringView from '@/views/MonitoringView.vue'
import MonitorEditView from '@/views/MonitorEditView.vue'
import IncidentsView from '@/views/IncidentsView.vue'
import StatusPagesView from '@/views/StatusPagesView.vue'
import StatusPublicView from '@/views/StatusPublicView.vue'
import CheckHistoryView from '@/views/CheckHistoryView.vue'
import MetricsView from '@/views/MetricsView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/monitors' },
    { path: '/monitors/:id?', name: 'monitoring', component: MonitoringView },
    { path: '/monitors/new', name: 'monitor-new', component: MonitorEditView },
    { path: '/monitors/:id/edit', name: 'monitor-edit', component: MonitorEditView },
    { path: '/incidents', name: 'incidents', component: IncidentsView },
    { path: '/status-pages', name: 'status-pages', component: StatusPagesView },
    { path: '/status', name: 'status-public', component: StatusPublicView, meta: { publicShell: true } },
    { path: '/check-history', name: 'check-history', component: CheckHistoryView },
    { path: '/metrics', name: 'metrics', component: MetricsView },
  ],
})

export default router
