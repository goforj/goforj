<script setup lang="ts">
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
</script>

<template>
  <Collapsible class="rounded-lg border px-4">
    <div class="flex items-center justify-between gap-4 py-4">
      <div class="grid gap-0.5">
        <p class="text-sm font-medium leading-none">Need a smaller disclosure?</p>
        <p class="text-sm text-muted-foreground">
          Use a collapsible when you need one hidden section rather than a stack.
        </p>
      </div>
      <CollapsibleTrigger as-child>
        <Button variant="ghost" size="sm">Toggle</Button>
      </CollapsibleTrigger>
    </div>
    <CollapsibleContent class="pb-4 text-sm text-muted-foreground">
      This suits advanced settings, environment details, or inline help that should
      stay secondary to the main workflow.
    </CollapsibleContent>
  </Collapsible>
</template>
