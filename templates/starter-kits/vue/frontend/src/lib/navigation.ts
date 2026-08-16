import type { Component } from 'vue'
import { LayoutDashboard } from '@lucide/vue'
// goforj:component-library:on:start
import { Blocks } from '@lucide/vue'
// goforj:component-library:end

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
  // goforj:component-library:on:start
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
  // goforj:component-library:end
]

export function findAppNavItem(path: string) {
  return appNavMain.find((item) => {
    if (item.url === '/') {
      return path === '/'
    }
    return path.startsWith(item.url) || item.items?.some((child) => path.startsWith(child.url))
  })
}
