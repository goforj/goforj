<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import Neighborhood from './eats/Neighborhood.vue'
import KitchenScene from './eats/KitchenScene.vue'
import { captions, menu, orderedHistory, stages, traceURL, type Order, type Pizza, type Progress } from './eats/model'

const pizza = ref<Pizza>('margherita')
const burn = ref(false), slow = ref(false), busy = ref(false), error = ref('')
const connection = ref('Ready to order'), grafana = ref('http://127.0.0.1:13001')
const history = ref<Progress[]>([]), orders = ref<Order[]>([])
const last = computed(() => history.value.at(-1))
const selected = computed(() => last.value?.order)
const current = computed(() => last.value?.stage === 'retrying' ? 1 : stages.findIndex(stage => stage.key === last.value?.stage))
const title = computed(() => selected.value ? `${menu.find(item => item.key === selected.value?.pizza)?.name} is ${last.value?.stage === 'delivered' ? 'at your door.' : 'in good hands.'}` : 'A fresh start, every time.')
let stream: EventSource | undefined
let generation = 0

// receive updates the tracker only from the selected order's service-authored milestones.
function receive(updates: Progress[]) {
  history.value = orderedHistory(updates)
  if (last.value?.stage === 'delivered') { stream?.close(); stream = undefined; connection.value = 'Delivered, with love' }
}

// follow replaces the previous subscription and ignores stale fetches when switching orders quickly.
async function follow(id: string) {
  const request = ++generation
  stream?.close(); stream = undefined
  const response = await fetch(`/api/orders/${encodeURIComponent(id)}`)
  if (request !== generation) return
  if (!response.ok) { localStorage.removeItem('gopher-eats-order'); throw Error('That order is no longer available. Start a fresh one.') }
  const updates: Progress[] = await response.json()
  if (request !== generation) return
  receive(updates)
  localStorage.setItem('gopher-eats-order', id)
  if (last.value?.stage === 'delivered') return
  stream = new EventSource(`/api/orders/${encodeURIComponent(id)}/events`)
  stream.addEventListener('progress', event => { if (request === generation) receive(JSON.parse((event as MessageEvent).data)) })
  stream.onopen = () => { if (request === generation) connection.value = 'Live from the services' }
  stream.onerror = () => { if (request === generation) connection.value = 'Reconnecting to your order…' }
}

// place creates real orders; lunch rush shares the kitchen's single worker and exposes each order as a tab.
async function place(count = 1) {
  busy.value = true; error.value = ''
  try {
    for (let index = 0; index < count; index++) {
      const response = await fetch('/api/orders', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ pizza: pizza.value, burn: burn.value, slow: slow.value }) })
      const order = await response.json()
      if (!response.ok) throw Error(order.error || 'Could not place your order.')
      orders.value = [...orders.value, order as Order].slice(-8)
      await follow(order.id)
    }
  } catch (cause) { error.value = cause instanceof Error ? cause.message : 'Could not connect to the storefront.' }
  finally { busy.value = false }
}

// selectOrder surfaces reconnection errors without leaving an unhandled UI promise.
function selectOrder(id: string) { follow(id).catch(cause => { error.value = cause.message }) }

onMounted(async () => {
  try { const response = await fetch('/api/eats'); if (response.ok) grafana.value = (await response.json()).grafana_url } catch { /* The tracker remains usable while Grafana is unavailable. */ }
  const saved = localStorage.getItem('gopher-eats-order')
  if (saved) selectOrder(saved)
})
onUnmounted(() => { generation++; stream?.close() })
</script>

