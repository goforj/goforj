<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { Check, Code2, Copy } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'
import { componentImportPath, slugify } from '.'

const props = defineProps<{
  /** The ui component this specimen demonstrates, in PascalCase. */
  name: string
  /** One line on when to reach for it. */
  description?: string
  /** Extra component names this specimen also relies on. */
  also?: string[]
  /**
   * Source of the rendered example, imported with Vite's `?raw` suffix from
   * the very same file the default slot renders. Importing one file both
   * ways is what keeps the listing and the live example from drifting.
   */
  source?: string
  class?: string
  contentClass?: string
}>()

const path = computed(() => componentImportPath(props.name))
const anchor = computed(() => slugify(props.name))

const showSource = ref(false)
const copied = ref(false)
let copyTimer: ReturnType<typeof setTimeout> | undefined

async function copySource() {
  if (!props.source) {
    return
  }
  await navigator.clipboard.writeText(props.source)
  copied.value = true
  clearTimeout(copyTimer)
  copyTimer = setTimeout(() => (copied.value = false), 1500)
}

onBeforeUnmount(() => clearTimeout(copyTimer))
</script>

<template>
  <section
    :id="anchor"
    data-slot="specimen"
    :class="cn('scroll-mt-24 overflow-hidden rounded-xl border bg-card', props.class)"
  >
    <header class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 border-b px-4 py-2.5">
      <a :href="`#${anchor}`" class="text-sm font-medium text-foreground hover:underline underline-offset-4">
        {{ name }}
      </a>
      <code class="font-mono text-[11px] text-muted-foreground" :data-import="path">{{ path }}</code>
    </header>

    <div class="grid gap-4 p-4">
      <p v-if="description" class="text-sm text-muted-foreground">{{ description }}</p>
      <p v-if="also?.length" class="-mt-1 flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
        <span>Composes with</span>
        <code
          v-for="extra in also"
          :key="extra"
          class="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px]"
          :data-import="componentImportPath(extra)"
        >{{ extra }}</code>
      </p>
      <div :class="cn('grid gap-4', props.contentClass)">
        <slot />
      </div>

      <Collapsible v-if="source" v-model:open="showSource" class="grid gap-2">
        <div class="flex items-center gap-2">
          <CollapsibleTrigger as-child>
            <Button variant="ghost" size="sm" class="-ml-2 gap-1.5 text-muted-foreground">
              <Code2 class="size-3.5" />
              {{ showSource ? 'Hide code' : 'Show code' }}
            </Button>
          </CollapsibleTrigger>
          <Button
            v-if="showSource"
            variant="ghost"
            size="sm"
            class="gap-1.5 text-muted-foreground"
            @click="copySource"
          >
            <component :is="copied ? Check : Copy" class="size-3.5" />
            {{ copied ? 'Copied' : 'Copy' }}
          </Button>
        </div>

        <CollapsibleContent>
          <pre class="max-h-96 overflow-auto rounded-lg bg-muted/40 p-4 text-xs leading-relaxed"><code>{{ source.trim() }}</code></pre>
        </CollapsibleContent>
      </Collapsible>
    </div>
  </section>
</template>
