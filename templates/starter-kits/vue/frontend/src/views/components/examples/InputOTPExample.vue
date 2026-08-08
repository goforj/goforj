<script setup lang="ts">
import { ref } from 'vue'
import { InputOTP, InputOTPGroup, InputOTPSeparator, InputOTPSlot } from '@/components/ui/input-otp'

const code = ref('')
</script>

<template>
  <InputOTP v-model="code" :maxlength="6">
    <template #default="{ slots }">
      <InputOTPGroup>
        <InputOTPSlot v-for="(_, index) in slots.slice(0, 3)" :key="index" :index="index" />
      </InputOTPGroup>
      <InputOTPSeparator />
      <InputOTPGroup>
        <InputOTPSlot v-for="(_, index) in slots.slice(3, 6)" :key="index + 3" :index="index + 3" />
      </InputOTPGroup>
    </template>
  </InputOTP>
</template>
