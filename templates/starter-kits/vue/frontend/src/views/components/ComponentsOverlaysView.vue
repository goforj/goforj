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
      >
        <Sheet>
          <SheetTrigger as-child>
            <Button variant="outline">Open side sheet</Button>
          </SheetTrigger>
          <SheetContent class="min-h-0 w-full gap-6 overflow-hidden p-6 pt-10 sm:max-w-md">
            <SheetHeader class="gap-2 border-b pb-5 pl-0 pr-10 pt-0">
              <SheetTitle>Inspector panel</SheetTitle>
              <SheetDescription class="text-base leading-relaxed">
                Sheets work well for detail views and side-task editors that should not interrupt the main page flow.
              </SheetDescription>
            </SheetHeader>
            <ScrollArea class="min-h-0 flex-1 -mr-2 pr-2">
              <div class="grid gap-6 pb-6">
                <div class="grid gap-3">
                  <div class="grid gap-1 rounded-lg bg-muted/40 p-4">
                    <p class="text-sm font-medium leading-none">Deployment status</p>
                    <p class="text-2xl font-semibold tracking-tight">Healthy</p>
                    <p class="text-sm text-muted-foreground">No active incidents in the last 24 hours.</p>
                  </div>

                  <div class="grid gap-3 sm:grid-cols-2">
                    <Item variant="outline" size="sm">
                      <ItemContent>
                        <ItemTitle>Request latency</ItemTitle>
                        <ItemDescription>Median 84ms over the last hour.</ItemDescription>
                      </ItemContent>
                    </Item>
                    <Item variant="outline" size="sm">
                      <ItemContent>
                        <ItemTitle>Error budget</ItemTitle>
                        <ItemDescription>99.95% availability target remains healthy.</ItemDescription>
                      </ItemContent>
                    </Item>
                  </div>
                </div>

                <div class="grid gap-3">
                  <p class="text-sm font-medium leading-none">Recent events</p>
                  <div class="grid gap-2 rounded-lg bg-muted/40 p-4">
                    <div v-for="event in sheetEvents" :key="event.title" class="flex items-start gap-3 px-2 py-2">
                      <div class="mt-1.5 size-2 shrink-0 rounded-full bg-primary" />
                      <div class="grid gap-1">
                        <p class="text-sm font-medium leading-none">{{ event.title }}</p>
                        <p class="text-sm text-muted-foreground">{{ event.description }}</p>
                      </div>
                    </div>
                  </div>
                </div>

                <div class="grid gap-3">
                  <p class="text-sm font-medium leading-none">Quick actions</p>
                  <div class="flex flex-wrap gap-2">
                    <Button class="flex-1 sm:flex-none">Open logs</Button>
                    <Button variant="outline" class="flex-1 sm:flex-none">Restart worker</Button>
                  </div>
                </div>
              </div>
            </ScrollArea>
          </SheetContent>
        </Sheet>
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
      >
        <Button variant="outline" @click="openMobileDrawer">Open mobile drawer</Button>

        <Drawer v-model:open="mobileDrawerOpen">
          <DrawerContent class="overflow-hidden border-none bg-transparent shadow-none">
            <div class="mx-auto mb-4 w-[min(42rem,calc(100%-1rem))] overflow-hidden rounded-xl border bg-background shadow-lg">
              <DrawerHeader class="px-6 pt-2">
                <DrawerTitle>Mobile actions</DrawerTitle>
                <DrawerDescription>
                  Drawers are often a better fit than centered modals for small-screen action clusters.
                </DrawerDescription>
              </DrawerHeader>
              <div class="grid gap-4 px-6 pb-2">
                <div class="grid gap-1 rounded-lg bg-muted/40 p-4">
                  <p class="text-sm font-medium leading-none">Queued deployment</p>
                  <p class="text-2xl font-semibold tracking-tight">Version 24</p>
                  <p class="text-sm text-muted-foreground">Promote now, inspect release notes, or dismiss this action sheet.</p>
                </div>
                <div class="grid gap-3">
                  <Button class="w-full justify-start">Create deployment</Button>
                  <Button variant="outline" class="w-full justify-start">View docs</Button>
                  <Button variant="ghost" class="w-full justify-start">Review release notes</Button>
                </div>
              </div>
              <DrawerFooter class="border-t px-6 pb-6 pt-4">
                <DrawerClose as-child>
                  <Button variant="outline">Done</Button>
                </DrawerClose>
              </DrawerFooter>
            </div>
          </DrawerContent>
        </Drawer>
      </Specimen>

      <Specimen
        name="CommandDialog"
        description="The searchable palette behind the shell shortcut. Same primitives as the sidebar command menu."
        :also="['Button']"
        content-class="justify-items-start"
      >
        <Button @click="commandOpen = true">Open command menu</Button>
      </Specimen>
    </div>

    <CommandDialog v-model:open="commandOpen">
      <CommandInput placeholder="Search starter commands..." />
      <CommandList>
        <CommandEmpty>No result found.</CommandEmpty>
        <CommandGroup heading="Starter">
          <CommandItem value="open-dashboard">
            Open dashboard
            <CommandShortcut>G D</CommandShortcut>
          </CommandItem>
          <CommandItem value="open-components">
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
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import DialogExample from './examples/DialogExample.vue'
import DialogExampleSource from './examples/DialogExample.vue?raw'
import AlertDialogExample from './examples/AlertDialogExample.vue'
import AlertDialogExampleSource from './examples/AlertDialogExample.vue?raw'
import SonnerExample from './examples/SonnerExample.vue'
import SonnerExampleSource from './examples/SonnerExample.vue?raw'
import PageHeader from '@/components/PageHeader.vue'
import { Specimen } from '@/components/showcase'
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
import { Drawer, DrawerClose, DrawerContent, DrawerDescription, DrawerFooter, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { Item, ItemContent, ItemDescription, ItemTitle } from '@/components/ui/item'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'

const commandOpen = ref(false)
const mobileDrawerOpen = ref(false)

const sheetEvents = [
  { title: 'Release promoted', description: 'Production rollout completed 12 minutes ago without errors.' },
  { title: 'Health check recovered', description: 'A brief API spike resolved automatically after autoscaling.' },
  { title: 'Audit trail captured', description: 'Configuration changes were recorded for the current deploy group.' },
]



function openMobileDrawer() {
  mobileDrawerOpen.value = true
}
</script>
