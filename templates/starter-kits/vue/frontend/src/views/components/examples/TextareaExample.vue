<script setup lang="ts">
import { ref } from 'vue'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

const summary = ref('')
</script>

<template>
  <div class="grid gap-2">
    <Label for="summary">Summary</Label>
    <Textarea id="summary" v-model="summary" rows="4" placeholder="Describe the screen you want to build next." />
    <p class="text-xs text-muted-foreground">{{ summary.length }} characters</p>
  </div>
</template>
