<script setup lang="ts">
import { IconGripVertical } from "@tabler/icons-vue"
import { useSortableContext } from "dnd-kit-vue"
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'

const { handleRef, sortable } = useSortableContext()
const { t } = useI18n()
</script>

<template>
  <Button
    :ref="handleRef"
    variant="ghost"
    size="icon"
    class="text-muted-foreground size-7 hover:bg-transparent"
  >
    <IconGripVertical class="text-muted-foreground size-3" />
    <span class="sr-only">{{ t('common.dragToReorder') }}</span>
  </Button>
</template>
