<template>
  <div>
    <section class="grid gap-4 lg:grid-cols-3">
      <Card class="card-texture dashboard-card dashboard-card-hero">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Jobs</p>
            <CardTitle>Queue totals</CardTitle>
          </template>
          <template #description>
            <CardDescription>Current queue load and throughput.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="grid grid-cols-2 gap-3 text-xs text-muted">
            <div>
              <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Pending</p>
              <p class="text-lg font-semibold text-foreground">{{ jobTotals.pending }}</p>
            </div>
            <div>
              <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Active</p>
              <p class="text-lg font-semibold text-foreground">{{ jobTotals.active }}</p>
            </div>
            <div>
              <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Scheduled</p>
              <p class="text-lg font-semibold text-foreground">{{ jobTotals.scheduled }}</p>
            </div>
            <div>
              <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Retry</p>
              <p class="text-lg font-semibold text-foreground">{{ jobTotals.retry }}</p>
            </div>
          </div>
          <div class="mt-4 flex items-center gap-4 text-xs text-muted">
            <span>Processed: <strong class="text-foreground">{{ jobTotals.processed }}</strong></span>
            <span>Failed: <strong class="text-foreground">{{ jobTotals.failed }}</strong></span>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture dashboard-card dashboard-card-hero">
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
          <p class="text-3xl font-semibold text-foreground">{{ totalRoutes }}</p>
        </CardContent>
      </Card>

      <Card class="card-texture dashboard-card dashboard-card-hero">
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
          <p class="text-3xl font-semibold text-foreground">{{ totalSchedules }}</p>
        </CardContent>
      </Card>
    </section>

    <section class="mt-6 grid gap-6">
      <Card class="card-texture dashboard-card">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Routes</p>
            <CardTitle>Active API routes across connected agents.</CardTitle>
          </template>
          <template #action>
            <RefreshButton :on-click="requestRoutesAll" />
          </template>
        </CardHeader>
        <CardContent>
          <div class="overflow-hidden rounded-xl border border-border/60">
            <table class="w-full text-xs">
              <thead class="bg-muted/40 text-muted">
                <tr>
                  <th class="px-4 py-3 text-left">Path</th>
                  <th class="px-4 py-3 text-left">Methods</th>
                  <th class="px-4 py-3 text-left">Handler</th>
                  <th class="px-4 py-3 text-left">Middleware</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="state.routes.length === 0" class="border-t border-border/60">
                  <td colspan="4" class="px-4 py-3 text-muted">No route data yet.</td>
                </tr>
                <tr
                  v-for="route in state.routes"
                  :key="route.path + route.handler"
                  class="border-t border-border/60"
                >
                  <td class="px-4 py-3 text-foreground">{{ route.path }}</td>
                  <td class="px-4 py-3 text-muted">{{ (route.methods || []).join(", ") }}</td>
                  <td class="px-4 py-3 text-muted">{{ route.handler }}</td>
                  <td class="px-4 py-3 text-muted">{{ (route.middlewares || []).join(", ") }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture dashboard-card">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Schedules</p>
            <CardTitle>Upcoming scheduler jobs from connected agents.</CardTitle>
          </template>
          <template #action>
            <RefreshButton :on-click="requestSchedulesAll" />
          </template>
        </CardHeader>
        <CardContent>
          <div class="overflow-hidden rounded-xl border border-border/60">
            <table class="w-full text-xs">
              <thead class="bg-muted/40 text-muted">
                <tr>
                  <th class="px-4 py-3 text-left">Name</th>
                  <th class="px-4 py-3 text-left">Next Run</th>
                  <th class="px-4 py-3 text-left">Tags</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="state.schedules.length === 0" class="border-t border-border/60">
                  <td colspan="3" class="px-4 py-3 text-muted">No schedule data yet.</td>
                </tr>
                <tr
                  v-for="schedule in state.schedules"
                  :key="schedule.id"
                  class="border-t border-border/60"
                >
                  <td class="px-4 py-3 text-foreground">{{ schedule.name }}</td>
                  <td class="px-4 py-3 text-muted">{{ schedule.next || schedule.next_run }}</td>
                  <td class="px-4 py-3 text-muted">{{ (schedule.tags || []).join(", ") }}</td>
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
import { computed, onMounted, ref, watch } from "vue";
import { useDevconsoleStore } from "../stores/devconsole";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import RefreshButton from "../components/ui/button/RefreshButton.vue";

const { state, requestRoutesAll, requestSchedulesAll, sendCommand } = useDevconsoleStore();
const totalRoutes = computed(() => state.routes.length);
const totalSchedules = computed(() => state.schedules.length);
const jobTotals = ref({
  pending: 0,
  active: 0,
  scheduled: 0,
  retry: 0,
  processed: 0,
  failed: 0,
});

const refreshJobTotals = async () => {
  const agent = state.agents.find(
    (entry) =>
      entry.capabilities.includes("queue") ||
      entry.capabilities.includes("jobs") ||
      entry.source === "jobs"
  );
  if (!agent) {
    jobTotals.value = { pending: 0, active: 0, scheduled: 0, retry: 0, processed: 0, failed: 0 };
    return;
  }
  const result = await sendCommand(agent.source, "queue:queues", {});
  if (!result?.data) return;
  const payload = typeof result.data === "string" ? JSON.parse(result.data) : result.data;
  const queues = payload.queues || [];
  jobTotals.value = queues.reduce(
    (acc: typeof jobTotals.value, queue: any) => ({
      pending: acc.pending + (queue.pending || 0),
      active: acc.active + (queue.active || 0),
      scheduled: acc.scheduled + (queue.scheduled || 0),
      retry: acc.retry + (queue.retry || 0),
      processed: acc.processed + (queue.processed || 0),
      failed: acc.failed + (queue.failed || 0),
    }),
    { pending: 0, active: 0, scheduled: 0, retry: 0, processed: 0, failed: 0 }
  );
};

onMounted(() => {
  refreshJobTotals();
});

watch(
  () => state.agents,
  () => {
    refreshJobTotals();
  }
);

</script>
