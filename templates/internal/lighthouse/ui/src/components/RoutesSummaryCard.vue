<template>
  <Card class="card-texture dashboard-card dashboard-card-hero">
    <CardHeader class="mb-3">
      <template #title>
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-center gap-3">
            <div class="dashboard-stat-icon dashboard-stat-icon-routes">
              <Route class="h-4 w-4" />
            </div>
            <div>
              <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Routes</p>
              <CardTitle>API surface</CardTitle>
            </div>
          </div>
          <span class="dashboard-count-pill">{{ summary.total }} endpoints</span>
        </div>
      </template>
    </CardHeader>
    <CardContent class="flex h-full flex-col gap-3">
      <div v-if="summary.total === 0" class="text-xs text-muted">
        No route data reported yet.
      </div>
      <template v-else>
        <div class="flex flex-wrap gap-2">
          <span class="dashboard-inline-chip">Handlers {{ summary.handlers }}</span>
          <span v-if="summary.agents > 0" class="dashboard-inline-chip">Agents {{ summary.agents }}</span>
          <span v-if="summary.dynamic > 0" class="dashboard-inline-chip">Dynamic {{ summary.dynamic }}</span>
        </div>
        <div class="flex flex-wrap gap-2">
          <span class="dashboard-inline-chip" v-for="item in summary.methodBreakdown" :key="item.label">
            <span :class="methodDotClass(item.label)" class="dashboard-metric-dot" />
            {{ item.label }} {{ item.count }}
          </span>
        </div>
        <div v-if="summary.prefixBreakdown.length > 0" class="flex flex-wrap gap-2">
          <span class="dashboard-inline-chip" v-for="item in summary.prefixBreakdown" :key="item.label">
            {{ item.label }} {{ item.count }}
          </span>
        </div>
        <div v-if="detailTo" class="mt-auto pt-1">
          <RouterLink :to="detailTo" class="inline-flex items-center gap-1 text-xs font-medium text-sky-300 transition hover:text-sky-200">
            {{ detailLabel }}
            <ArrowRight class="h-3.5 w-3.5" />
          </RouterLink>
        </div>
      </template>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";
import { ArrowRight, Route } from "lucide-vue-next";
import Card from "./ui/card/Card.vue";
import CardContent from "./ui/card/CardContent.vue";
import CardHeader from "./ui/card/CardHeader.vue";
import CardTitle from "./ui/card/CardTitle.vue";
import { summarizeRoutes, methodDotClass } from "../lib/dashboard-insights";

const props = withDefaults(
  defineProps<{
    routes: Array<{
      path: string;
      handler?: string;
      methods?: string[];
      source?: string;
    }>;
    detailTo?: string;
    detailLabel?: string;
  }>(),
  {
    detailTo: "",
    detailLabel: "View all routes",
  }
);

const summary = computed(() => summarizeRoutes(props.routes));
</script>

<style scoped>
.dashboard-card {
  position: relative;
  overflow: hidden;
  animation: dashFade 220ms ease-out both;
}

.dashboard-card-hero {
  min-height: 190px;
}

.dashboard-stat-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border-radius: 0.75rem;
  border: 1px solid color-mix(in oklab, var(--border) 70%, transparent);
  background: color-mix(in oklab, var(--muted) 35%, transparent);
  color: var(--foreground);
  box-shadow: inset 0 1px 0 color-mix(in oklab, white 8%, transparent);
}

.dashboard-stat-icon-routes {
  color: color-mix(in oklab, #c4b5fd 80%, var(--foreground));
  background: color-mix(in oklab, #8b5cf6 14%, transparent);
}

.dashboard-count-pill {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  border: 1px solid color-mix(in oklab, var(--border) 70%, transparent);
  background: color-mix(in oklab, var(--background) 72%, transparent);
  padding: 0.35rem 0.65rem;
  font-size: 11px;
  line-height: 1;
  color: var(--foreground);
  white-space: nowrap;
}

.dashboard-inline-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  border-radius: 999px;
  border: 1px solid color-mix(in oklab, var(--border) 70%, transparent);
  background: color-mix(in oklab, var(--background) 72%, transparent);
  padding: 0.3rem 0.55rem;
  font-size: 11px;
  line-height: 1;
  color: var(--foreground);
}

.dashboard-metric-dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  flex: 0 0 auto;
}

.dashboard-metric-dot-get { background: #7dd3fc; }
.dashboard-metric-dot-post { background: #86efac; }
.dashboard-metric-dot-put { background: #fcd34d; }
.dashboard-metric-dot-patch { background: #f9a8d4; }
.dashboard-metric-dot-delete { background: #fca5a5; }
.dashboard-metric-dot-default { background: #cbd5e1; }

@media (prefers-reduced-motion: reduce) {
  .dashboard-card {
    animation: none;
  }
}

@keyframes dashFade {
  from { opacity: 0; transform: translateY(6px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
