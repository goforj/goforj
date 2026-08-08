<script setup lang="ts">
import { CircleCheckBig } from '@lucide/vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
</script>

<template>
  <Alert>
    <CircleCheckBig class="size-4" />
    <AlertTitle>Everything here ships as local source</AlertTitle>
    <AlertDescription>
      The generated app owns these examples, so teams can adapt them directly
      instead of relying on a remote component catalog.
    </AlertDescription>
  </Alert>
</template>
