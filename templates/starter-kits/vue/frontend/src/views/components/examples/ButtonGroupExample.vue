<script setup lang="ts">
import { ArrowLeft, ArrowRight, Bot, ChevronDown, Ellipsis } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { ButtonGroup } from '@/components/ui/button-group'
</script>

<template>
  <div class="grid gap-4">
    <div class="flex flex-wrap items-center gap-3">
      <ButtonGroup>
        <Button variant="outline" size="icon" aria-label="Go back">
          <ArrowLeft class="size-4" />
        </Button>
      </ButtonGroup>

      <ButtonGroup>
        <Button variant="outline" size="sm">Archive</Button>
        <Button variant="outline" size="sm">Report</Button>
      </ButtonGroup>

      <ButtonGroup>
        <Button variant="outline" size="sm">Snooze</Button>
        <Button variant="outline" size="sm" class="px-2.5" aria-label="More options">
          <Ellipsis class="size-4" />
        </Button>
      </ButtonGroup>
    </div>

    <div class="flex flex-wrap items-center gap-3">
      <ButtonGroup>
        <Button variant="outline" size="sm">1</Button>
        <Button variant="outline" size="sm">2</Button>
        <Button variant="outline" size="sm">3</Button>
      </ButtonGroup>

      <ButtonGroup>
        <Button variant="outline" size="sm" class="px-2.5" aria-label="Previous page">
          <ArrowLeft class="size-4" />
        </Button>
        <Button variant="outline" size="sm" class="px-2.5" aria-label="Next page">
          <ArrowRight class="size-4" />
        </Button>
      </ButtonGroup>

      <ButtonGroup>
        <Button variant="outline" size="sm">
          <Bot class="size-4" />
          Copilot
        </Button>
        <Button variant="outline" size="sm" class="px-2.5" aria-label="Copilot options">
          <ChevronDown class="size-4" />
        </Button>
      </ButtonGroup>
    </div>
  </div>
</template>
