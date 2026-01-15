<template>
  <div>
    <PageHeader label="Platform" title="Routes">
      <template #right>
        <AgentPills />
        <LivePill />
      </template>
    </PageHeader>

    <section class="mt-8 grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Routes</p>
            <CardTitle>HTTP surface across agents.</CardTitle>
          </template>
          <template #description>
            <CardDescription>Filter routes by agent, method, or path.</CardDescription>
          </template>
          <template #action>
            <Button @click="refresh">Refresh</Button>
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 flex flex-wrap items-center gap-3 text-xs">
            <input
              v-model="query"
              type="text"
              placeholder="Search routes..."
              class="h-9 w-full max-w-xs rounded-lg border border-border/70 bg-white/5 px-3 text-xs text-white placeholder:text-muted focus:border-white/30 focus:outline-none"
            />
            <select
              v-model="agentFilter"
              class="h-9 rounded-lg border border-border/70 bg-white/5 px-3 text-xs text-white focus:border-white/30 focus:outline-none"
            >
              <option value="">All agents</option>
              <option v-for="agent in routeAgents" :key="agent.source" :value="agent.source">
                {{ agent.source }}
              </option>
            </select>
            <select
              v-model="methodFilter"
              class="h-9 rounded-lg border border-border/70 bg-white/5 px-3 text-xs text-white focus:border-white/30 focus:outline-none"
            >
              <option value="">All methods</option>
              <option v-for="method in methods" :key="method" :value="method">
                {{ method }}
              </option>
            </select>
          </div>
          <div class="max-h-[70vh] overflow-auto rounded-xl border border-border/70">
            <table class="w-full text-xs">
              <thead class="bg-white/5 text-muted">
                <tr>
                  <th class="px-4 py-3 text-left">Agent</th>
                  <th class="px-4 py-3 text-left">Path</th>
                  <th class="px-4 py-3 text-left">Methods</th>
                  <th class="px-4 py-3 text-left">Handler</th>
                  <th class="px-4 py-3 text-left">Middleware</th>
                  <th class="px-2 py-3 text-right"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredRoutes.length === 0" class="border-t border-border/70">
                  <td colspan="6" class="px-4 py-3 text-muted">No routes found.</td>
                </tr>
                <tr
                  v-for="route in filteredRoutes"
                  :key="route.source + route.path + route.handler"
                  class="group border-t border-border/70"
                >
                  <td class="px-4 py-3 text-white">{{ route.source }}</td>
                  <td class="px-4 py-3 text-white">{{ route.path }}</td>
                  <td class="px-4 py-3 text-muted">{{ (route.methods || []).join(", ") }}</td>
                  <td class="px-4 py-3 text-muted">{{ route.handler }}</td>
                  <td class="px-4 py-3 text-muted">{{ (route.middlewares || []).join(", ") }}</td>
                  <td class="px-2 py-3 text-right">
                    <button
                      class="rounded-md border border-border/70 bg-white/5 px-2 py-1 text-[10px] text-muted opacity-0 transition group-hover:opacity-100"
                      @click="copyRoute(route)"
                    >
                      Copy
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useDevconsoleStore } from "../stores/devconsole";
import AgentPills from "../components/AgentPills.vue";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import PageHeader from "../components/PageHeader.vue";
import LivePill from "../components/LivePill.vue";

const store = useDevconsoleStore();
const { state } = store;
const query = ref("");
const agentFilter = ref(store.state.selectedAgent || "");
const methodFilter = ref("");

const routeAgents = computed(() =>
  state.agents.filter((agent) => agent.capabilities.includes("routes"))
);

watch(
  () => routeAgents.value,
  (agents) => {
    if (agents.length === 0) {
      agentFilter.value = "";
      return;
    }
    const selected = store.state.selectedAgent;
    if (selected && agents.some((agent) => agent.source === selected)) {
      agentFilter.value = selected;
      return;
    }
    agentFilter.value = "";
  },
  { immediate: true }
);

watch(
  () => store.state.selectedAgent,
  (value) => {
    if (!value) return;
    if (routeAgents.value.some((agent) => agent.source === value)) {
      agentFilter.value = value;
    }
  }
);

const methods = computed(() =>
  Array.from(
    new Set(
      Object.values(state.routesByAgent)
        .flat()
        .flatMap((route) => route.methods || [])
    )
  )
);


const filteredRoutes = computed(() => {
  const needle = query.value.trim().toLowerCase();
  const routes = agentFilter.value
    ? state.routesByAgent[agentFilter.value] || []
    : state.routes;
  return routes
    .map((route) => ({ ...route, source: route.source || agentFilter.value || "api" }))
    .filter((route) => {
      if (methodFilter.value && !(route.methods || []).includes(methodFilter.value)) {
        return false;
      }
      if (!needle) return true;
      return (
        route.path.toLowerCase().includes(needle) ||
        route.handler.toLowerCase().includes(needle) ||
        (route.methods || []).join(", ").toLowerCase().includes(needle)
      );
    });
});

const refresh = () => {
  if (agentFilter.value) {
    store.requestRoutes(agentFilter.value);
    return;
  }
  store.requestRoutesAll();
};

const copyRoute = async (route: any) => {
  const payload = [
    `agent=${route.source}`,
    `path=${route.path}`,
    `methods=${(route.methods || []).join(",")}`,
    `handler=${route.handler}`,
    `middleware=${(route.middlewares || []).join(",")}`,
  ]
    .filter(Boolean)
    .join(" · ");
  try {
    await navigator.clipboard.writeText(payload);
  } catch {
    return;
  }
};
</script>
