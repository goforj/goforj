<script setup lang="ts">
import { computed, ref } from 'vue'
import { getLocalTimeZone, today } from '@internationalized/date'
import { Calendar } from '@/components/ui/calendar'
import { Input } from '@/components/ui/input'

const value = ref(today(getLocalTimeZone()))

const formatter = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
const label = computed(() => formatter.format(value.value.toDate(getLocalTimeZone())))
</script>

<template>
  <div class="grid justify-items-center gap-3">
    <Input :model-value="label" readonly class="h-8 w-44 text-center text-sm" />
    <Calendar v-model="value" class="rounded-lg border p-3" />
  </div>
</template>
