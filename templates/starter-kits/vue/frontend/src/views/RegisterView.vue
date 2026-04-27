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
            <h1 class="text-xl font-medium">Create your account</h1>
            <p class="text-sm text-muted-foreground">Enter your details below to sign up</p>
          </div>
        </div>

        <form class="flex flex-col gap-6" @submit.prevent="submit">
          <div class="grid gap-6">
            <div class="grid gap-2">
              <Label for="display-name">Name</Label>
              <Input
                id="display-name"
                v-model="displayName"
                autocomplete="name"
                placeholder="Full name"
                autofocus
              />
            </div>

            <div class="grid gap-2">
              <Label for="email">Email address</Label>
              <Input id="email" v-model="email" type="email" autocomplete="email" placeholder="email@example.com" />
            </div>

            <div class="grid gap-2">
              <Label for="password">Password</Label>
              <div class="relative">
                <Input
                  id="password"
                  v-model="password"
                  :type="showPassword ? 'text' : 'password'"
                  autocomplete="new-password"
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
            </div>

            <div class="grid gap-2">
              <Label for="password-confirmation">Confirm password</Label>
              <Input
                id="password-confirmation"
                v-model="passwordConfirmation"
                :type="showPassword ? 'text' : 'password'"
                autocomplete="new-password"
                placeholder="Confirm password"
              />
            </div>

            <p
              v-if="errorMessage"
              class="rounded-lg border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive"
            >
              {{ errorMessage }}
            </p>

            <Button type="submit" class="mt-2 w-full" :disabled="submitting">
              <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
              {{ submitting ? 'Creating account...' : 'Create account' }}
            </Button>
          </div>
        </form>

        <div class="text-center text-sm text-muted-foreground">
          Already have an account?
          <RouterLink to="/login" class="underline underline-offset-4 hover:text-foreground">Log in</RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { LoaderCircle } from 'lucide-vue-next'
import { registerWithPassword } from '@/lib/auth'
import logoMark from '@/assets/goforj-v7.png'
import Button from '@/components/ui/button/Button.vue'
import Input from '@/components/ui/input/Input.vue'
import Label from '@/components/ui/label/Label.vue'

const displayName = ref('')
const email = ref('')
const password = ref('')
const passwordConfirmation = ref('')
const submitting = ref(false)
const showPassword = ref(false)
const errorMessage = ref('')
const router = useRouter()

async function submit() {
  if (password.value !== passwordConfirmation.value) {
    errorMessage.value = 'Passwords do not match.'
    return
  }
  submitting.value = true
  errorMessage.value = ''
  try {
    const user = await registerWithPassword(displayName.value, email.value, password.value)
    if (!user) {
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
