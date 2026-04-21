<template>
  <Card class="card-texture dashboard-card dashboard-card-hero">
    <CardHeader class="mb-3">
      <template #title>
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-center gap-3">
            <div class="dashboard-stat-icon dashboard-stat-icon-schedules">
              <CalendarClock class="h-4 w-4" />
            </div>
            <div>
              <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Schedules</p>
              <CardTitle>Upcoming jobs</CardTitle>
            </div>
          </div>
          <span class="dashboard-count-pill">{{ summary.total }} jobs</span>
        </div>
      </template>
    </CardHeader>
    <CardContent class="flex h-full flex-col gap-3">
      <div v-if="summary.total === 0" class="text-xs text-muted">
        No schedules reported yet.
      </div>
      <template v-else>
        <div class="flex flex-wrap gap-2">
          <span class="dashboard-inline-chip">Active {{ summary.active }}</span>
          <span v-if="summary.paused > 0" class="dashboard-inline-chip">Paused {{ summary.paused }}</span>
          <span v-if="summary.tagged > 0" class="dashboard-inline-chip">Tagged {{ summary.tagged }}</span>
        </div>
        <div class="rounded-xl border border-border/65 bg-background/55 px-3 py-2.5">
          <div class="flex items-center justify-between gap-2">
            <p class="dashboard-stat-label">Next {{ summary.upcoming.length }}</p>
            <span class="text-[10px] text-muted">Soonest runs</span>
          </div>
          <div class="mt-2 space-y-1">
            <div
              v-for="schedule in summary.upcoming"
              :key="schedule.id"
              class="flex items-start justify-between gap-3 text-xs"
            >
              <p class="min-w-0 flex-1 truncate text-[11px] font-medium text-foreground" :title="schedule.name">
                {{ schedule.name }}
              </p>
              <span class="shrink-0 truncate text-[10px] text-muted" :title="schedule.next">
                {{ schedule.next }}
              </span>
            </div>
          </div>
        </div>
        <div v-if="detailTo" class="mt-auto pt-1">
          <RouterLink :to="detailTo" class="inline-flex items-center gap-1 text-xs font-medium text-amber-300 transition hover:text-amber-200">
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
import { ArrowRight, CalendarClock } from "lucide-vue-next";
import Card from "./ui/card/Card.vue";
import CardContent from "./ui/card/CardContent.vue";
import CardHeader from "./ui/card/CardHeader.vue";
import CardTitle from "./ui/card/CardTitle.vue";
import { summarizeSchedules } from "../lib/dashboard-insights";

const props = withDefaults(
  defineProps<{
    schedules: Array<{
      id: string;
      name?: string;
      next?: string;
      next_run?: string;
      tags?: string[];
      paused?: boolean;
    }>;
    detailTo?: string;
    detailLabel?: string;
  }>(),
  {
    detailTo: "",
    detailLabel: "View schedule details",
  }
);

const summary = computed(() => summarizeSchedules(props.schedules));
</script>
