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
            <h1 class="text-xl font-medium">Log in to your account</h1>
            <p class="text-sm text-muted-foreground">Enter your email and password below to log in</p>
          </div>
        </div>

        <form class="flex flex-col gap-6" @submit.prevent="submit">
          <div class="grid gap-6">
            <div class="grid gap-2">
              <Label for="login">Username or email</Label>
              <Input id="login" v-model="login" autocomplete="username" placeholder="email@example.com" autofocus />
            </div>

            <div class="grid gap-2">
              <div class="flex items-center justify-between">
                <Label for="password">Password</Label>
              </div>
              <div class="relative">
                <Input
                  id="password"
                  v-model="password"
                  :type="showPassword ? 'text' : 'password'"
                  autocomplete="current-password"
                  placeholder="Password"
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
              <div class="flex justify-end">
                <RouterLink
                  to="/forgot-password"
                  class="text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
                >
                  Forgot password?
                </RouterLink>
              </div>
            </div>

            <div class="flex items-center justify-between">
              <Label for="remember" class="flex items-center space-x-3">
                <Checkbox id="remember" v-model="remember" />
                <span>Remember me</span>
              </Label>
            </div>

            <p
              v-if="errorMessage"
              class="rounded-lg border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive"
            >
              {{ errorMessage }}
            </p>

            <Button type="submit" class="mt-4 w-full" :disabled="submitting">
              <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
              {{ submitting ? 'Logging in...' : 'Log in' }}
            </Button>
          </div>
        </form>

        <div class="text-center text-sm text-muted-foreground">
          Don't have an account?
          <RouterLink to="/register" class="underline underline-offset-4 hover:text-foreground">Sign up</RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { LoaderCircle } from '@lucide/vue'
import { loginWithPassword } from '@/lib/auth'
import logoMark from '@/assets/goforj-logo.png'
import Button from '@/components/ui/button/Button.vue'
import Checkbox from '@/components/ui/checkbox/Checkbox.vue'
import Input from '@/components/ui/input/Input.vue'
import Label from '@/components/ui/label/Label.vue'

const login = ref('')
const password = ref('')
const remember = ref(false)
const submitting = ref(false)
const showPassword = ref(false)
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
