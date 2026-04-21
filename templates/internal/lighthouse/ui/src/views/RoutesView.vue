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
        <CardContent class="space-y-4">
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
          <div class="routes-overview-strip">
            <span class="routes-overview-chip">
              <span class="routes-overview-chip-label">Routes</span>
              <span class="routes-overview-chip-value">{{ filteredRoutes.length }}</span>
            </span>
            <span class="routes-overview-chip">
              <span class="routes-overview-chip-label">Handlers</span>
              <span class="routes-overview-chip-value">{{ routeOverview.uniqueHandlers }}</span>
            </span>
            <span class="routes-overview-chip">
              <span class="routes-overview-chip-label">Dynamic</span>
              <span class="routes-overview-chip-value">{{ routeOverview.dynamicRoutes }}</span>
            </span>
            <span class="routes-overview-chip">
              <span class="routes-overview-chip-label">Middleware</span>
              <span class="routes-overview-chip-value">{{ routeOverview.middlewareRoutes }}</span>
            </span>
          </div>
          <div v-if="routeOverview.methodBreakdown.length > 0" class="flex flex-wrap gap-2">
            <span
              v-for="item in routeOverview.methodBreakdown"
              :key="item.label"
              class="routes-method-pill"
              :class="methodPillClass(item.label)"
            >
              <span class="routes-method-pill-dot" />
              {{ item.label }} {{ item.count }}
            </span>
          </div>
          <div
            class="max-h-[calc(100vh-21rem)] overflow-auto rounded-xl border border-border/60"
            :class="filteredRoutes.length > 8 ? 'min-h-[22rem]' : ''"
          >
            <table class="w-full text-xs">
              <thead class="sticky top-0 z-10 bg-muted/85 text-muted backdrop-blur supports-[backdrop-filter]:bg-muted/70">
                <tr>
                  <th v-if="showAgentColumn" class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Server class="h-3.5 w-3.5" />
                      Agent
                    </span>
                  </th>
                  <th class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Link2 class="h-3.5 w-3.5" />
                      Path
                    </span>
                  </th>
                  <th class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Hash class="h-3.5 w-3.5" />
                      Methods
                    </span>
                  </th>
                  <th class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Code2 class="h-3.5 w-3.5" />
                      Handler
                    </span>
                  </th>
                  <th class="px-4 py-2.5 text-left">
                    <span class="inline-flex items-center gap-1">
                      <Layers class="h-3.5 w-3.5" />
                      Middleware(s)
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredRoutes.length === 0" class="border-t border-border/60">
                  <td
                    :colspan="showAgentColumn ? 5 : 4"
                    class="px-4 py-2.5 text-muted"
                  >
                    No routes found.
                  </td>
                </tr>
                <tr
                  v-for="route in filteredRoutes"
                  :key="route.source + route.path + route.handler"
                  class="routes-table-row group border-t border-border/60"
                >
                  <td v-if="showAgentColumn" class="px-4 py-2 text-foreground">
                    <span class="routes-agent-pill">{{ route.source }}</span>
                  </td>
                  <td class="px-4 py-2 text-foreground">
                    <div class="routes-path-shell">
                      <span class="routes-path-root">/</span>
                      <template v-for="(segment, index) in routePathSegments(route.path)" :key="route.path + segment + index">
                        <span v-if="index > 0" class="routes-path-divider">/</span>
                        <span :class="isDynamicPathSegment(segment) ? 'routes-path-segment routes-path-segment-dynamic' : 'routes-path-segment'">
                          {{ segment }}
                        </span>
                      </template>
                    </div>
                  </td>
                  <td class="px-4 py-2 text-muted">
                    <div class="flex flex-wrap gap-1.5">
                      <span
                        v-for="method in route.methods || []"
                        :key="route.path + method"
                        class="routes-method-pill routes-method-pill-sm"
                        :class="methodPillClass(method)"
                      >
                        <span class="routes-method-pill-dot" />
                        {{ method }}
                      </span>
                    </div>
                  </td>
                  <td class="px-4 py-2 text-muted">
                    <div class="flex items-center gap-2 min-w-0">
                      <span class="routes-handler-chip min-w-0 truncate">{{ route.handler }}</span>
                      <EditorDropdown
                        v-if="showEditorColumn && canOpenEditorSymbol(editorSymbolForRoute(route))"
                        :symbol="editorSymbolForRoute(route)"
                        label="Open in Editor"
                        compact
                      />
                    </div>
                  </td>
                  <td class="px-4 py-2 text-muted">
                    <div v-if="(route.middlewares || []).length > 0" class="flex flex-wrap gap-1.5">
                      <div
                        v-for="middleware in (route.middlewares || []).slice(0, 3)"
                        :key="route.path + middleware"
                        class="flex items-center gap-1.5"
                      >
                        <span class="routes-middleware-chip">
                          {{ middleware }}
                        </span>
                        <EditorDropdown
                          v-if="showEditorColumn && canOpenEditorSymbol(editorSymbolForMiddleware(middleware))"
                          :symbol="editorSymbolForMiddleware(middleware)"
                          :label="`Open ${middleware} in Editor`"
                          compact
                        />
                      </div>
                      <span
                        v-if="(route.middlewares || []).length > 3"
                        class="routes-middleware-chip routes-middleware-chip-overflow"
                      >
                        +{{ (route.middlewares || []).length - 3 }} more
                      </span>
                    </div>
                    <span v-else class="text-muted/80">None</span>
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
import { useLighthouseStore } from "../stores/lighthouse";
import { Code2, Hash, Layers, Link2, Route, Server } from "lucide-vue-next";
import EditorDropdown from "../components/EditorDropdown.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import Select from "../components/ui/form/Select.vue";
import RefreshButton from "../components/ui/button/RefreshButton.vue";

