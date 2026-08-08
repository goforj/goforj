<template>
  <AuthLayout title="Verify email" description="Confirm your email address to continue">
    <!-- A form rather than a bare button, so Enter submits like every other auth screen. -->
    <form class="grid gap-6" @submit.prevent="submit">
      <AuthMessage v-if="successMessage" variant="success">{{ successMessage }}</AuthMessage>
      <AuthMessage v-if="errorMessage">{{ errorMessage }}</AuthMessage>

      <Button type="submit" class="w-full" :disabled="submitting || success">
        <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
        {{ submitting ? 'Verifying email...' : success ? 'Email verified' : 'Verify email' }}
      </Button>
    </form>

    <template #footer>
      Back to
      <RouterLink to="/login" class="underline underline-offset-4 hover:text-foreground">log in</RouterLink>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { LoaderCircle } from '@lucide/vue'
import { AuthLayout, AuthMessage } from '@/components/auth'
import { Button } from '@/components/ui/button'
import { verifyEmail } from '@/lib/auth'

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
