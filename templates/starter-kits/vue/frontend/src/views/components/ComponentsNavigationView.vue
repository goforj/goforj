<template>
  <section class="grid gap-6">
    <PageHeader
      eyebrow="Components"
      section="Navigation"
      title="Navigation and layout patterns"
      description="Menus, action bars, resizable panes, and scroll surfaces shown the way they appear in product UI rather than a flat component catalog."
    />

    <Specimen
      name="ButtonGroup"
      description="Joins related buttons into a single segmented control. Use for toolbars, pagination, and split actions."
      :also="['Button']"
      :source="ButtonGroupExampleSource"
    >
        <ButtonGroupExample />
    </Specimen>

    <div class="grid gap-6 xl:grid-cols-2">
      <Specimen
        name="DropdownMenu"
        description="A trigger-anchored menu supporting labels, separators, checkbox items, and radio groups."
        :also="['Button', 'Popover', 'Calendar']"
        :source="DropdownMenuExampleSource"
      >
        <DropdownMenuExample />
      </Specimen>

      <Specimen
        name="Collapsible"
        description="One hidden section. Reach for this instead of an accordion when there is only a single disclosure."
        :also="['Button']"
        :source="CollapsibleExampleSource"
      >
        <CollapsibleExample />
      </Specimen>

      <Specimen
        name="ContextMenu"
        description="The same menu vocabulary, opened by right click on a surface rather than a button."
        :source="ContextMenuExampleSource"
      >
        <ContextMenuExample />
      </Specimen>

      <Specimen
        name="Accordion"
        description="A stack of disclosures where opening one can close the others."
        :source="AccordionExampleSource"
      >
        <AccordionExample />
      </Specimen>
    </div>

    <Specimen
      name="ResizablePanelGroup"
      description="Draggable split panes for dashboards, inspectors, and log surfaces."
      :also="['ScrollArea', 'Carousel', 'Item']"
      :source="ResizablePanelGroupExampleSource"
    >
        <ResizablePanelGroupExample />
    </Specimen>
  </section>
</template>

<script setup lang="ts">
import ButtonGroupExample from './examples/ButtonGroupExample.vue'
import ButtonGroupExampleSource from './examples/ButtonGroupExample.vue?raw'
import DropdownMenuExample from './examples/DropdownMenuExample.vue'
import DropdownMenuExampleSource from './examples/DropdownMenuExample.vue?raw'
import CollapsibleExample from './examples/CollapsibleExample.vue'
import CollapsibleExampleSource from './examples/CollapsibleExample.vue?raw'
import ContextMenuExample from './examples/ContextMenuExample.vue'
import ContextMenuExampleSource from './examples/ContextMenuExample.vue?raw'
import AccordionExample from './examples/AccordionExample.vue'
import AccordionExampleSource from './examples/AccordionExample.vue?raw'
import ResizablePanelGroupExample from './examples/ResizablePanelGroupExample.vue'
import ResizablePanelGroupExampleSource from './examples/ResizablePanelGroupExample.vue?raw'
import PageHeader from '@/components/PageHeader.vue'
import { Specimen } from '@/components/showcase'
</script>
