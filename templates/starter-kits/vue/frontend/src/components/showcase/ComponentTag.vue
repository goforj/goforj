<script setup lang="ts">
import { computed } from 'vue'
import { componentImportPath } from '.'

const props = defineProps<{
  /** One or more ui component names in PascalCase, e.g. ['InputGroup']. */
  names: string[]
}>()

const tags = computed(() =>
  props.names.map((name) => ({ name, path: componentImportPath(name) })),
)
</script>

<template>
  <ul class="flex flex-wrap items-center gap-1" aria-label="Components used">
    <li v-for="tag in tags" :key="tag.name">
      <code
        class="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] leading-normal text-muted-foreground"
        :title="`import { ${tag.name} } from '${tag.path}'`"
        :data-import="tag.path"
      >{{ tag.name }}</code>
    </li>
  </ul>
</template>
