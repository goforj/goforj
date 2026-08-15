<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { type DateValue, getLocalTimeZone, parseDate } from '@internationalized/date'
import type { DateRange } from 'reka-ui'
import { Input } from '@/components/ui/input'
import { RangeCalendar } from '@/components/ui/range-calendar'

// shallowRef, not ref. Dates are immutable value objects, and ref's deep
// unwrapping strips the private field the DateValue union is discriminated
// on, leaving a type that will not assign to RangeCalendar's model.
const range = shallowRef<DateRange>({ start: parseDate('2026-04-23'), end: parseDate('2026-04-30') })

const formatter = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
function label(value?: DateValue | null) {
  return value ? formatter.format(value.toDate(getLocalTimeZone())) : ''
}

const start = computed(() => label(range.value?.start) || 'Start date')
const end = computed(() => label(range.value?.end) || 'End date')
</script>

<template>
  <div class="grid justify-items-center gap-3">
    <div class="flex flex-wrap justify-center gap-2">
      <Input :model-value="start" readonly class="h-8 w-44 text-center text-sm" />
      <Input :model-value="end" readonly class="h-8 w-44 text-center text-sm" />
    </div>
    <RangeCalendar v-model="range" class="rounded-lg border p-3" />
  </div>
</template>
