<script setup lang="ts">
import { ref } from 'vue'
import { PinInput, PinInputGroup, PinInputSlot } from '@/components/ui/pin-input'

const pin = ref(['1', '4', '8', '2'])
</script>

<template>
  <PinInput v-model="pin" class="justify-between">
    <PinInputGroup class="gap-2">
      <PinInputSlot v-for="index in 4" :key="index" :index="index - 1" />
    </PinInputGroup>
  </PinInput>
</template>
