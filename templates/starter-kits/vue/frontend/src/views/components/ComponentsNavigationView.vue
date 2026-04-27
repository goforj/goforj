<template>
  <section class="grid gap-6">
    <Card class="border-border/60">
      <CardHeader>
        <div class="flex flex-wrap items-center gap-2">
          <Badge>Components</Badge>
          <Badge variant="outline">Navigation</Badge>
        </div>
        <CardTitle class="text-3xl">Navigation and layout patterns</CardTitle>
        <CardDescription class="max-w-3xl">
          Menus, action bars, resizable panes, and scroll surfaces shown the way they appear in product UI rather than a flat component catalog.
        </CardDescription>
      </CardHeader>
    </Card>

    <div class="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Toolbar and action clusters</CardTitle>
          <CardDescription>Action bars, pagination controls, and utility toggles are easier to evaluate when they are grouped the way a product would use them.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-5">
          <div class="flex flex-wrap items-center justify-between gap-4 rounded-xl border p-4">
            <ButtonGroup>
              <Button variant="outline" size="icon" aria-label="Go back">
                <ArrowLeft class="size-4" />
              </Button>
            </ButtonGroup>

            <div class="flex flex-wrap items-center gap-3">
              <ButtonGroup>
                <Button variant="outline" size="sm">Archive</Button>
                <Button variant="outline" size="sm">Report</Button>
              </ButtonGroup>

              <ButtonGroup>
                <Button variant="outline" size="sm">Snooze</Button>
                <Button variant="outline" size="sm" class="px-2.5" aria-label="More options">
                  <Ellipsis class="size-4" />
                </Button>
              </ButtonGroup>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-4 rounded-xl border p-4">
            <ButtonGroup>
              <Button variant="outline" size="sm">1</Button>
              <Button variant="outline" size="sm">2</Button>
              <Button variant="outline" size="sm">3</Button>
            </ButtonGroup>

            <div class="flex flex-wrap items-center gap-3">
              <ButtonGroup>
                <Button variant="outline" size="sm" class="px-2.5" aria-label="Previous page">
                  <ArrowLeft class="size-4" />
                </Button>
                <Button variant="outline" size="sm" class="px-2.5" aria-label="Next page">
                  <ArrowRight class="size-4" />
                </Button>
              </ButtonGroup>

              <ButtonGroup>
                <Button variant="outline" size="sm">
                  <Bot class="size-4" />
                  Copilot
                </Button>
                <Button variant="outline" size="sm" class="px-2.5" aria-label="Copilot options">
                  <ChevronDown class="size-4" />
                </Button>
              </ButtonGroup>
            </div>
          </div>

          <label class="flex items-center gap-3 rounded-xl border border-primary/20 bg-primary/8 px-4 py-3">
            <Checkbox v-model="agreedToTerms" />
            <span class="text-sm font-medium">I agree to the terms and conditions</span>
          </label>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Menus and contextual actions</CardTitle>
          <CardDescription>Dropdowns, context menus, and disclosures should feel like product tools, not isolated demos.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-5">
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

          <ContextMenu>
            <ContextMenuTrigger class="flex min-h-40 items-center justify-center rounded-xl border border-dashed bg-muted/30 p-6 text-sm text-muted-foreground">
              Right click this deployment surface
            </ContextMenuTrigger>
            <ContextMenuContent class="w-60">
              <ContextMenuLabel>Deployment</ContextMenuLabel>
              <ContextMenuItem inset>Open logs</ContextMenuItem>
              <ContextMenuItem inset>Restart worker</ContextMenuItem>
              <ContextMenuSeparator />
              <ContextMenuCheckboxItem v-model:checked="showHints">Show hints</ContextMenuCheckboxItem>
              <ContextMenuSeparator />
              <ContextMenuLabel>Region</ContextMenuLabel>
              <ContextMenuRadioGroup v-model="nativeRegion">
                <ContextMenuRadioItem value="iad">US East</ContextMenuRadioItem>
                <ContextMenuRadioItem value="sfo">US West</ContextMenuRadioItem>
                <ContextMenuRadioItem value="fra">Europe</ContextMenuRadioItem>
              </ContextMenuRadioGroup>
            </ContextMenuContent>
          </ContextMenu>

          <Collapsible class="rounded-xl border px-4">
            <div class="flex items-center justify-between py-4">
              <div>
                <p class="font-medium">Need a smaller disclosure?</p>
                <p class="text-sm text-muted-foreground">Use a collapsible block when you need one hidden section rather than a full accordion stack.</p>
              </div>
              <CollapsibleTrigger as-child>
                <Button variant="ghost" size="sm">Toggle</Button>
              </CollapsibleTrigger>
            </div>
            <CollapsibleContent class="pb-4 text-sm text-muted-foreground">
              This pattern works well for advanced settings, environment details, or inline help that should stay secondary to the main workflow.
            </CollapsibleContent>
          </Collapsible>

          <Accordion type="single" collapsible class="rounded-xl border px-4">
            <AccordionItem value="faq-1">
              <AccordionTrigger>When should a page keep the examples?</AccordionTrigger>
              <AccordionContent>
                Keep only the examples that act as a real scaffold for your team. Delete the rest once you have product-specific replacements.
              </AccordionContent>
            </AccordionItem>
            <AccordionItem value="faq-2">
              <AccordionTrigger>What about the sidebar family?</AccordionTrigger>
              <AccordionContent>
                The generated app shell is already a sidebar example, so this page focuses on the supporting navigation families around it.
              </AccordionContent>
            </AccordionItem>
          </Accordion>
        </CardContent>
      </Card>

      <Card class="xl:col-span-2">
        <CardHeader>
          <CardTitle>Scrollable and resizable work surfaces</CardTitle>
          <CardDescription>Dashboards often need split panes and bounded scroll containers for logs, inspectors, and operational activity.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-5">
          <ResizablePanelGroup direction="horizontal" class="min-h-[430px] rounded-xl border">
            <ResizablePanel :default-size="46" :min-size="38" class="p-4">
              <p class="mb-3 font-medium">Activity feed</p>
              <ScrollArea class="h-[350px] rounded-lg border">
                <div class="grid gap-2 p-3">
                  <Item v-for="entry in activityFeed" :key="entry.title" size="sm">
                    <ItemMedia variant="icon">
                      <Activity class="size-4" />
                    </ItemMedia>
                    <ItemContent>
                      <ItemTitle>{{ entry.title }}</ItemTitle>
                      <ItemDescription>{{ entry.description }}</ItemDescription>
                    </ItemContent>
                  </Item>
                </div>
              </ScrollArea>
            </ResizablePanel>
            <ResizableHandle with-handle />
            <ResizablePanel :default-size="54" :min-size="34" class="p-4">
              <p class="mb-3 font-medium">Content carousel</p>
              <Carousel class="w-full">
                <CarouselContent>
                  <CarouselItem v-for="slide in carouselSlides" :key="slide.title" class="md:basis-1/2">
                    <Card class="h-full">
                      <CardHeader>
                        <CardTitle class="text-lg">{{ slide.title }}</CardTitle>
                        <CardDescription>{{ slide.description }}</CardDescription>
                      </CardHeader>
                      <CardContent>
                        <div class="rounded-xl border bg-muted/30 p-6 text-sm text-muted-foreground">
                          {{ slide.copy }}
                        </div>
                      </CardContent>
                    </Card>
                  </CarouselItem>
                </CarouselContent>
                <CarouselPrevious />
                <CarouselNext />
              </Carousel>
            </ResizablePanel>
          </ResizablePanelGroup>
        </CardContent>
      </Card>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Activity, ArrowLeft, ArrowRight, Bot, Calendar as CalendarIcon, ChevronDown, ChevronsUpDown, Ellipsis } from 'lucide-vue-next'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ButtonGroup } from '@/components/ui/button-group'
