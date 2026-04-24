<script setup lang="ts">
import type { DialogRootEmits, DialogRootProps } from "reka-ui"
import { DialogContent, DialogOverlay, DialogPortal, useForwardPropsEmits } from "reka-ui"
import { Dialog, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import Command from "./Command.vue"

const props = withDefaults(defineProps<DialogRootProps & {
  title?: string
  description?: string
}>(), {
  title: "Command Menu",
  description: "Search for a command to run...",
})
const emits = defineEmits<DialogRootEmits>()

const forwarded = useForwardPropsEmits(props, emits)
</script>

<template>
  <Dialog v-slot="slotProps" v-bind="forwarded">
    <DialogPortal>
      <DialogOverlay
        class="fixed inset-0 z-50 bg-black/80 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
      />
      <div class="pointer-events-none fixed inset-0 z-50 flex items-start justify-center p-4 pt-[18vh] sm:items-center sm:pt-4">
        <DialogContent class="pointer-events-auto relative z-50 w-full max-w-2xl overflow-hidden rounded-lg border bg-background p-0 shadow-lg duration-200 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95">
          <DialogHeader class="sr-only">
            <DialogTitle>{{ title }}</DialogTitle>
            <DialogDescription>{{ description }}</DialogDescription>
          </DialogHeader>
          <Command>
            <slot v-bind="slotProps" />
          </Command>
        </DialogContent>
      </div>
    </DialogPortal>
  </Dialog>
</template>
