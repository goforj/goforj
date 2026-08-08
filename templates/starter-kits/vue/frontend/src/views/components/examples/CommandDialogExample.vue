<script setup lang="ts">
import { ref } from 'vue'
import { Button } from '@/components/ui/button'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from '@/components/ui/command'

const open = ref(false)
</script>

<template>
  <Button @click="open = true">Open command menu</Button>

  <CommandDialog v-model:open="open">
    <CommandInput placeholder="Search commands..." />
    <CommandList>
      <CommandEmpty>No result found.</CommandEmpty>
      <CommandGroup heading="Navigate">
        <CommandItem value="dashboard">
          Open dashboard
          <CommandShortcut>G D</CommandShortcut>
        </CommandItem>
        <CommandItem value="components">
          Open components
          <CommandShortcut>G C</CommandShortcut>
        </CommandItem>
      </CommandGroup>
      <CommandSeparator />
      <CommandGroup heading="Account">
        <CommandItem value="profile">Edit profile</CommandItem>
        <CommandItem value="logout">Log out</CommandItem>
      </CommandGroup>
    </CommandList>
  </CommandDialog>
</template>
