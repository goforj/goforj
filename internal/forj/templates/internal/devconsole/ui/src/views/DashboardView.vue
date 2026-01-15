<template>
  <div>
    <PageHeader label="Platform" title="Dashboard">
      <template #right>
        <AgentPills />
        <LivePill />
      </template>
    </PageHeader>

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
          <p class="text-3xl font-semibold text-white">{{ state.agents.length }}</p>
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
          <p class="text-3xl font-semibold text-white">{{ totalRoutes }}</p>
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
          <p class="text-3xl font-semibold text-white">{{ totalSchedules }}</p>
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
            <Button @click="requestRoutesAll">Refresh</Button>
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
                <tr v-if="state.routes.length === 0" class="border-t border-border/70">
                  <td colspan="4" class="px-4 py-3 text-muted">No route data yet.</td>
                </tr>
                <tr
                  v-for="route in state.routes"
                  :key="route.path + route.handler"
                  class="border-t border-border/70"
                >
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
            <Button @click="requestSchedulesAll">Refresh</Button>
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
                <tr v-if="state.schedules.length === 0" class="border-t border-border/70">
                  <td colspan="3" class="px-4 py-3 text-muted">No schedule data yet.</td>
                </tr>
                <tr
                  v-for="schedule in state.schedules"
                  :key="schedule.id"
                  class="border-t border-border/70"
                >
                  <td class="px-4 py-3 text-white">{{ schedule.name }}</td>
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
import { computed } from "vue";
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

const { state, requestRoutesAll, requestSchedulesAll } = useDevconsoleStore();
const totalRoutes = computed(() => state.routes.length);
const totalSchedules = computed(() => state.schedules.length);

</script>