<template>
  <header>
    <div class="brand"><span aria-hidden="true">🍕</span> GOPHER EATS <span style="font-size:11px;font-weight:400;margin-left:12px;color:#a9bb9d">by GoForj</span></div>
    <div class="header-right"><span class="tag">Fresh pizza. Connected services.</span><span style="font-size:11px"><i class="live-dot" />{{ connection }}</span></div>
  </header>
  <main>
    <div class="intro"><div><span class="eyebrow">A LITTLE PIZZA. A LOT GOING ON.</span><h1>Good things are on the way.</h1><p>From the first stretch of dough to your doorstep. Follow every moment.</p></div><div class="intro-note">Three independent apps.<br>One order you can follow.<br>Made with GoForj.</div></div>
    <div class="layout">
      <aside class="card menu">
        <h2 class="section-label">What's your slice?</h2><p class="menu-note">Your neighborhood gophers have you covered.</p>
        <button v-for="item in menu" :key="item.key" class="pizza-choice" :aria-pressed="pizza === item.key" @click="pizza = item.key"><span class="pizza-icon" aria-hidden="true">{{ item.icon }}</span><span><strong>{{ item.name }}</strong><small>{{ item.description }}</small></span></button>
        <div class="divider" /><span class="eyebrow">A TWIST IN THE STORY</span>
        <label class="switch-label"><input v-model="burn" type="checkbox"><span>Burn the first pizza<small>Watch the kitchen make it right.</small></span></label>
        <label class="switch-label"><input v-model="slow" type="checkbox"><span>Take the scenic route<small>Your courier enjoys the neighborhood.</small></span></label>
        <button class="primary" :disabled="busy" @click="place()">Place my order <span aria-hidden="true">↗</span></button>
        <p id="error" role="alert">{{ error }}</p>
        <div style="text-align:center"><button class="secondary" :disabled="busy" @click="place(3)">Lunch rush · 3 orders</button></div>
        <div class="menu-foot">DEMO PIZZA. REAL SERVICE CALLS.</div>
      </aside>
      <section class="content" aria-label="Live order tracker">
        <nav v-if="orders.length > 1" class="order-tabs" aria-label="Your orders"><button v-for="order in orders" :key="order.id" class="secondary" :aria-pressed="selected?.id === order.id" @click="selectOrder(order.id)">Order #{{ order.id.slice(0,6).toUpperCase() }}</button></nav>
        <div class="card">
          <div class="tracker-header"><div><span class="eyebrow">{{ selected ? `ORDER #${selected.id.slice(0,6).toUpperCase()}` : 'YOUR NEXT FAVORITE ORDER' }}</span><h2 class="order-title">{{ title }}</h2></div><span class="pill">{{ last?.stage.toUpperCase() || 'KITCHEN OPEN' }}</span></div>
          <div class="stages"><div v-for="(stage,index) in stages" :key="stage.key" class="stage" :class="{ done: index < current, current: index === current }" :aria-current="index === current ? 'step' : undefined"><span aria-hidden="true">{{ stage.icon }}</span>{{ stage.label }}</div></div>
          <div class="scene"><span class="scene-label">{{ last ? (last.stage === 'delivered' ? 'ANOTHER HAPPY DOORSTEP' : `${last.service.toUpperCase()} IS ON IT`) : 'WELCOME TO THE NEIGHBORHOOD' }}</span><Neighborhood v-if="!last || ['delivering', 'delivered'].includes(last.stage)" :position="last?.position || 0" :transition-ms="last?.transition_ms || 400" /><KitchenScene v-else :stage="last.stage" /><div class="caption"><strong>{{ last ? captions[last.stage] || 'Something needs attention.' : 'Something delicious starts here.' }}</strong><small>{{ last?.stage === 'delivering' ? `Courier progress · ${last.position}%` : last?.stage === 'retrying' ? 'A real queue retry' : 'Made fresh by gophers' }}</small></div></div>
          <div class="status-row"><p role="status" aria-live="polite">{{ last?.message || "Choose your pizza. We'll take care of the rest." }}</p><a v-if="selected" class="trace-link visible" :href="traceURL(grafana, selected.trace_id)" target="_blank" rel="noopener">View trace in Grafana ↗</a></div>
        </div>
        <div class="services" aria-label="Service architecture"><b>storefront</b><span class="arrow">→ HTTP →</span><b>kitchen</b><span class="arrow">→ NATS →</span><b>delivery</b><span class="arrow">→ events →</span><b>you</b></div>
        <div class="card activity"><div class="activity-top"><h2 class="section-label">Fresh from the services</h2><small>{{ history.length ? `${history.length} SERVICE UPDATES` : 'WAITING FOR YOUR FIRST ORDER' }}</small></div><ol id="feed"><li v-if="!history.length" class="empty">A little behind-the-scenes magic will appear here.</li><li v-for="update in history.slice().reverse()" :key="update.step" class="feed-item" :data-stage="update.stage"><time>{{ new Date(update.at).toLocaleTimeString([], { hour12: false }) }}</time><span class="service-name">{{ update.service }}</span><p>{{ update.message }}</p></li></ol></div>
      </section>
    </div>
    <footer><span>Built with GoForj · Multi-app composition, queues, events & OpenTelemetry</span><span>A living example. Every update comes from a real service.</span></footer>
  </main>
</template>
