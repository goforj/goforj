<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { Check, Code2, Copy } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { highlightVue } from '@/lib/highlight'

const props = defineProps<{
  /** Component the source belongs to, used for the panel title. */
  name: string
  /** Import path of that component. */
  path: string
  /** Raw file contents. */
  source: string
}>()

const highlighted = computed(() => highlightVue(props.source.trim()))

const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

async function copy() {
  await navigator.clipboard.writeText(props.source.trim())
  copied.value = true
  clearTimeout(timer)
  timer = setTimeout(() => (copied.value = false), 1500)
}

onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <Sheet>
    <SheetTrigger as-child>
      <Button variant="ghost" size="sm" class="h-7 gap-1.5 px-2 text-muted-foreground">
        <Code2 class="size-3.5" />
        Code
      </Button>
    </SheetTrigger>

    <SheetContent class="flex w-full flex-col gap-4 p-0 sm:max-w-2xl">
      <SheetHeader class="gap-1 border-b px-6 pb-4 pt-6 pr-14">
        <SheetTitle>{{ name }}</SheetTitle>
        <SheetDescription class="font-mono text-xs">{{ path }}</SheetDescription>
      </SheetHeader>

      <ScrollArea class="min-h-0 flex-1">
        <pre class="px-6 text-xs leading-relaxed"><code v-html="highlighted" /></pre>
      </ScrollArea>

      <div class="border-t px-6 pb-6 pt-4">
        <Button variant="outline" size="sm" class="gap-1.5" @click="copy">
          <component :is="copied ? Check : Copy" class="size-3.5" />
          {{ copied ? 'Copied' : 'Copy source' }}
        </Button>
      </div>
    </SheetContent>
  </Sheet>
</template>
