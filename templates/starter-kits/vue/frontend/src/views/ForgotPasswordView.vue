<template>
  <AuthLayout title="Forgot password" description="Enter your email to receive a password reset link">
    <form class="grid gap-6" @submit.prevent="submit">
      <div class="grid gap-2">
        <Label for="login">Email address</Label>
        <Input
          id="login"
          v-model="login"
          type="email"
          autocomplete="off"
          placeholder="email@example.com"
          autofocus
          :disabled="submitting || submitted"
        />
      </div>

      <StatusMessage v-if="successMessage" variant="success">{{ successMessage }}</StatusMessage>

      <div
        v-if="showLocalResetLink"
        class="grid gap-2 rounded-lg border bg-muted/20 px-3 py-3 text-sm text-muted-foreground"
      >
        <p class="font-medium text-foreground">Local development shortcut</p>
        <a :href="resetLink" class="text-sm text-foreground underline underline-offset-4">Open reset password page</a>
      </div>

      <StatusMessage v-if="errorMessage">{{ errorMessage }}</StatusMessage>

      <Button type="submit" class="w-full" :disabled="submitting || submitted">
        <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
        {{ submitting ? 'Sending reset link...' : submitted ? 'Instructions sent' : 'Email password reset link' }}
      </Button>
    </form>

    <template #footer>
      Or, return to
      <RouterLink to="/login" class="underline underline-offset-4 hover:text-foreground">log in</RouterLink>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { LoaderCircle } from '@lucide/vue'
import StatusMessage from '@/components/StatusMessage.vue'
import { AuthLayout } from '@/components/auth'
import { requestPasswordReset } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const login = ref('')
const submitting = ref(false)
const submitted = ref(false)
const successMessage = ref('')
const errorMessage = ref('')
const resetLink = ref('')
const showLocalResetLink = computed(() => import.meta.env.VITE_APP_ENV === 'local' && Boolean(resetLink.value))

async function submit() {
  submitting.value = true
  errorMessage.value = ''
  resetLink.value = ''
  try {
    const result = await requestPasswordReset(login.value)
    submitted.value = true
    successMessage.value = 'We have emailed your password reset link.'
    resetLink.value = result.reset_token
      ? `/reset-password?token=${encodeURIComponent(result.reset_token)}`
      : ''
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : 'Unable to send password reset instructions.'
  } finally {
    submitting.value = false
  }
}
</script>
