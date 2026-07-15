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
            <h1 class="text-xl font-medium">Verify email</h1>
            <p class="text-sm text-muted-foreground">Confirm your email address to continue</p>
          </div>
        </div>

        <div class="grid gap-6">
          <p v-if="successMessage" class="text-center text-sm font-medium text-green-600">
            {{ successMessage }}
          </p>

          <p
            v-if="errorMessage"
            class="rounded-lg border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive"
          >
            {{ errorMessage }}
          </p>

          <Button type="button" class="w-full" :disabled="submitting || success" @click="submit">
            <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
            {{ submitting ? 'Verifying email...' : success ? 'Email verified' : 'Verify email' }}
          </Button>
        </div>

        <div class="text-center text-sm text-muted-foreground">
          Back to
          <RouterLink to="/login" class="underline underline-offset-4 hover:text-foreground">log in</RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { LoaderCircle } from '@lucide/vue'
import { verifyEmail } from '@/lib/auth'
import logoMark from '@/assets/goforj-logo.png'
import Button from '@/components/ui/button/Button.vue'

const route = useRoute()
const router = useRouter()
const token = ref(typeof route.query.token === 'string' ? route.query.token : '')
const submitting = ref(false)
const success = ref(false)
const successMessage = ref('')
const errorMessage = ref('')

async function submit() {
  if (!token.value.trim()) {
    errorMessage.value = 'Verification link is invalid or incomplete.'
    return
  }
  submitting.value = true
  errorMessage.value = ''
  try {
    await verifyEmail(token.value)
    success.value = true
    successMessage.value = 'Your email has been verified. Redirecting you to log in...'
    window.setTimeout(() => {
      void router.replace('/login')
    }, 1200)
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Unable to verify your email.'
  } finally {
    submitting.value = false
  }
}
</script>
