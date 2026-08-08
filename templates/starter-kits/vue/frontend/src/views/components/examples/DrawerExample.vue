<script setup lang="ts">
import { ref } from 'vue'
import { Button } from '@/components/ui/button'
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from '@/components/ui/drawer'

const open = ref(false)
</script>

<template>
  <Drawer v-model:open="open">
    <DrawerTrigger as-child>
      <Button variant="outline">Open mobile drawer</Button>
    </DrawerTrigger>

    <DrawerContent>
      <div class="mx-auto w-full max-w-md">
        <DrawerHeader>
          <DrawerTitle>Mobile actions</DrawerTitle>
          <DrawerDescription>
            Drawers often fit small-screen action clusters better than a centered modal.
          </DrawerDescription>
        </DrawerHeader>

        <div class="grid gap-3 px-4">
          <Button class="w-full justify-start">Create deployment</Button>
          <Button variant="outline" class="w-full justify-start">View docs</Button>
          <Button variant="ghost" class="w-full justify-start">Review release notes</Button>
        </div>

        <DrawerFooter>
          <DrawerClose as-child>
            <Button variant="outline">Done</Button>
          </DrawerClose>
        </DrawerFooter>
      </div>
    </DrawerContent>
  </Drawer>
</template>
