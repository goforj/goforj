<script setup lang="ts">
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'

function notifySaved() {
  toast.success('Settings saved', {
    description: 'Rendered by the local shadcn-vue sonner wrapper.',
  })
}

function notifySignedOut() {
  toast('Signed out', {
    description: 'Non-blocking notifications suit auth and settings flows.',
  })
}
</script>

<template>
  <div class="flex flex-wrap gap-2">
    <Button variant="outline" @click="notifySaved">Show success toast</Button>
    <Button variant="secondary" @click="notifySignedOut">Show signed-out toast</Button>
  </div>
</template>
