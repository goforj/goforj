<template>
  <div class="login-shell">
    <div class="login-grid">
      <section class="login-form-pane">
        <div class="login-form-wrap">
          <div class="login-form-inner">
            <div class="login-brand">
              <img :src="logoMark" alt="Uptime Gopher" class="login-brand-mark" />
              <div class="login-brand-copy">
                <h1 class="login-title">Sign in</h1>
              </div>
            </div>

            <form class="login-form-stack" @submit.prevent="submit">
              <div class="login-form-header">
                <p class="login-form-kicker">Uptime Gopher</p>
                <CardDescription class="login-form-description">
                  Access monitors, incidents, diagnostics, and notification settings from one control surface.
                </CardDescription>
              </div>

              <div class="login-form-fields">
                <div class="grid gap-2">
                  <Label for="login-username">Username</Label>
                  <Input id="login-username" v-model="username" placeholder="admin" />
                </div>
                <div class="grid gap-2">
                  <Label for="login-password">Password</Label>
                  <div class="relative">
                    <Input
                      id="login-password"
                      v-model="password"
                      :type="showPassword ? 'text' : 'password'"
                      placeholder="Your password"
                      class="pr-16"
                    />
                    <button
                      type="button"
                      class="login-toggle"
                      @click="showPassword = !showPassword"
                    >
                      {{ showPassword ? 'Hide' : 'Show' }}
                    </button>
                  </div>
                </div>
              </div>

              <Button type="submit" class="w-full" :disabled="submitting">
                {{ submitting ? 'Signing in…' : 'Sign in' }}
              </Button>
            </form>
          </div>
        </div>
      </section>

      <section class="login-cover">
        <div class="login-cover-overlay">
          <div class="login-cover-copy">
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { CardDescription } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import logoMark from '@/assets/favicons/favicon-96x96.png'
import { signIn } from '@/lib/auth'

const username = ref('admin')
const password = ref('admin')
const showPassword = ref(false)
const submitting = ref(false)
const route = useRoute()
const router = useRouter()

async function submit() {
  if (submitting.value) return
  submitting.value = true
  try {
    await signIn(username.value, password.value)
    const redirect = typeof route.query.redirect === 'string' && route.query.redirect.trim() ? route.query.redirect : '/monitors'
    await router.replace(redirect)
  } catch (error) {
    toast.error('Sign in failed', {
      description: error instanceof Error ? error.message : 'Check your credentials and try again.',
    })
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.login-shell {
  width: 100%;
  min-height: 100vh;
  display: flex;
  flex: 1;
  animation: loginFadeDown 420ms ease-out both;
}

.login-grid {
  display: grid;
  min-height: 100%;
  width: 100%;
  flex: 1;
  overflow: hidden;
  background: color-mix(in oklab, var(--card) 82%, transparent);
}

.login-form-pane {
  display: flex;
  min-width: 0;
  position: relative;
  isolation: isolate;
  background: linear-gradient(180deg, rgba(12, 16, 22, 0.92), rgba(10, 12, 18, 0.96));
}

.login-form-pane::before {
  content: "";
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background:
    linear-gradient(180deg, rgba(8, 10, 16, 0.82), rgba(8, 10, 16, 0.94)),
    radial-gradient(circle at top left, rgba(63, 131, 248, 0.24), transparent 36%),
    radial-gradient(circle at 18% 72%, rgba(28, 62, 181, 0.2), transparent 34%);
  opacity: 0.9;
}

.login-form-pane::after {
  content: "";
  position: absolute;
  inset: 1.75rem 1.5rem 1.75rem 1.75rem;
  z-index: 0;
  pointer-events: none;
  border-radius: 1.75rem;
  border: 1px solid rgba(255, 255, 255, 0.05);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.02), rgba(255, 255, 255, 0)),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.04), transparent 42%);
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
  gap: 1rem;
  transform: translateY(3%);
}

.login-brand {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.2rem 0.1rem 0;
}

.login-brand-mark {
  width: 4rem;
  height: 4rem;
  object-fit: contain;
  filter: drop-shadow(0 12px 28px rgba(0, 0, 0, 0.32));
}

.login-brand-copy {
  display: grid;
}

.login-title {
  font-size: clamp(2rem, 2.7vw, 2.35rem);
  line-height: 0.98;
  font-weight: 700;
  letter-spacing: -0.04em;
  color: var(--foreground);
}

.login-form-stack {
  display: grid;
  gap: 1rem;
  border: 1px solid color-mix(in oklab, var(--border) 62%, transparent);
  border-radius: 1rem;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.035), rgba(255, 255, 255, 0)),
    color-mix(in oklab, var(--card) 76%, transparent);
  padding: 1.35rem;
  box-shadow: 0 18px 38px rgba(0, 0, 0, 0.2);
}

.login-form-header {
  display: grid;
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

.login-form-fields {
  display: grid;
  gap: 1rem;
}

.login-toggle {
  position: absolute;
  right: 0.5rem;
  top: 50%;
  transform: translateY(-50%);
  border-radius: 0.5rem;
  border: 1px solid color-mix(in oklab, var(--border) 72%, transparent);
  background: color-mix(in oklab, var(--muted) 38%, transparent);
  padding: 0.2rem 0.55rem;
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.18em;
  color: var(--muted-foreground);
}

.login-toggle:hover {
  color: var(--foreground);
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
    linear-gradient(180deg, rgba(8, 10, 16, 0.18), rgba(8, 10, 16, 0.84)),
    linear-gradient(90deg, rgba(8, 10, 16, 0.78) 0%, rgba(8, 10, 16, 0.42) 34%, rgba(8, 10, 16, 0.12) 56%, rgba(8, 10, 16, 0.34) 100%),
    radial-gradient(circle at 20% 52%, rgba(41, 92, 255, 0.16), transparent 28%),
    url("../assets/uptime-gopher-wallpaper.png");
  background-size: cover;
  background-position: 72% center;
  background-repeat: no-repeat;
  transform: scale(1.02);
}

.login-cover-overlay {
  position: relative;
  z-index: 1;
  display: flex;
  height: 100%;
  align-items: flex-end;
  padding: 2.5rem 2.75rem 2.75rem;
}

.login-cover-copy {
  max-width: 23rem;
  display: grid;
  gap: 0.8rem;
  padding: 1.35rem 1.4rem 1.5rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 1.2rem;
  background:
    linear-gradient(180deg, rgba(8, 10, 16, 0.44), rgba(8, 10, 16, 0.74)),
    color-mix(in oklab, var(--card) 22%, transparent);
  backdrop-filter: blur(10px);
  box-shadow: 0 18px 44px rgba(0, 0, 0, 0.28);
}

.login-cover-kicker {
  font-size: 0.8rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.74);
}

.login-cover-title {
  font-size: clamp(1.85rem, 2.7vw, 2.55rem);
  line-height: 0.96;
  font-weight: 700;
  letter-spacing: -0.05em;
  color: white;
  text-wrap: balance;
  text-shadow: 0 8px 28px rgba(0, 0, 0, 0.36);
}

.login-cover-text {
  max-width: 22rem;
  font-size: 0.95rem;
  line-height: 1.6;
  color: rgba(255, 255, 255, 0.72);
}

@keyframes loginFadeDown {
  from {
    opacity: 0;
    transform: translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
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
