<template>
  <AuthLayout title="Reset password" description="Choose a new password for your account">
    <form class="grid gap-6" @submit.prevent="submit">
      <div class="grid gap-2">
        <Label for="password">New password</Label>
        <PasswordInput id="password" v-model="password" autocomplete="new-password" placeholder="New password" />
        <p class="text-xs text-muted-foreground">{{ passwordRulesText }}</p>
      </div>

      <div class="grid gap-2">
        <Label for="password-confirmation">Confirm new password</Label>
        <PasswordInput
          id="password-confirmation"
          v-model="passwordConfirmation"
          autocomplete="new-password"
          placeholder="Confirm new password"
        />
      </div>

      <AuthMessage v-if="successMessage" variant="success">{{ successMessage }}</AuthMessage>
      <AuthMessage v-if="errorMessage">{{ errorMessage }}</AuthMessage>

      <Button type="submit" class="w-full" :disabled="submitting || success || !token">
        <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
        {{ submitting ? 'Resetting password...' : success ? 'Password updated' : 'Reset password' }}
      </Button>
    </form>

    <template #footer>
      Remembered it?
      <RouterLink to="/login" class="underline underline-offset-4 hover:text-foreground">Log in</RouterLink>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { LoaderCircle } from '@lucide/vue'
import { AuthLayout, AuthMessage, PasswordInput } from '@/components/auth'
import { resetPassword } from '@/lib/auth'
import { passwordRequirementsText } from '@/lib/password-policy'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'

const route = useRoute()
const router = useRouter()
const token = ref(typeof route.query.token === 'string' ? route.query.token : '')
const password = ref('')
const passwordConfirmation = ref('')
const submitting = ref(false)
const success = ref(false)
const successMessage = ref('')
const errorMessage = ref('')
const passwordRulesText = passwordRequirementsText()

async function submit() {
  if (!token.value.trim()) {
    errorMessage.value = 'Reset link is invalid or incomplete.'
    return
  }
  if (password.value !== passwordConfirmation.value) {
    errorMessage.value = 'Passwords do not match.'
    return
  }
  submitting.value = true
  errorMessage.value = ''
  try {
    await resetPassword(token.value, password.value)
    success.value = true
    successMessage.value = 'Your password has been reset. Redirecting you to log in...'
    window.setTimeout(() => {
      void router.replace('/login')
    }, 1200)
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Unable to reset your password.'
  } finally {
    submitting.value = false
  }
}
</script>
