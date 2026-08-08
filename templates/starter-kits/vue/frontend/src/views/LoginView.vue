<template>
  <AuthLayout title="Log in to your account" description="Enter your email and password below to log in">
    <form class="grid gap-6" @submit.prevent="submit">
      <div class="grid gap-2">
        <Label for="login">Username or email</Label>
        <Input id="login" v-model="login" autocomplete="username" placeholder="email@example.com" autofocus />
      </div>

      <div class="grid gap-2">
        <Label for="password">Password</Label>
        <PasswordInput id="password" v-model="password" autocomplete="current-password" placeholder="Password" />
        <div class="flex justify-end">
          <RouterLink
            to="/forgot-password"
            class="text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
          >
            Forgot password?
          </RouterLink>
        </div>
      </div>

      <Label for="remember" class="flex items-center gap-3">
        <Checkbox id="remember" v-model="remember" />
        <span>Remember me</span>
      </Label>

      <StatusMessage v-if="errorMessage">{{ errorMessage }}</StatusMessage>

      <Button type="submit" class="w-full" :disabled="submitting">
        <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
        {{ submitting ? 'Logging in...' : 'Log in' }}
      </Button>
    </form>

    <template #footer>
      Don't have an account?
      <RouterLink to="/register" class="underline underline-offset-4 hover:text-foreground">Sign up</RouterLink>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { LoaderCircle } from '@lucide/vue'
import StatusMessage from '@/components/StatusMessage.vue'
import { AuthLayout, PasswordInput } from '@/components/auth'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { loginWithPassword } from '@/lib/auth'

const login = ref('')
const password = ref('')
const remember = ref(false)
const submitting = ref(false)
const errorMessage = ref('')
const router = useRouter()

async function submit() {
  submitting.value = true
  errorMessage.value = ''
  try {
    const user = await loginWithPassword(login.value, password.value, remember.value)
    if (!user) {
      throw new Error('Signed in, but the current user endpoint did not return a user.')
    }
    await router.replace('/')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Unable to log in.'
  } finally {
    submitting.value = false
  }
}
</script>
