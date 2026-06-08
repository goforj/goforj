<template>
  <div class="login-shell">
    <div class="login-grid">
      <section class="login-form-pane">
        <div class="login-form-wrap">
          <div class="login-form-inner">
            <div class="login-brand">
              <img :src="logoMark" alt="GoForj Lighthouse" class="login-brand-mark" />
              <div class="login-brand-copy">
                <p class="login-eyebrow">GoForj Lighthouse</p>
                <h1 class="login-title">Sign in</h1>
                <p class="login-subtitle">Use the admin username and project password to enter the local platform console.</p>
              </div>
            </div>

            <form class="login-form-stack" @submit.prevent="submit">
              <div class="login-form-header">
                <p class="login-form-kicker">Dev Console</p>
                <CardDescription class="login-form-description">
                  Authenticate to open routes, commands, logs, and live agents.
                </CardDescription>
              </div>

              <div class="login-form-fields">
                <FormField label="Username">
                  <Input v-model="username" placeholder="admin" />
                </FormField>
                <FormField label="Password">
                  <div class="relative">
                    <Input
                      ref="passwordInput"
                      v-model="password"
                      :type="showPassword ? 'text' : 'password'"
                      placeholder="LIGHTHOUSE_SECRET"
                      class="pr-16"
                    />
                    <button
                      type="button"
                      class="absolute right-2 top-1/2 -translate-y-1/2 rounded-md border border-border/60 bg-muted/40 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-muted-foreground hover:text-foreground"
                      @click="showPassword = !showPassword"
                    >
                      {{ showPassword ? "Hide" : "Show" }}
                    </button>
                  </div>
                </FormField>
              </div>

              <div v-if="error" class="text-xs text-red-300">{{ error }}</div>

              <Button type="submit" variant="default" class="w-full">Sign in</Button>
            </form>
          </div>
        </div>
      </section>

      <section class="login-cover">
        <div class="login-cover-overlay">
          <div class="login-cover-copy">
            <p class="login-cover-kicker">Local Platform Control</p>
            <h2 class="login-cover-title">Run routes, queues, schedules, logs, and benchmarks from one console.</h2>
            <p class="login-cover-text">
              Bring your generated app sources together in a single operator surface without leaving local development.
            </p>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useLighthouseStore } from "../stores/lighthouse";
import Button from "../components/ui/button/Button.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/input/Input.vue";
import logoMark from "../assets/goforj-logo.png";

const router = useRouter();
const store = useLighthouseStore();

const username = ref("admin");
const password = ref("");
const error = ref("");
const showPassword = ref(false);
const passwordInput = ref<{ focus: () => void } | null>(null);

const submit = async () => {
  error.value = "";
  const res = await fetch("/lighthouse/auth", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      username: username.value,
      password: password.value,
    }),
  });
  if (!res.ok) {
    error.value = "Invalid credentials.";
    return;
  }
  await store.fetchAgents();
  await store.fetchLocal();
  store.connectSocket();
  router.replace("/");
};

onMounted(() => {
  nextTick(() => {
    passwordInput.value?.focus();
  });
});
</script>

<style scoped>
.login-shell {
  width: 100%;
  min-height: 100%;
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
    linear-gradient(180deg, rgba(8, 10, 16, 0.84), rgba(8, 10, 16, 0.9)),
    url("../assets/lighthouse-wallpaper.png");
  background-size: cover;
  background-position: 18% center;
  background-repeat: no-repeat;
  background-blend-mode: normal, screen;
  opacity: 0.5;
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
  background: color-mix(in oklab, var(--card) 74%, transparent);
  padding: 1.35rem;
  box-shadow: 0 14px 28px rgba(0, 0, 0, 0.18);
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

.login-form-title {
  font-size: 1.7rem;
  line-height: 1.02;
}

.login-form-description {
  max-width: 24rem;
  color: color-mix(in oklab, var(--foreground) 60%, transparent);
}

.login-form-fields {
  display: grid;
  gap: 1rem;
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
    linear-gradient(180deg, rgba(8, 10, 16, 0.42), rgba(8, 10, 16, 0.82)),
    url("../assets/goforj-wallpaper.webp");
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
  color: rgba(255, 255, 255, 0.78);
}

.login-cover-title {
  font-size: clamp(1.85rem, 2.85vw, 2.7rem);
  line-height: 0.96;
  font-weight: 700;
  letter-spacing: -0.05em;
  color: white;
  text-wrap: balance;
}

.login-cover-text {
  max-width: 22rem;
  font-size: 0.95rem;
  line-height: 1.6;
  color: rgba(255, 255, 255, 0.68);
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
