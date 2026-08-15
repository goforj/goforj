<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { type DateValue, getLocalTimeZone, today } from '@internationalized/date'
import { Calendar } from '@/components/ui/calendar'
import { Input } from '@/components/ui/input'

// shallowRef, not ref. Dates are immutable value objects, and ref's deep
// unwrapping strips the private field the DateValue union is discriminated
// on, leaving a type that will not assign to Calendar's model.
const value = shallowRef<DateValue>(today(getLocalTimeZone()))

const formatter = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
const label = computed(() => formatter.format(value.value.toDate(getLocalTimeZone())))
</script>

<template>
  <div class="grid justify-items-center gap-3">
    <Input :model-value="label" readonly class="h-8 w-44 text-center text-sm" />
    <Calendar v-model="value" class="rounded-lg border p-3" />
  </div>
</template>
