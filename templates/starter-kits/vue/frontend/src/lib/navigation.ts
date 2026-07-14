import type { Component } from 'vue'
import { Blocks, LayoutDashboard } from '@lucide/vue'

export type AppNavItem = {
  title: string
  url: string
  icon: Component
  items?: Array<{
    title: string
    url: string
  }>
}

export const appNavMain: AppNavItem[] = [
  { title: 'Dashboard', url: '/', icon: LayoutDashboard },
  {
    title: 'Components',
    url: '/components',
    icon: Blocks,
    items: [
      { title: 'Overview', url: '/components/overview' },
      { title: 'Forms', url: '/components/forms' },
      { title: 'Navigation', url: '/components/navigation' },
      { title: 'Overlays', url: '/components/overlays' },
      { title: 'Data', url: '/components/data' },
    ],
  },
]

export function findAppNavItem(path: string) {
  return appNavMain.find((item) => {
    if (item.url === '/') {
      return path === '/'
    }
    return path.startsWith(item.url) || item.items?.some((child) => path.startsWith(child.url))
  })
}
