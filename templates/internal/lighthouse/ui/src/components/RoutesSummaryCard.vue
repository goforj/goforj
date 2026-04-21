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
