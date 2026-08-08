<script setup lang="ts">
import { ref } from 'vue'
import { CircleCheckBig } from '@lucide/vue'
import {
  Stepper,
  StepperDescription,
  StepperIndicator,
  StepperItem,
  StepperSeparator,
  StepperTitle,
  StepperTrigger,
} from '@/components/ui/stepper'

const active = ref(2)

const steps = [
  { step: 1, title: 'Choose the shell', description: 'Sidebar, navbar, and route structure.' },
  { step: 2, title: 'Connect auth', description: 'Sign-in, session bootstrap, and logout.' },
  { step: 3, title: 'Ship the first workflow', description: 'Replace examples with product behaviour.' },
]
</script>

<template>
  <Stepper v-model="active" class="flex-col gap-2">
    <StepperItem v-for="step in steps" :key="step.step" :step="step.step" class="items-start">
      <div class="grid w-full gap-2">
        <StepperTrigger as-child>
          <button
            class="grid w-full grid-cols-[auto_1fr] items-start gap-3 rounded-lg border border-transparent p-3 text-left transition group-data-[state=active]:border-primary/25 group-data-[state=active]:bg-primary/10 group-data-[state=completed]:bg-muted/30 group-data-[state=inactive]:opacity-65"
          >
            <StepperIndicator class="mt-0.5 group-data-[state=inactive]:bg-transparent group-data-[state=inactive]:text-muted-foreground/60">
              <CircleCheckBig v-if="active > step.step" class="size-4" />
              <span v-else>{{ step.step }}</span>
            </StepperIndicator>
            <div class="grid gap-1">
              <StepperTitle class="whitespace-normal leading-tight group-data-[state=inactive]:text-muted-foreground">
                {{ step.title }}
              </StepperTitle>
              <StepperDescription class="text-sm group-data-[state=inactive]:text-muted-foreground/80">
                {{ step.description }}
              </StepperDescription>
            </div>
          </button>
        </StepperTrigger>
        <StepperSeparator v-if="step.step !== steps.length" class="ml-4 h-8 w-px group-data-[state=inactive]:bg-muted/40" />
      </div>
    </StepperItem>
  </Stepper>
</template>
