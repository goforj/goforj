<template>
  <div class="flex min-h-full flex-1 items-center justify-center bg-background p-6 md:p-10">
    <div class="w-full max-w-sm">
      <div class="flex flex-col gap-8">
        <div class="flex flex-col items-center gap-4">
          <RouterLink to="/" class="flex flex-col items-center gap-2 font-medium">
            <img :src="logoMark" alt="GoForj Starter Kit" class="h-12 w-12 object-contain" />
            <span class="sr-only">GoForj Starter Kit</span>
          </RouterLink>

          <div class="space-y-2 text-center">
            <h1 class="text-xl font-medium">Reset password</h1>
            <p class="text-sm text-muted-foreground">Please enter your new password below</p>
          </div>
        </div>

        <form class="flex flex-col gap-6" @submit.prevent="submit">
          <div class="grid gap-6">
            <div class="grid gap-2">
              <Label for="password">New password</Label>
              <div class="relative">
                <Input
                  id="password"
                  v-model="password"
                  :type="showPassword ? 'text' : 'password'"
                  autocomplete="new-password"
                  placeholder="New password"
                  class="pr-16"
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 -translate-y-1/2 rounded-md border border-border/60 bg-muted/40 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-muted-foreground hover:text-foreground"
                  @click="showPassword = !showPassword"
                >
                  {{ showPassword ? 'Hide' : 'Show' }}
                </button>
              </div>
              <p class="text-xs text-muted-foreground">
                {{ passwordRulesText }}
              </p>
            </div>

            <div class="grid gap-2">
              <Label for="password-confirmation">Confirm new password</Label>
              <Input
                id="password-confirmation"
                v-model="passwordConfirmation"
                :type="showPassword ? 'text' : 'password'"
                autocomplete="new-password"
                placeholder="Confirm new password"
              />
            </div>

            <p
              v-if="successMessage"
              class="text-center text-sm font-medium text-green-600"
            >
              {{ successMessage }}
            </p>

            <p
              v-if="errorMessage"
              class="rounded-lg border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive"
            >
              {{ errorMessage }}
            </p>

            <Button type="submit" class="mt-4 w-full" :disabled="submitting || success || !token">
              <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
              {{ submitting ? 'Resetting password...' : success ? 'Password updated' : 'Reset password' }}
            </Button>
          </div>
        </form>

        <div class="text-center text-sm text-muted-foreground">
          Remembered it?
          <RouterLink to="/login" class="underline underline-offset-4 hover:text-foreground">Log in</RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { LoaderCircle } from '@lucide/vue'
import { resetPassword } from '@/lib/auth'
import { passwordRequirementsText } from '@/lib/password-policy'
import logoMark from '@/assets/goforj-logo.png'
import Button from '@/components/ui/button/Button.vue'
import Input from '@/components/ui/input/Input.vue'
import Label from '@/components/ui/label/Label.vue'

const route = useRoute()
const router = useRouter()
const token = ref(typeof route.query.token === 'string' ? route.query.token : '')
const password = ref('')
const passwordConfirmation = ref('')
const submitting = ref(false)
const showPassword = ref(false)
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
