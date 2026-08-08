<script setup lang="ts">
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion'
</script>

<template>
  <Accordion type="single" collapsible class="rounded-lg border px-4">
    <AccordionItem value="faq-1">
      <AccordionTrigger>When should a page keep the examples?</AccordionTrigger>
      <AccordionContent>
        Keep only the examples that act as a real scaffold for your team. Delete the
        rest once you have product-specific replacements.
      </AccordionContent>
    </AccordionItem>
    <AccordionItem value="faq-2">
      <AccordionTrigger>What about the sidebar family?</AccordionTrigger>
      <AccordionContent>
        The generated app shell is already a sidebar example, so this page focuses on
        the supporting navigation families around it.
      </AccordionContent>
    </AccordionItem>
  </Accordion>
</template>
