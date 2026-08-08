<template>
  <AuthLayout title="Create an account" description="Enter your details below to create your account">
    <form class="grid gap-6" @submit.prevent="submit">
      <div class="grid gap-2">
        <Label for="display-name">Name</Label>
        <Input id="display-name" v-model="displayName" autocomplete="name" placeholder="Full name" autofocus />
      </div>

      <div class="grid gap-2">
        <Label for="email">Email address</Label>
        <Input id="email" v-model="email" type="email" autocomplete="email" placeholder="email@example.com" />
      </div>

      <div class="grid gap-2">
        <Label for="password">Password</Label>
        <PasswordInput id="password" v-model="password" autocomplete="new-password" placeholder="Password" />
        <p class="text-xs text-muted-foreground">{{ passwordRulesText }}</p>
      </div>

      <div class="grid gap-2">
        <Label for="password-confirmation">Confirm password</Label>
        <PasswordInput
          id="password-confirmation"
          v-model="passwordConfirmation"
          autocomplete="new-password"
          placeholder="Confirm password"
        />
      </div>

      <AuthMessage v-if="successMessage" variant="success">{{ successMessage }}</AuthMessage>
      <AuthMessage v-if="errorMessage">{{ errorMessage }}</AuthMessage>

      <Button type="submit" class="w-full" :disabled="submitting">
        <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
        {{ submitting ? 'Creating account...' : 'Create account' }}
      </Button>
    </form>

    <template #footer>
      Already have an account?
      <RouterLink to="/login" class="underline underline-offset-4 hover:text-foreground">Log in</RouterLink>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { LoaderCircle } from '@lucide/vue'
import { AuthLayout, AuthMessage, PasswordInput } from '@/components/auth'
import { registerWithPassword } from '@/lib/auth'
import { passwordRequirementsText } from '@/lib/password-policy'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const displayName = ref('')
const email = ref('')
const password = ref('')
const passwordConfirmation = ref('')
const submitting = ref(false)
const errorMessage = ref('')
const successMessage = ref('')
const verificationLink = ref('')
const router = useRouter()
const passwordRulesText = passwordRequirementsText()

async function submit() {
  if (password.value !== passwordConfirmation.value) {
    errorMessage.value = 'Passwords do not match.'
    return
  }
  submitting.value = true
  errorMessage.value = ''
  successMessage.value = ''
  verificationLink.value = ''
  try {
    const result = await registerWithPassword(displayName.value, email.value, password.value)
    if (result.requires_email_verification) {
      successMessage.value = 'Check your email to verify your account before logging in.'
      if (result.verification_token && import.meta.env.VITE_APP_ENV === 'local') {
        verificationLink.value = `/verify-email?token=${encodeURIComponent(result.verification_token)}`
      }
      displayName.value = ''
      email.value = ''
      password.value = ''
      passwordConfirmation.value = ''
      return
    }
    if (!result.user) {
      throw new Error('Account created, but the current user endpoint did not return a user.')
    }
    await router.replace('/')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Unable to create your account.'
  } finally {
    submitting.value = false
  }
}
</script>