import { Calendar } from '@/components/ui/calendar'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Carousel, CarouselContent, CarouselItem, CarouselNext, CarouselPrevious } from '@/components/ui/carousel'
import { Checkbox } from '@/components/ui/checkbox'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import {
  ContextMenu,
  ContextMenuCheckboxItem,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuRadioGroup,
  ContextMenuRadioItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
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
import { Item, ItemContent, ItemDescription, ItemMedia, ItemTitle } from '@/components/ui/item'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import { ScrollArea } from '@/components/ui/scroll-area'

const showSidebar = ref(true)
const showHints = ref(true)
const releaseTrack = ref('stable')
const nativeRegion = ref('iad')
const agreedToTerms = ref(true)

const activityFeed = [
  { title: 'Starter rendered', description: 'Vue shell copied into the generated frontend folder.' },
  { title: 'User authenticated', description: 'The navbar and sidebar can consume `/api/v1/auth/me`.' },
  { title: 'Theme detected', description: 'Dark mode follows system preference by default.' },
  { title: 'Components installed', description: 'The local `src/components/ui` tree is ready to import.' },
]

const carouselSlides = [
  { title: 'Dashboard baseline', description: 'Cards, charts, tables, and app navigation.', copy: 'Start here when you want an internal admin UI with a fast path to production.' },
  { title: 'Auth surface', description: 'Login, logout, me bootstrap, and user shell.', copy: 'Connect these screens to generated auth endpoints and replace the placeholders with product copy.' },
  { title: 'Reference gallery', description: 'Menus, overlays, and form controls.', copy: 'Split routes like this keep the gallery browseable without one giant monolithic page.' },
]
</script>
