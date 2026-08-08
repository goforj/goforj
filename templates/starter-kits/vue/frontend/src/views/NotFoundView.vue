<template>
  <section class="flex flex-1 items-center justify-center">
    <Empty class="max-w-md">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <FileQuestion />
        </EmptyMedia>
        <EmptyTitle>Page not found</EmptyTitle>
        <EmptyDescription>
          We couldn't find <code>{{ attemptedPath }}</code>. It may have been moved, or the link
          that brought you here may be out of date.
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <div class="flex flex-wrap items-center justify-center gap-2">
          <Button as-child size="sm">
            <RouterLink to="/">
              <House class="size-3.5" />
              Back to dashboard
            </RouterLink>
          </Button>
          <Button variant="outline" size="sm" @click="goBack">
            <ArrowLeft class="size-3.5" />
            Go back
          </Button>
        </div>
      </EmptyContent>
    </Empty>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, FileQuestion, House } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'

const route = useRoute()
const router = useRouter()

const attemptedPath = computed(() => route.fullPath)

function goBack() {
  if (window.history.state?.back) {
    router.back()
    return
  }
  router.replace('/')
}
</script>
