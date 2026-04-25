<template>
  <section class="grid gap-6">
    <Card class="border-border/60">
      <CardHeader>
        <div class="flex flex-wrap items-center gap-2">
          <Badge>Components</Badge>
          <Badge variant="outline">Overlays</Badge>
        </div>
        <CardTitle class="text-3xl">Dialogs, drawers, and command patterns</CardTitle>
        <CardDescription class="max-w-3xl">
          Overlay primitives are most useful when they are shown as real workflows: invite dialogs, destructive confirms, drawers, inspectors, and command-driven actions.
        </CardDescription>
      </CardHeader>
    </Card>

    <div class="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Modal workflows</CardTitle>
          <CardDescription>Dialogs, sheets, and drawers are easier to judge when they look like product workflows rather than isolated mechanics.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-4">
          <div class="flex flex-wrap gap-3">
            <Dialog>
              <DialogTrigger as-child>
                <Button>Open invite dialog</Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Invite a teammate</DialogTitle>
                  <DialogDescription>
                    Use the same field patterns inside dialogs that the rest of the app uses so overlays do not feel like a separate system.
                  </DialogDescription>
                </DialogHeader>
                <div class="grid gap-4 py-2">
                  <div class="grid gap-2">
                    <Label>Email address</Label>
                    <Input placeholder="person@example.com" />
                  </div>
                  <div class="grid gap-2">
                    <Label>Role</Label>
                    <Select default-value="admin">
                      <SelectTrigger class="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="admin">Administrator</SelectItem>
                        <SelectItem value="editor">Editor</SelectItem>
                        <SelectItem value="viewer">Viewer</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <DialogFooter>
                  <DialogClose as-child>
                    <Button variant="outline">Cancel</Button>
                  </DialogClose>
                  <Button>Send invite</Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>

            <AlertDialog>
              <AlertDialogTrigger as-child>
                <Button variant="destructive">Archive project</Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Archive this project?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Reserve alert dialogs for destructive actions rather than routine confirmations.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction>Archive</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>

          <div class="flex flex-wrap gap-3">
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
                      <div class="rounded-xl border bg-muted/30 p-4">
                        <p class="text-sm font-medium">Deployment status</p>
                        <p class="mt-1 text-2xl font-semibold">Healthy</p>
                        <p class="mt-2 text-sm text-muted-foreground">No active incidents in the last 24 hours.</p>
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
                      <p class="text-sm font-medium">Recent events</p>
                      <div class="grid gap-2 rounded-xl border p-4">
                        <div v-for="event in sheetEvents" :key="event.title" class="flex items-start gap-3 rounded-lg border border-transparent px-2 py-2">
                          <div class="mt-0.5 size-2 rounded-full bg-primary" />
                          <div class="grid gap-1">
                            <p class="text-sm font-medium">{{ event.title }}</p>
                            <p class="text-sm text-muted-foreground">{{ event.description }}</p>
                          </div>
                        </div>
                      </div>
                    </div>

                    <div class="grid gap-3">
                      <p class="text-sm font-medium">Quick actions</p>
                      <div class="flex flex-wrap gap-2">
                        <Button class="flex-1 sm:flex-none">Open logs</Button>
                        <Button variant="outline" class="flex-1 sm:flex-none">Restart worker</Button>
                      </div>
                    </div>
                  </div>
                </ScrollArea>
              </SheetContent>
            </Sheet>

            <Button variant="outline" @click="openMobileDrawer">Open mobile drawer</Button>

            <Drawer :open="mobileDrawerOpen" @update:open="handleMobileDrawerOpenChange" :key="mobileDrawerKey">
              <DrawerContent class="overflow-hidden border-none bg-transparent shadow-none">
                <div class="mx-auto mb-4 w-[min(42rem,calc(100%-1rem))] overflow-hidden rounded-2xl border bg-background shadow-2xl">
                  <DrawerHeader class="px-6 pt-2">
                    <DrawerTitle>Mobile actions</DrawerTitle>
                    <DrawerDescription>
                      Drawers are often a better fit than centered modals for small-screen action clusters.
                    </DrawerDescription>
                  </DrawerHeader>
                  <div class="grid gap-4 px-6 pb-2">
                    <div class="rounded-xl border bg-muted/30 p-4">
                      <p class="text-sm font-medium">Queued deployment</p>
                      <p class="mt-1 text-2xl font-semibold">Version 24</p>
                      <p class="mt-2 text-sm text-muted-foreground">Promote now, inspect release notes, or dismiss this action sheet.</p>
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
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Command and notification patterns</CardTitle>
          <CardDescription>Command palette and toast feedback round out the overlay patterns users expect in an admin shell.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-5">
          <div class="rounded-xl border p-4">
            <p class="font-medium">Command palette</p>
            <p class="mt-1 text-sm text-muted-foreground">
              This uses the same command primitives that power the shell-level shortcut.
            </p>
            <Button class="mt-4" @click="commandOpen = true">Open command menu</Button>
          </div>

          <div class="rounded-xl border p-4">
            <p class="font-medium">Toast feedback</p>
            <p class="mt-1 text-sm text-muted-foreground">
              Toasts are useful for non-blocking confirmation after save, sync, or authentication actions.
            </p>
            <div class="mt-4 flex flex-wrap gap-2">
              <Button variant="outline" @click="notifyPreview">Show success toast</Button>
              <Button variant="secondary" @click="notifySignedOut">Show signed-out toast</Button>
            </div>
          </div>
        </CardContent>
      </Card>
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
import { toast } from 'vue-sonner'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
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
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Drawer, DrawerClose, DrawerContent, DrawerDescription, DrawerFooter, DrawerHeader, DrawerTitle, DrawerTrigger } from '@/components/ui/drawer'
import { Input } from '@/components/ui/input'
import { Item, ItemContent, ItemDescription, ItemTitle } from '@/components/ui/item'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'

const commandOpen = ref(false)
const mobileDrawerOpen = ref(false)
const mobileDrawerKey = ref(0)

const sheetEvents = [
  { title: 'Release promoted', description: 'Production rollout completed 12 minutes ago without errors.' },
  { title: 'Health check recovered', description: 'A brief API spike resolved automatically after autoscaling.' },
  { title: 'Audit trail captured', description: 'Configuration changes were recorded for the current deploy group.' },
]

function notifyPreview() {
  toast.success('Starter preview saved', {
    description: 'This toast is rendered by the local shadcn-vue sonner wrapper.',
  })
}

function notifySignedOut() {
  toast('Signed out', {
    description: 'Non-blocking notifications are ready for auth and settings flows.',
  })
}

function openMobileDrawer() {
  mobileDrawerOpen.value = true
}

function handleMobileDrawerOpenChange(value: boolean) {
  mobileDrawerOpen.value = value
  if (!value) {
    mobileDrawerKey.value += 1
  }
}
</script>
