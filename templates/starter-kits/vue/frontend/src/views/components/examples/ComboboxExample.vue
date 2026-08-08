<script setup lang="ts">
import { ref } from 'vue'
import { Check, ChevronsUpDown } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxAnchor,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxItemIndicator,
  ComboboxList,
  ComboboxTrigger,
  ComboboxViewport,
} from '@/components/ui/combobox'

const selected = ref('Vue')
const frameworks = ['Vue', 'Nuxt', 'React', 'Svelte', 'Laravel Blade']
</script>

<template>
  <Combobox v-model="selected">
    <ComboboxAnchor class="w-full">
      <div class="relative">
        <ComboboxInput class="pr-9" placeholder="Search framework..." />
        <ComboboxTrigger as-child>
          <Button variant="ghost" size="icon" class="absolute right-1 top-1 size-7">
            <ChevronsUpDown class="size-4" />
          </Button>
        </ComboboxTrigger>
      </div>
    </ComboboxAnchor>
    <ComboboxList class="w-[var(--reka-combobox-trigger-width)]">
      <ComboboxEmpty>No framework found.</ComboboxEmpty>
      <ComboboxViewport>
        <ComboboxGroup>
          <ComboboxItem v-for="framework in frameworks" :key="framework" :value="framework">
            {{ framework }}
            <ComboboxItemIndicator class="ml-auto">
              <Check class="size-4" />
            </ComboboxItemIndicator>
          </ComboboxItem>
        </ComboboxGroup>
      </ComboboxViewport>
    </ComboboxList>
  </Combobox>
</template>
