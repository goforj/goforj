<script setup lang="ts">
import { ref } from 'vue'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldSeparator,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

const project = ref('Acme Admin')
const approvals = ref(true)
</script>

<template>
  <FieldGroup class="gap-4">
    <Field>
      <FieldLabel>Project name</FieldLabel>
      <Input v-model="project" />
      <FieldDescription>Stacked is the default orientation.</FieldDescription>
    </Field>

    <FieldSeparator />

    <Field orientation="horizontal">
      <FieldContent>
        <FieldLabel>Require device approval</FieldLabel>
        <FieldDescription>Horizontal puts the control beside the label.</FieldDescription>
      </FieldContent>
      <Switch v-model="approvals" />
    </Field>
  </FieldGroup>
</template>
