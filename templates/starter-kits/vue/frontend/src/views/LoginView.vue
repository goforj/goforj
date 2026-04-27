<template>
  <div class="login-shell">
    <div class="login-grid">
      <section class="login-form-pane">
        <div class="login-form-wrap">
          <div class="login-form-inner">
            <div class="login-brand">
              <img :src="logoMark" alt="GoForj Starter Kit" class="login-brand-mark" />
              <div class="login-brand-copy">
                <p class="login-eyebrow">GoForj Starter Kit</p>
                <h1 class="login-title">Sign in</h1>
                <p class="login-subtitle">Sign in to continue.</p>
              </div>
            </div>

            <form class="login-form-stack" @submit.prevent="submit">
              <div class="login-form-header">
                <p class="login-form-kicker">Application Auth</p>
                <CardDescription class="login-form-description">
                  Connect this form to local auth, OAuth, password reset, and session management as your product evolves.
                </CardDescription>
              </div>

              <div class="login-form-fields">
                <div class="grid gap-2">
                  <Label for="login">Username or email</Label>
                  <Input id="login" v-model="login" autocomplete="username" placeholder="admin" />
                </div>
                <div class="grid gap-2">
                  <Label for="password">Password</Label>
                  <div class="relative">
                    <Input
                      id="password"
                      v-model="password"
                      :type="showPassword ? 'text' : 'password'"
                      autocomplete="current-password"
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
                <label class="flex items-center gap-3 text-sm text-foreground">
                  <Checkbox v-model="remember" />
                  <span>Remember me</span>
                </label>
              </div>

              <p v-if="errorMessage" class="rounded-lg border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive">
                {{ errorMessage }}
              </p>

              <Button type="submit" variant="default" class="w-full" :disabled="submitting">
                <LoaderCircle v-if="submitting" class="size-4 animate-spin" />
                {{ submitting ? 'Signing in...' : 'Sign in' }}
              </Button>
            </form>
          </div>
        </div>
      </section>

      <section class="login-cover">
        <div class="login-cover-overlay">
          <div class="login-cover-copy">
            <p class="login-cover-kicker">Starter foundation</p>
            <h2 class="login-cover-title">An application shell with auth flows and a dashboard starting point.</h2>
            <p class="login-cover-text">
              The starter is copied into your application so your team can shape and maintain the code locally.
            </p>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { LoaderCircle } from 'lucide-vue-next'
import { loginWithPassword } from '@/lib/auth'
import logoMark from '@/assets/goforj-v7.png'
import Button from '@/components/ui/button/Button.vue'
import CardDescription from '@/components/ui/card/CardDescription.vue'
import Checkbox from '@/components/ui/checkbox/Checkbox.vue'
import Input from '@/components/ui/input/Input.vue'
import Label from '@/components/ui/label/Label.vue'

const login = ref('admin')
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
    errorMessage.value = error instanceof Error ? error.message : 'Unable to sign in.'
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.login-shell {
  width: 100%;
  min-height: 100%;
  display: flex;
  flex: 1;
}

.login-grid {
  display: grid;
  min-height: 100%;
  width: 100%;
  flex: 1;
  overflow: hidden;
  background:
    linear-gradient(180deg, color-mix(in oklab, var(--background) 94%, var(--card)), color-mix(in oklab, var(--background) 98%, black 2%));
}

.login-form-pane {
  display: flex;
  min-width: 0;
  position: relative;
  isolation: isolate;
  background:
    linear-gradient(180deg, color-mix(in oklab, var(--card) 94%, white), color-mix(in oklab, var(--background) 96%, white));
}

.login-form-pane::before {
  content: "";
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.14), rgba(255, 255, 255, 0.03));
  opacity: 1;
}

.login-form-wrap {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
  padding: 2.5rem 3rem 2.75rem;
  position: relative;
  z-index: 1;
}

.login-form-inner {
  width: min(100%, 28rem);
  display: grid;
  gap: 1.1rem;
  transform: translateY(3%);
}

