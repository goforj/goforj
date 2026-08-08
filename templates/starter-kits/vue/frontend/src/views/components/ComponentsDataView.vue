<template>
  <section class="grid gap-6">
    <PageHeader
      eyebrow="Components"
      section="Data"
      title="Tables, pagination, and dates"
      description="Reference patterns for resource indexes, operational reporting, scheduling, and review workflows."
    />

    <div class="grid gap-6 md:grid-cols-3">
      <Card v-for="metric in metrics" :key="metric.label">
        <CardHeader class="gap-1">
          <CardDescription>{{ metric.label }}</CardDescription>
          <CardTitle class="text-2xl font-semibold tracking-tight">{{ metric.value }}</CardTitle>
        </CardHeader>
        <CardContent>
          <p class="text-sm text-muted-foreground">{{ metric.copy }}</p>
        </CardContent>
      </Card>
    </div>

    <Specimen
      name="Table"
      description="A low-level primitive with no opinion on chrome. These are three product treatments built from the same parts: an admin index, a dense event stream, and a financial summary."
      :also="['Badge', 'Button', 'InputGroup', 'Select']"
      :source="TableExampleSource"
    >
      <TableExample />
    </Specimen>

    <Specimen
      name="Pagination"
      description="Page controls with ellipsis collapsing. Drives the page number through v-model:page."
      :source="PaginationExampleSource"
    >
      <PaginationExample />
    </Specimen>

    <div class="grid items-start gap-6 xl:grid-cols-2">
      <Specimen
        name="Calendar"
        description="Single date selection for launch planning and reporting windows."
        :also="['Input']"
        :source="CalendarExampleSource"
      >
      <CalendarExample />
      </Specimen>

      <Specimen
        name="RangeCalendar"
        description="Start and end selection for booking flows and release coordination."
        :also="['Input']"
        :source="RangeCalendarExampleSource"
      >
      <RangeCalendarExample />
      </Specimen>
    </div>

    <Specimen
      name="Item"
      description="A compact row with content and trailing actions. Use for queues, scheduled windows, and other operational lists."
      :also="['Badge']"
      :source="ItemListExampleSource"
    >
      <ItemListExample />
    </Specimen>

    <Showcase
      title="Reporting snapshots"
      description="Small summary blocks help table and calendar sections read as a dashboard instead of isolated widgets."
      content-class="md:grid-cols-3"
    >
      <ShowcaseRow v-for="snapshot in snapshots" :key="snapshot.label" class="gap-1">
        <p class="text-sm text-muted-foreground">{{ snapshot.label }}</p>
        <p class="text-2xl font-semibold tracking-tight">{{ snapshot.value }}</p>
        <p class="text-sm text-muted-foreground">{{ snapshot.copy }}</p>
      </ShowcaseRow>
    </Showcase>
  </section>
</template>

<script setup lang="ts">
import TableExample from './examples/TableExample.vue'
import TableExampleSource from './examples/TableExample.vue?raw'
import PaginationExample from './examples/PaginationExample.vue'
import PaginationExampleSource from './examples/PaginationExample.vue?raw'
import CalendarExample from './examples/CalendarExample.vue'
import CalendarExampleSource from './examples/CalendarExample.vue?raw'
import RangeCalendarExample from './examples/RangeCalendarExample.vue'
import RangeCalendarExampleSource from './examples/RangeCalendarExample.vue?raw'
import ItemListExample from './examples/ItemListExample.vue'
import ItemListExampleSource from './examples/ItemListExample.vue?raw'
import PageHeader from '@/components/PageHeader.vue'
import { Showcase, ShowcaseRow, Specimen } from '@/components/showcase'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

const metrics = [
  { label: 'Indexed resources', value: '128', copy: 'Routes, screens, and generated surfaces tracked by the shell.' },
  { label: 'Ready to ship', value: '42', copy: 'Resources currently in a production-ready state.' },
  { label: 'Scheduled windows', value: '3', copy: 'Upcoming launches, cutovers, or maintenance windows.' },
]

const snapshots = [
  { label: 'Daily events', value: '18.4k', copy: 'Across API, auth, and renderer flows.' },
  { label: 'Failed jobs', value: '12', copy: 'Down from 31 after the last deploy.' },
  { label: 'Median latency', value: '84ms', copy: 'Healthy for the current load profile.' },
]
</script>
