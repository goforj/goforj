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

.dashboard-stat-icon-schedules {
  color: color-mix(in oklab, #fcd34d 82%, var(--foreground));
  background: color-mix(in oklab, #f59e0b 14%, transparent);
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

.dashboard-stat-label {
  font-size: 10px;
  line-height: 1;
  text-transform: uppercase;
  color: var(--muted-foreground);
}

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
