<script setup lang="ts">
import { ArrowUp, Check, Info, Plus, Search, Star } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
  InputGroupText,
  InputGroupTextarea,
} from '@/components/ui/input-group'
import { Label } from '@/components/ui/label'
</script>

<template>
  <div class="grid gap-4 md:grid-cols-2">
    <div class="grid gap-2 md:col-span-2">
      <Label>Command bar</Label>
      <InputGroup class="[--radius:9999px]">
        <InputGroupAddon>
          <Search class="size-4" />
        </InputGroupAddon>
        <InputGroupInput placeholder="Search docs, commands, and project resources..." />
        <InputGroupAddon align="inline-end">
          <InputGroupText>Ctrl K</InputGroupText>
        </InputGroupAddon>
      </InputGroup>
    </div>

    <div class="grid gap-2">
      <Label>Search input</Label>
      <InputGroup>
        <InputGroupAddon>
          <Search class="size-4" />
        </InputGroupAddon>
        <InputGroupInput placeholder="Search..." />
        <InputGroupAddon align="inline-end">12 results</InputGroupAddon>
      </InputGroup>
    </div>

    <div class="grid gap-2">
      <Label>URL input</Label>
      <InputGroup>
        <InputGroupAddon>
          <InputGroupText>https://</InputGroupText>
        </InputGroupAddon>
        <InputGroupInput placeholder="example.com" class="!pl-1" />
        <InputGroupAddon align="inline-end">
          <InputGroupButton variant="ghost" aria-label="Info">
            <Info class="size-4" />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </div>

    <div class="grid gap-2 md:col-span-2">
      <Label>Message composer</Label>
      <InputGroup class="h-auto">
        <InputGroupTextarea placeholder="Ask, Search or Chat..." class="min-h-24" />
        <InputGroupAddon align="block-end" class="w-full justify-start gap-2 border-t pt-2">
          <Button variant="outline" size="icon" class="rounded-full" aria-label="Add attachment">
            <Plus class="size-4" />
          </Button>
          <Button variant="ghost" size="sm" class="rounded-full px-2">Auto</Button>
          <span class="ml-auto text-sm text-muted-foreground">52% used</span>
          <Button size="icon" class="rounded-full" :disabled="true" aria-label="Send">
            <ArrowUp class="size-4" />
          </Button>
        </InputGroupAddon>
      </InputGroup>
    </div>

    <div class="grid gap-4 rounded-lg bg-muted/40 p-4 md:col-span-2">
    <div class="grid gap-1">
      <p class="text-sm font-medium leading-none">Assistant handoff</p>
      <p class="text-sm text-muted-foreground">Compose richer prompts with source scope, mode selection, and a named destination before sending.</p>
    </div>
      <InputGroup class="h-auto">
        <InputGroupTextarea placeholder="Describe what the assistant should investigate, change, or summarize." class="min-h-28" />
        <InputGroupAddon align="block-start" class="w-full justify-start gap-2 border-b pb-2">
          <Button variant="outline" size="sm" class="rounded-full px-3">@ Add context</Button>
          <Badge variant="outline">frontend</Badge>
          <Badge variant="outline">billing</Badge>
          <Badge variant="outline">starter-kit</Badge>
        </InputGroupAddon>
        <InputGroupAddon align="block-end" class="w-full justify-start gap-2 border-t pt-2">
          <Button variant="ghost" size="sm" class="rounded-full px-2">Auto</Button>
          <Button variant="ghost" size="sm" class="rounded-full px-2">All sources</Button>
          <span class="ml-auto text-sm text-muted-foreground">Draft response mode</span>
          <Button size="icon" class="rounded-full" aria-label="Send handoff">
            <ArrowUp class="size-4" />
          </Button>
        </InputGroupAddon>
      </InputGroup>
    </div>

    <div class="grid gap-2">
      <Label>Secure input</Label>
      <InputGroup class="[--radius:9999px]">
        <InputGroupAddon>
          <InputGroupButton variant="secondary" aria-label="Info">
            <Info class="size-4" />
          </InputGroupButton>
        </InputGroupAddon>
        <InputGroupAddon class="!pl-1">
          <InputGroupText>https://</InputGroupText>
        </InputGroupAddon>
        <InputGroupInput placeholder="example.com" class="!pl-1" />
        <InputGroupAddon align="inline-end">
          <InputGroupButton variant="ghost" aria-label="Favorite">
            <Star class="size-4" />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </div>

    <div class="grid gap-2">
      <Label>Webhook endpoint</Label>
      <InputGroup>
        <InputGroupAddon>
          <InputGroupText>POST</InputGroupText>
        </InputGroupAddon>
        <InputGroupInput placeholder="/api/v1/webhooks/stripe" />
        <InputGroupAddon align="inline-end">
          <InputGroupButton variant="outline">Copy</InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </div>

    <div class="grid gap-2">
      <Label>Verified handle</Label>
      <InputGroup>
        <InputGroupInput placeholder="@your-handle" />
        <InputGroupAddon align="inline-end">
          <div class="flex size-4 items-center justify-center rounded-full bg-primary text-primary-foreground">
            <Check class="size-3" />
          </div>
        </InputGroupAddon>
      </InputGroup>
    </div>

    <div class="grid gap-4 rounded-lg bg-muted/40 p-4 md:col-span-2">
      <div class="grid gap-1 md:grid-cols-[1fr_auto] md:items-start">
        <div class="grid gap-1">
          <p class="text-sm font-medium leading-none">Domain and callback routing</p>
          <p class="text-sm text-muted-foreground">
            Configuration screens often mix URL prefixes, callback paths, and verification helpers in one block.
          </p>
        </div>
        <Button variant="outline" size="sm">Validate</Button>
      </div>

      <div class="grid gap-3">
        <InputGroup>
          <InputGroupAddon>
            <InputGroupText>https://</InputGroupText>
          </InputGroupAddon>
          <InputGroupInput placeholder="app.example.com" />
          <InputGroupAddon align="inline-end">
            <InputGroupText>Primary domain</InputGroupText>
          </InputGroupAddon>
        </InputGroup>

        <InputGroup>
          <InputGroupAddon>
            <InputGroupText>Callback</InputGroupText>
          </InputGroupAddon>
          <InputGroupInput placeholder="/auth/callback" />
          <InputGroupAddon align="inline-end">
            <div class="flex size-4 items-center justify-center rounded-full bg-primary text-primary-foreground">
              <Check class="size-3" />
            </div>
          </InputGroupAddon>
        </InputGroup>
      </div>
    </div>
  </div>
</template>
