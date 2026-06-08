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
            <h1 class="text-xl font-medium">Forgot password</h1>
            <p class="text-sm text-muted-foreground">Enter your email to receive a password reset link</p>
          </div>
        </div>

        <form class="flex flex-col gap-6" @submit.prevent="submit">
          <div class="grid gap-6">
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

            <p
              v-if="successMessage"
              class="text-center text-sm font-medium text-green-600"
            >
              {{ successMessage }}
            </p>

            <div
              v-if="showLocalResetLink"
              class="rounded-lg border border-border bg-muted/20 px-3 py-3 text-sm text-muted-foreground"
            >
              <p class="font-medium text-foreground">Local development shortcut</p>
              <a :href="resetLink" class="mt-2 inline-flex text-sm text-foreground underline underline-offset-4">
                Open reset password page
              </a>
            </div>

            <p
              v-if="errorMessage"
              class="rounded-lg border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive"
            >
              {{ errorMessage }}
            </p>

            <Button type="submit" class="w-full" :disabled="submitting || submitted">
              <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
              {{ submitting ? 'Sending reset link...' : submitted ? 'Instructions sent' : 'Email password reset link' }}
            </Button>
          </div>
        </form>

        <div class="text-center text-sm text-muted-foreground">
          Or, return to
          <RouterLink to="/login" class="underline underline-offset-4 hover:text-foreground">log in</RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { LoaderCircle } from 'lucide-vue-next'
import { requestPasswordReset } from '@/lib/auth'
import logoMark from '@/assets/goforj-logo.png'
import Button from '@/components/ui/button/Button.vue'
import Input from '@/components/ui/input/Input.vue'
import Label from '@/components/ui/label/Label.vue'

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
