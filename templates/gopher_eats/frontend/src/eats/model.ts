export type Pizza = 'margherita' | 'pepperoni' | 'garden'
export interface Order { id: string; pizza: Pizza; burn: boolean; slow: boolean; trace_id: string }
export interface Progress { order: Order; step: number; stage: string; message: string; service: string; at: string; position: number; transition_ms?: number }
export const stages = [
  { key: 'received', label: 'Received' },
  { key: 'preparing', label: 'Preparing' },
  { key: 'baking', label: 'Baking' },
  { key: 'ready', label: 'Ready' },
  { key: 'delivering', label: 'On the way' },
  { key: 'delivered', label: 'Delivered' },
]
export const menu: { key: Pizza; name: string; description: string }[] = [
  { key: 'margherita', name: 'Margherita', description: 'Tomato, mozzarella, basil' },
  { key: 'pepperoni', name: 'Pepperoni', description: 'The crowd favorite' },
  { key: 'garden', name: 'Garden party', description: 'All the good green things' },
]
export const captions: Record<string, string> = {
  received: 'Your pizza story starts now.', preparing: 'A little flour. A lot of care.',
  baking: 'Good things are worth the wait.', retrying: 'A fresh pizza is on its way.',
  ready: 'Next stop: your doorstep.', delivering: 'Here comes the good stuff.', delivered: 'Happiness, delivered.',
}

// orderedHistory tolerates transport reordering and deduplicates repeated business milestones.
export function orderedHistory(history: Progress[]): Progress[] {
  return [...new Map(history.map(update => [update.step, update])).values()].sort((a, b) => a.step - b.step)
}

// traceURL opens the order's actual trace in the provisioned Tempo datasource.
export function traceURL(base: string, id: string): string {
  const panes = { order: { datasource: 'goforj-tempo', queries: [{ refId: 'A', queryType: 'traceql', query: id }], range: { from: 'now-1h', to: 'now' } } }
  return base.replace(/\/$/, '') + '/explore?schemaVersion=1&panes=' + encodeURIComponent(JSON.stringify(panes)) + '&orgId=1'
}

// activityHistory keeps distinct workflow milestones visible while courier position updates continue.
export function activityHistory(history: Progress[]): Progress[] {
  return orderedHistory(history).filter((update, index, ordered) => index === 0 || update.stage !== ordered[index - 1]!.stage || update.service !== ordered[index - 1]!.service)
}
