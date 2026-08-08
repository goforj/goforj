<script setup lang="ts">
import { ref } from 'vue'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'

const analytics = ref(true)
const auditLogs = ref(false)
const terms = ref(true)
</script>

<template>
  <div class="grid gap-3">
    <Label class="flex items-center gap-3 rounded-lg border p-3">
      <Checkbox v-model="analytics" />
      <span class="text-sm font-normal">Analytics</span>
    </Label>
    <Label class="flex items-center gap-3 rounded-lg border p-3">
      <Checkbox v-model="auditLogs" />
      <span class="text-sm font-normal">Audit logs</span>
    </Label>
    <Label class="flex items-center gap-3 rounded-lg border border-primary/20 bg-primary/10 p-3">
      <Checkbox v-model="terms" />
      <span class="text-sm font-medium">I agree to the terms and conditions</span>
    </Label>
  </div>
</template>
