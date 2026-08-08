<script setup lang="ts">
import { ref } from 'vue'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { Slider } from '@/components/ui/slider'

const density = ref([60])
</script>

<template>
  <div class="grid gap-4">
    <div class="grid gap-2">
      <div class="flex items-center justify-between">
        <Label>Feature density</Label>
        <span class="text-sm text-muted-foreground">{{ density[0] }}%</span>
      </div>
      <Slider v-model="density" :max="100" :step="5" />
    </div>

    <div class="grid gap-2">
      <div class="flex items-center justify-between text-sm">
        <span class="text-muted-foreground">Project setup</span>
        <span class="font-medium">72%</span>
      </div>
      <Progress :model-value="72" />
      <p class="text-xs text-muted-foreground">Progress is the read-only counterpart.</p>
    </div>
  </div>
</template>