const store = useLighthouseStore();
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

const routeOverview = computed(() => {
  const uniqueHandlers = new Set<string>();
  const methodCounts = new Map<string, number>();
  let dynamicRoutes = 0;
  let middlewareRoutes = 0;

  for (const route of filteredRoutes.value) {
    if (route.handler) {
      uniqueHandlers.add(route.handler);
    }
    if ((route.middlewares || []).length > 0) {
      middlewareRoutes += 1;
    }
    if (routePathSegments(route.path).some((segment) => isDynamicPathSegment(segment))) {
      dynamicRoutes += 1;
    }
    for (const method of route.methods || []) {
      methodCounts.set(method, (methodCounts.get(method) || 0) + 1);
    }
  }

  return {
    uniqueHandlers: uniqueHandlers.size,
    dynamicRoutes,
    middlewareRoutes,
    methodBreakdown: Array.from(methodCounts.entries())
      .map(([label, count]) => ({ label, count }))
      .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
      .slice(0, 6),
  };
});

const editorCandidateSymbols = computed(() =>
  Array.from(
    new Set(
      filteredRoutes.value
        .flatMap((route) => [
          editorSymbolForRoute(route),
          ...(route.middlewares || []).map((middleware) => editorSymbolForMiddleware(middleware)),
        ])
        .filter(Boolean)
    )
  )
);

const routePathSegments = (path: string) => path.split("/").filter(Boolean);

const isDynamicPathSegment = (segment: string) =>
  segment.includes(":") || segment.includes("*") || (segment.includes("{") && segment.includes("}"));

const methodPillClass = (method: string) => {
  switch (method.toUpperCase()) {
    case "GET":
      return "routes-method-pill-get";
    case "POST":
      return "routes-method-pill-post";
    case "PUT":
      return "routes-method-pill-put";
    case "PATCH":
      return "routes-method-pill-patch";
    case "DELETE":
      return "routes-method-pill-delete";
    default:
      return "routes-method-pill-default";
  }
};

const canOpenEditorSymbol = (symbol?: string) => store.canOpenEditorSymbol(symbol);

const refresh = () => {
  if (agentFilter.value) {
    store.requestRoutes(agentFilter.value);
    return;
  }
  store.requestRoutesAll();
};

const editorSymbolForRoute = (route: any) => {
  const handler = route.handler || "";
  return resolveEditorSymbol(handler);
};

const editorSymbolForMiddleware = (middleware: string) => resolveEditorSymbol(middleware);

const resolveEditorSymbol = (value: string) => {
  const symbol = String(value || "").trim();
  if (!symbol || symbol === "-") return "";
  if (symbol.includes(":")) return "";
  if (symbol.includes(" ")) return "";
  if (!symbol.includes(".")) return "";
  return symbol;
};

watch(
  () => [showEditorColumn.value, ...editorCandidateSymbols.value],
  async ([enabled]) => {
    if (!enabled || editorCandidateSymbols.value.length === 0) {
      return;
    }
    try {
      await store.validateEditorSymbols(editorCandidateSymbols.value);
    } catch {
      //
    }
  },
  { immediate: true }
);
</script>
