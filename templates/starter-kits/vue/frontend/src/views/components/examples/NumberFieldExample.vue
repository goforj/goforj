<script setup lang="ts">
import { ref } from 'vue'
import {
  NumberField,
  NumberFieldContent,
  NumberFieldDecrement,
  NumberFieldIncrement,
  NumberFieldInput,
} from '@/components/ui/number-field'
import { Label } from '@/components/ui/label'

const seats = ref(8)
</script>

<template>
  <div class="grid gap-2">
    <Label>Seat count</Label>
    <NumberField v-model="seats" :min="1" :max="25" class="w-36">
      <NumberFieldContent>
        <NumberFieldDecrement />
        <NumberFieldInput />
        <NumberFieldIncrement />
      </NumberFieldContent>
    </NumberField>
    <p class="text-xs text-muted-foreground">Clamped between 1 and 25.</p>
  </div>
</template>
