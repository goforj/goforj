<script setup lang="ts">
import { ref } from 'vue'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const profile = ref('vue')
const region = ref('iad')
</script>

<template>
  <div class="grid gap-4">
    <div class="grid gap-2">
      <Label>Starter profile</Label>
      <Select v-model="profile">
        <SelectTrigger class="w-full">
          <SelectValue placeholder="Choose a profile" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="vue">Vue + shadcn-vue</SelectItem>
          <SelectItem value="auth">Auth shell</SelectItem>
          <SelectItem value="dashboard">Dashboard baseline</SelectItem>
        </SelectContent>
      </Select>
    </div>

    <div class="grid gap-2">
      <Label>Region</Label>
      <NativeSelect v-model="region">
        <NativeSelectOption value="iad">US East</NativeSelectOption>
        <NativeSelectOption value="sfo">US West</NativeSelectOption>
        <NativeSelectOption value="fra">Europe</NativeSelectOption>
      </NativeSelect>
      <p class="text-xs text-muted-foreground">NativeSelect suits lower-importance defaults.</p>
    </div>
  </div>
</template>
