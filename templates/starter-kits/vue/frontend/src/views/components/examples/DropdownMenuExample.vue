<script setup lang="ts">
import { ref } from 'vue'
import { Calendar as CalendarIcon, ChevronsUpDown } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

const showSidebar = ref(true)
const showHints = ref(true)
const releaseTrack = ref('stable')
</script>

<template>
  <div class="flex flex-wrap gap-3">
    <DropdownMenu>
      <DropdownMenuTrigger as-child>
        <Button variant="outline">
          Project actions
          <ChevronsUpDown class="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent class="w-64">
        <DropdownMenuLabel>Project</DropdownMenuLabel>
        <DropdownMenuItem inset>Open dashboard</DropdownMenuItem>
        <DropdownMenuItem inset>View repository</DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuCheckboxItem v-model:checked="showSidebar">Show sidebar</DropdownMenuCheckboxItem>
        <DropdownMenuCheckboxItem v-model:checked="showHints">Show hints</DropdownMenuCheckboxItem>
        <DropdownMenuSeparator />
        <DropdownMenuLabel>Track</DropdownMenuLabel>
        <DropdownMenuRadioGroup v-model="releaseTrack">
          <DropdownMenuRadioItem value="stable">Stable</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="beta">Beta</DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>

    <Popover>
      <PopoverTrigger as-child>
        <Button variant="outline">
          Pick launch date
          <CalendarIcon class="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent class="w-auto p-0">
        <Calendar />
      </PopoverContent>
    </Popover>
  </div>
</template>
