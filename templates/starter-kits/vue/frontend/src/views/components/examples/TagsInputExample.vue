<script setup lang="ts">
import { ref } from 'vue'
import {
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText,
} from '@/components/ui/tags-input'

const tags = ref(['admin', 'finance', 'ops'])
</script>

<template>
  <TagsInput v-model="tags">
    <TagsInputItem v-for="tag in tags" :key="tag" :value="tag">
      <TagsInputItemText />
      <TagsInputItemDelete />
    </TagsInputItem>
    <TagsInputInput placeholder="Add tag..." />
  </TagsInput>
</template>
