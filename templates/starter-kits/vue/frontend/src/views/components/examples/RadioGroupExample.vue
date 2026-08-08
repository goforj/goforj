<script setup lang="ts">
import { ref } from 'vue'
import { FieldDescription, FieldLegend, FieldSet } from '@/components/ui/field'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'

const target = ref('containers')
</script>

<template>
  <FieldSet class="grid gap-3">
    <FieldLegend>Deployment target</FieldLegend>
    <FieldDescription>Radio cards give each option room for a description.</FieldDescription>

    <RadioGroup v-model="target" class="grid gap-2">
      <label class="flex items-start gap-3 rounded-lg border p-4 transition has-[:checked]:border-primary/30 has-[:checked]:bg-primary/10">
        <div class="grid flex-1 gap-1">
          <span class="font-medium">Containers</span>
          <span class="text-sm text-muted-foreground">Run from the local compose stack. This is the default.</span>
        </div>
        <RadioGroupItem value="containers" />
      </label>
      <label class="flex items-start gap-3 rounded-lg border p-4 transition has-[:checked]:border-primary/30 has-[:checked]:bg-primary/10">
        <div class="grid flex-1 gap-1">
          <span class="font-medium">Virtual machine</span>
          <span class="text-sm text-muted-foreground">Target a provisioned VM instead of the container runtime.</span>
        </div>
        <RadioGroupItem value="vm" />
      </label>
    </RadioGroup>
  </FieldSet>
</template>