.login-brand {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.login-brand-mark {
  width: 3.5rem;
  height: 3.5rem;
  object-fit: contain;
  filter: drop-shadow(0 8px 24px rgba(0, 0, 0, 0.28));
}

.login-brand-copy {
  display: grid;
  gap: 0.05rem;
}

.login-eyebrow {
  font-size: 0.72rem;
  letter-spacing: 0.28em;
  text-transform: uppercase;
  color: color-mix(in oklab, var(--foreground) 72%, transparent);
}

.login-title {
  font-size: clamp(1.85rem, 2.5vw, 2.2rem);
  line-height: 1;
  font-weight: 700;
  letter-spacing: -0.04em;
  color: var(--foreground);
}

.login-subtitle {
  font-size: 0.9rem;
  max-width: 24rem;
  color: color-mix(in oklab, var(--foreground) 58%, transparent);
}

.login-form-stack {
  display: grid;
  gap: 1rem;
  border: 1px solid color-mix(in oklab, var(--border) 62%, transparent);
  border-radius: 1rem;
  background: color-mix(in oklab, var(--card) 94%, transparent);
  padding: 1.35rem;
  box-shadow: 0 14px 28px rgba(0, 0, 0, 0.18);
}

.login-form-header,
.login-form-fields {
  display: grid;
  gap: 1rem;
}

.login-form-header {
  gap: 0.25rem;
}

.login-form-kicker {
  font-size: 0.72rem;
  letter-spacing: 0.28em;
  text-transform: uppercase;
  color: color-mix(in oklab, var(--foreground) 68%, transparent);
}

.login-form-description {
  max-width: 24rem;
  color: color-mix(in oklab, var(--foreground) 60%, transparent);
}

.login-cover {
  position: relative;
  display: none;
  min-width: 0;
  overflow: hidden;
}

.login-cover::before {
  content: "";
  position: absolute;
  inset: 0;
  background:
    linear-gradient(135deg, color-mix(in oklab, var(--card) 96%, white), color-mix(in oklab, var(--background) 94%, white));
  background-size: cover;
  background-position: 54% 44%;
  background-repeat: no-repeat;
  transform: scale(0.98);
}

.login-cover-overlay {
  position: relative;
  z-index: 1;
  display: flex;
  height: 100%;
  align-items: center;
  padding: 2.25rem 2.75rem;
}

.login-cover-copy {
  max-width: 24rem;
  display: grid;
  gap: 0.75rem;
  transform: translateY(8%);
}

.login-cover-kicker {
  font-size: 0.72rem;
  letter-spacing: 0.28em;
  text-transform: uppercase;
  color: color-mix(in oklab, var(--foreground) 72%, transparent);
}

.login-cover-title {
  font-size: clamp(1.85rem, 2.85vw, 2.7rem);
  line-height: 0.96;
  font-weight: 700;
  letter-spacing: -0.05em;
  color: var(--foreground);
  text-wrap: balance;
}

.login-cover-text {
  max-width: 22rem;
  font-size: 0.95rem;
  line-height: 1.6;
  color: color-mix(in oklab, var(--foreground) 62%, transparent);
}

:global(.dark) .login-form-pane {
  background:
    linear-gradient(180deg, hsl(0 0% 5%), hsl(0 0% 7%));
}

:global(.dark) .login-form-pane::before {
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.015), rgba(255, 255, 255, 0));
  opacity: 0.95;
}

:global(.dark) .login-form-stack {
  border-color: rgba(148, 163, 184, 0.16);
  background: linear-gradient(180deg, rgba(14, 20, 30, 0.98), rgba(12, 18, 28, 0.94));
  box-shadow:
    0 22px 42px rgba(0, 0, 0, 0.34),
    inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

:global(.dark) .login-grid {
  background: linear-gradient(180deg, hsl(0 0% 4%), hsl(0 0% 5.5%));
}

:global(.dark) .login-cover::before {
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.02), rgba(255, 255, 255, 0)),
    linear-gradient(135deg, hsl(0 0% 5%), hsl(0 0% 8%) 48%, hsl(0 0% 5%));
  transform: scale(1);
}

:global(.dark) .login-cover {
  border-left-color: rgba(148, 163, 184, 0.08);
}

:global(.dark) .login-eyebrow,
:global(.dark) .login-form-kicker,
:global(.dark) .login-cover-kicker {
  color: rgba(255, 255, 255, 0.72);
}

:global(.dark) .login-subtitle,
:global(.dark) .login-form-description,
:global(.dark) .login-cover-text {
  color: rgba(255, 255, 255, 0.62);
}

:global(.dark) .login-cover-title {
  color: white;
}

@media (min-width: 960px) {
  .login-grid {
    grid-template-columns: minmax(420px, 0.92fr) minmax(0, 1.08fr);
  }

  .login-cover {
    display: block;
    border-left: 1px solid color-mix(in oklab, var(--border) 72%, transparent);
  }
}
</style>
