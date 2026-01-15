<template>
  <div class="min-h-screen app-bg">
    <div class="app-shell">
      <aside class="sidebar-surface">
        <div class="px-6 pt-6">
          <p class="text-xs uppercase tracking-[0.35em] text-muted">GoForj</p>
          <h1 class="mt-2 text-lg font-semibold text-white">Developer Console</h1>
        </div>
        <nav class="mt-8 px-4">
          <button class="nav-item nav-item-active">Dashboard</button>
        </nav>
        <div class="mt-8 px-6">
          <p class="text-xs uppercase tracking-[0.3em] text-muted">Agents</p>
          <div class="mt-4 space-y-3">
            <div v-if="agents.length === 0" class="text-xs text-muted">
              No agents connected.
            </div>
            <div
              v-for="agent in agents"
              :key="agent.id + agent.source"
              class="rounded-xl border border-border/60 bg-white/5 p-3"
            >
              <p class="text-sm text-white">{{ agent.source }}</p>
              <p class="mt-1 text-xs text-muted">
                {{ agent.env || "unknown" }} · {{ agent.capabilities.join(", ") }}
              </p>
            </div>
          </div>
        </div>
        <div class="mt-auto px-6 pb-6 text-xs text-muted">
          <div class="mb-2">Repository</div>
          <div>Documentation</div>
        </div>
      </aside>

      <main class="main-surface">
        <header class="flex items-center justify-between">
          <div>
            <p class="text-xs uppercase tracking-[0.35em] text-muted">Platform</p>
            <h2 class="mt-2 text-2xl font-semibold text-white">Dashboard</h2>
          </div>
          <div class="status-pill">
            <span class="status-dot"></span>
            Live
          </div>
        </header>

        <section class="mt-6 grid gap-4 lg:grid-cols-3">
          <Card class="card-texture">
            <CardHeader>
              <template #title>
                <p class="text-xs uppercase tracking-[0.3em] text-muted">Agents</p>
                <CardTitle>Connected</CardTitle>
              </template>
              <template #description>
                <CardDescription>Active agents reporting in.</CardDescription>
              </template>
            </CardHeader>
            <CardContent>
              <p class="text-3xl font-semibold text-white">{{ agents.length }}</p>
            </CardContent>
          </Card>

          <Card class="card-texture">
            <CardHeader>
              <template #title>
                <p class="text-xs uppercase tracking-[0.3em] text-muted">Routes</p>
                <CardTitle>API Surface</CardTitle>
              </template>
              <template #description>
                <CardDescription>Registered HTTP endpoints.</CardDescription>
              </template>
            </CardHeader>
            <CardContent>
              <p class="text-3xl font-semibold text-white">{{ routes.length }}</p>
            </CardContent>
          </Card>

          <Card class="card-texture">
            <CardHeader>
              <template #title>
                <p class="text-xs uppercase tracking-[0.3em] text-muted">Schedules</p>
                <CardTitle>Upcoming Jobs</CardTitle>
              </template>
              <template #description>
                <CardDescription>Scheduler entries loaded.</CardDescription>
              </template>
            </CardHeader>
            <CardContent>
              <p class="text-3xl font-semibold text-white">{{ schedules.length }}</p>
            </CardContent>
          </Card>
        </section>

        <section class="mt-8 grid gap-6">
          <Card class="card-texture">
            <CardHeader>
              <template #title>
                <p class="text-xs uppercase tracking-[0.3em] text-muted">Routes</p>
                <CardTitle>Active API routes across connected agents.</CardTitle>
              </template>
              <template #action>
                <Button @click="refresh">Refresh</Button>
              </template>
            </CardHeader>
            <CardContent>
              <div class="overflow-hidden rounded-xl border border-border/70">
                <table class="w-full text-xs">
                  <thead class="bg-white/5 text-muted">
                    <tr>
                      <th class="px-4 py-3 text-left">Path</th>
                      <th class="px-4 py-3 text-left">Methods</th>
                      <th class="px-4 py-3 text-left">Handler</th>
                      <th class="px-4 py-3 text-left">Middleware</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-if="routes.length === 0" class="border-t border-border/70">
                      <td colspan="4" class="px-4 py-3 text-muted">No route data yet.</td>
                    </tr>
                    <tr v-for="route in routes" :key="route.path + route.handler" class="border-t border-border/70">
                      <td class="px-4 py-3 text-white">{{ route.path }}</td>
                      <td class="px-4 py-3 text-muted">{{ (route.methods || []).join(", ") }}</td>
                      <td class="px-4 py-3 text-muted">{{ route.handler }}</td>
                      <td class="px-4 py-3 text-muted">{{ (route.middlewares || []).join(", ") }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>

          <Card class="card-texture">
            <CardHeader>
              <template #title>
                <p class="text-xs uppercase tracking-[0.3em] text-muted">Schedules</p>
                <CardTitle>Upcoming scheduler jobs from connected agents.</CardTitle>
              </template>
              <template #action>
                <Button @click="refreshSchedules">Refresh</Button>
              </template>
            </CardHeader>
            <CardContent>
              <div class="overflow-hidden rounded-xl border border-border/70">
                <table class="w-full text-xs">
                  <thead class="bg-white/5 text-muted">
                    <tr>
                      <th class="px-4 py-3 text-left">Name</th>
                      <th class="px-4 py-3 text-left">Next Run</th>
                      <th class="px-4 py-3 text-left">Tags</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-if="schedules.length === 0" class="border-t border-border/70">
                      <td colspan="3" class="px-4 py-3 text-muted">No schedule data yet.</td>
                    </tr>
                    <tr v-for="schedule in schedules" :key="schedule.id" class="border-t border-border/70">
                      <td class="px-4 py-3 text-white">{{ schedule.name }}</td>
                      <td class="px-4 py-3 text-muted">{{ schedule.next_run }}</td>
                      <td class="px-4 py-3 text-muted">{{ (schedule.tags || []).join(", ") }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
        </section>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import Button from "./components/ui/button/Button.vue";
import Card from "./components/ui/card/Card.vue";
import CardContent from "./components/ui/card/CardContent.vue";
import CardDescription from "./components/ui/card/CardDescription.vue";
import CardHeader from "./components/ui/card/CardHeader.vue";
import CardTitle from "./components/ui/card/CardTitle.vue";

type AgentInfo = {
  id: string;
  source: string;
  env: string;
  capabilities: string[];
};

type RouteInfo = {
  path: string;
  handler: string;
  methods: string[];
  middlewares: string[];
};

type ScheduleInfo = {
  id: string;
  name: string;
  next_run: string;
  tags: string[];
};

const agents = ref<AgentInfo[]>([]);
const routes = ref<RouteInfo[]>([]);
const schedules = ref<ScheduleInfo[]>([]);
const token = new URLSearchParams(window.location.search).get("token");

const headers = () => {
  if (!token) return {};
  return { Authorization: "Bearer " + token };
};

const fetchAgents = async () => {
  const res = await fetch("/__devconsole/api/agents", { headers: headers() });
  if (!res.ok) return;
  agents.value = await res.json();
};

const requestRoutes = (socket: WebSocket) => {
  const payload = { name: "routes:list", params: {} };
  socket.send(
    JSON.stringify({
      type: "command",
      id: String(Date.now()),
      target: "api",
      payload: payload,
    })
  );
};

const requestSchedules = (socket: WebSocket) => {
  const payload = { name: "schedule:list", params: {} };
  socket.send(
    JSON.stringify({
      type: "command",
      id: String(Date.now()) + "-schedule",
      target: "scheduler",
      payload: payload,
    })
  );
};

const socketRef = ref<WebSocket | null>(null);

const connectSocket = () => {
  if (socketRef.value && socketRef.value.readyState === WebSocket.OPEN) {
    return socketRef.value;
  }
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  const url = `${scheme}://${window.location.host}/__devconsole/ws/console${
    token ? `?token=${encodeURIComponent(token)}` : ""
  }`;
  const socket = new WebSocket(url);
  socket.addEventListener("open", () => {
    requestRoutes(socket);
    requestSchedules(socket);
  });
  socket.addEventListener("message", (event) => {
    try {
      const msg = JSON.parse(event.data);
      if (msg.type !== "response") return;
      const payload =
        typeof msg.payload === "string" ? JSON.parse(msg.payload || "{}") : msg.payload || {};
      if (!payload.ok || !payload.data) return;
      const data = typeof payload.data === "string" ? JSON.parse(payload.data) : payload.data;
      if (data.routes) {
        routes.value = data.routes.map((route: RouteInfo) => ({
          ...route,
          methods: route.methods || [],
          middlewares: route.middlewares || [],
        }));
      }
      if (data.schedules) {
        schedules.value = data.schedules.map((schedule: ScheduleInfo) => ({
          ...schedule,
          tags: schedule.tags || [],
        }));
      }
    } catch {
      return;
    }
  });
  socketRef.value = socket;
  return socket;
};

const refresh = () => {
  fetchAgents();
  const socket = connectSocket();
  if (socket.readyState === WebSocket.OPEN) {
    requestRoutes(socket);
  }
};

const refreshSchedules = () => {
  const socket = connectSocket();
  if (socket.readyState === WebSocket.OPEN) {
    requestSchedules(socket);
  }
};

onMounted(() => {
  fetchAgents();
  connectSocket();
});
</script>
