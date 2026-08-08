<script setup lang="ts">
import { LoaderCircle, Plus } from '@lucide/vue'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'

const pendingMembers = ['AR', 'MK', 'TS']
</script>

<template>
  <div class="grid gap-4 md:grid-cols-2">
    <!-- Empty sets border-dashed but no width, so pass a border here. -->
    <Empty class="border p-6 md:p-6">
      <EmptyHeader>
        <EmptyMedia variant="default" class="mb-2">
          <div class="flex -space-x-2">
            <Avatar v-for="member in pendingMembers" :key="member" class="ring-background size-8 ring-2">
              <AvatarFallback class="text-xs">{{ member }}</AvatarFallback>
            </Avatar>
          </div>
        </EmptyMedia>
        <EmptyTitle>No team members added</EmptyTitle>
        <EmptyDescription>Invite collaborators when the workspace is ready.</EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button size="sm">
          <Plus class="size-3.5" />
          Invite collaborators
        </Button>
      </EmptyContent>
    </Empty>

    <Empty class="border p-6 md:p-6">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <LoaderCircle class="size-4 animate-spin" />
        </EmptyMedia>
        <EmptyTitle>Processing request</EmptyTitle>
        <EmptyDescription>Please wait while we process your request.</EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button variant="outline" size="sm">Cancel</Button>
      </EmptyContent>
    </Empty>
  </div>
</template>
