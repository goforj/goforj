<template>
  <div><section class="grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <Route class="h-4 w-4 text-muted-foreground" />
              Routes
            </CardTitle>
          </template>
          <template #description>
            <CardDescription>Filter routes by agent, method, or path.</CardDescription>
          </template>
          <template #action>
            <RefreshButton :on-click="refresh" />
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 grid gap-4 text-xs sm:grid-cols-2 lg:grid-cols-3">
            <FormField label="Search">
              <Input v-model="query" placeholder="Search routes..." />
            </FormField>
            <FormField v-if="showAgentFilter" label="Agent">
              <Select v-model="agentFilter">
                <option value="">All agents</option>
                <option v-for="agent in routeAgents" :key="agent.source" :value="agent.source">
                  {{ agent.source }}
                </option>
              </Select>
            </FormField>
            <FormField label="Method">
              <Select v-model="methodFilter">
                <option value="">All methods</option>
                <option v-for="method in methods" :key="method" :value="method">
                  {{ method }}
                </option>
              </Select>
            </FormField>
          </div>
          <div class="max-h-[70vh] overflow-auto rounded-xl border border-border/60">
            <table class="w-full text-xs">
              <thead class="bg-muted/40 text-muted">
                <tr>
                  <th v-if="showAgentColumn" class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Server class="h-3.5 w-3.5" />
                      Agent
                    </span>
                  </th>
                  <th class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Link2 class="h-3.5 w-3.5" />
                      Path
                    </span>
                  </th>
                  <th class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Hash class="h-3.5 w-3.5" />
                      Methods
                    </span>
                  </th>
                  <th class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Code2 class="h-3.5 w-3.5" />
                      Handler
                    </span>
                  </th>
                  <th v-if="showEditorColumn" class="px-2 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Laptop class="h-3.5 w-3.5" />
                      Editor
                    </span>
                  </th>
                  <th class="px-4 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Layers class="h-3.5 w-3.5" />
                      Middleware(s)
                    </span>
                  </th>
                  <th class="px-2 py-3 text-left">
                    <span class="inline-flex items-center gap-1">
                      <SlidersHorizontal class="h-3.5 w-3.5" />
                      Actions
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredRoutes.length === 0" class="border-t border-border/60">
                  <td
                    :colspan="showAgentColumn ? (showEditorColumn ? 7 : 6) : showEditorColumn ? 6 : 5"
                    class="px-4 py-3 text-muted"
                  >
                    No routes found.
                  </td>
                </tr>
                <tr
                  v-for="route in filteredRoutes"
                  :key="route.source + route.path + route.handler"
                  class="group border-t border-border/60"
                >
                  <td v-if="showAgentColumn" class="px-4 py-3 text-foreground">{{ route.source }}</td>
                  <td class="px-4 py-3 text-foreground">{{ route.path }}</td>
                  <td class="px-4 py-3 text-muted">{{ (route.methods || []).join(", ") }}</td>
                  <td class="px-4 py-3 text-muted">{{ route.handler }}</td>
                  <td v-if="showEditorColumn" class="px-2 py-3 text-left">
                    <EditorDropdown :symbol="editorSymbolForRoute(route)" label="Open in Editor" />
                  </td>
                  <td class="px-4 py-3 text-muted">{{ (route.middlewares || []).join(", ") }}</td>
                  <td class="px-2 py-3 text-left">
                    <button
                      class="flex h-7 w-7 items-center justify-center rounded-md border border-border/60 bg-muted/40 text-muted-foreground transition active:scale-95 active:bg-muted"
                      title="Copy route"
                      aria-label="Copy route"
                      @click="copyRoute(route)"
                    >
                      <Copy class="h-3.5 w-3.5" />
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
import { toast } from "vue-sonner";
import { useDevconsoleStore } from "../stores/devconsole";
import { Code2, Copy, Hash, Laptop, Layers, Link2, Route, Server, SlidersHorizontal } from "lucide-vue-next";
import EditorDropdown from "../components/EditorDropdown.vue";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import Select from "../components/ui/form/Select.vue";
import RefreshButton from "../components/ui/button/RefreshButton.vue";

const store = useDevconsoleStore();
const { state } = store;
const query = ref("");
const agentFilter = ref(store.state.selectedAgent || "");
const methodFilter = ref("");

const routeAgents = computed(() =>
  state.agents.filter((agent) => agent.capabilities.includes("routes"))
);

const showAgentColumn = computed(() => routeAgents.value.length > 1);
const showAgentFilter = computed(() => routeAgents.value.length > 1);
const showEditorColumn = computed(() => state.localClient);

watch(
  () => routeAgents.value,
  (agents) => {
    if (agents.length === 0) {
      agentFilter.value = "";
      return;
    }
    if (agents.length === 1) {
      agentFilter.value = agents[0].source;
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

const editorSymbolForRoute = (route: any) => {
  const handler = route.handler || "";
  if (!handler || handler === "-") return "";
  if (handler.includes(":")) return "";
  if (handler.includes(" ")) return "";
  if (!handler.includes(".")) return "";
  return handler;
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
    toast("Route copied to clipboard");
  } catch (error: any) {
    const message = error?.message || "Unable to copy route.";
    toast(message);
  }
};
</script>
