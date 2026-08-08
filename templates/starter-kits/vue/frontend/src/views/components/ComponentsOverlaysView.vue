<template>
  <section class="grid gap-6">
    <PageHeader
      eyebrow="Components"
      section="Overlays"
      title="Dialogs, drawers, and command patterns"
      description="Overlay primitives are most useful when they are shown as real workflows: invite dialogs, destructive confirms, drawers, inspectors, and command-driven actions."
    />

    <div class="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
      <Specimen
        name="Dialog"
        description="A centered modal for focused tasks. Use the same field patterns inside it that the rest of the app uses."
        :also="['Input', 'Select', 'Label', 'Button']"
        content-class="justify-items-start"
        :source="DialogExampleSource"
      >
        <DialogExample />
      </Specimen>

      <Specimen
        name="AlertDialog"
        description="A confirm that cannot be dismissed by clicking away. Reserve it for destructive actions rather than routine confirmations."
        :also="['Button']"
        content-class="justify-items-start"
        :source="AlertDialogExampleSource"
      >
        <AlertDialogExample />
      </Specimen>

      <Specimen
        name="Sheet"
        description="An edge-anchored panel for detail views and side-task editors that should not interrupt the page flow."
        :also="['ScrollArea', 'Item', 'Button']"
        content-class="justify-items-start"
        :source="SheetExampleSource"
      >
        <SheetExample />
      </Specimen>

      <Specimen
        name="Sonner"
        description="Non-blocking confirmation after save, sync, or authentication actions."
        :also="['Button']"
        content-class="justify-items-start"
        :source="SonnerExampleSource"
      >
        <SonnerExample />
      </Specimen>

      <Specimen
        name="Drawer"
        description="A bottom-anchored action sheet. Often a better fit than a centered modal for small-screen action clusters."
        :also="['Button']"
        content-class="justify-items-start"
        :source="DrawerExampleSource"
      >
        <DrawerExample />
      </Specimen>

      <Specimen
        name="CommandDialog"
        description="The searchable palette behind the shell shortcut. Same primitives as the sidebar command menu."
        :also="['Button']"
        content-class="justify-items-start"
        :source="CommandDialogExampleSource"
      >
        <CommandDialogExample />
      </Specimen>
    </div>
  </section>
</template>

<script setup lang="ts">
import DialogExample from './examples/DialogExample.vue'
import DialogExampleSource from './examples/DialogExample.vue?raw'
import AlertDialogExample from './examples/AlertDialogExample.vue'
import AlertDialogExampleSource from './examples/AlertDialogExample.vue?raw'
import SonnerExample from './examples/SonnerExample.vue'
import SonnerExampleSource from './examples/SonnerExample.vue?raw'
import SheetExample from './examples/SheetExample.vue'
import SheetExampleSource from './examples/SheetExample.vue?raw'
import DrawerExample from './examples/DrawerExample.vue'
import DrawerExampleSource from './examples/DrawerExample.vue?raw'
import CommandDialogExample from './examples/CommandDialogExample.vue'
import CommandDialogExampleSource from './examples/CommandDialogExample.vue?raw'
import PageHeader from '@/components/PageHeader.vue'
import { Specimen } from '@/components/showcase'
</script>
