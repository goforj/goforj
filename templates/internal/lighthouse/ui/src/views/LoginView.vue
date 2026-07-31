<template>
  <div class="login-shell">
    <main class="login-panel">
      <div class="login-brand">
        <LighthouseMark class="login-brand-mark" />
        <span class="sr-only">GoForj Lighthouse</span>
      </div>

      <div class="login-heading">
        <h1>Log in to Lighthouse</h1>
        <p>Enter the project admin credentials to open the local development console.</p>
      </div>

      <form class="login-form" @submit.prevent="submit">
        <FormField label="Username">
          <Input
            v-model="username"
            autocomplete="username"
            placeholder="admin"
          />
        </FormField>

        <FormField label="Password">
          <div class="relative">
            <Input
              ref="passwordInput"
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
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

        <p
          v-if="error"
          class="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive"
        >
          {{ error }}
        </p>

        <Button type="submit" variant="default" class="mt-2 w-full">
          Log in
        </Button>
      </form>
    </main>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useLighthouseStore } from "../stores/lighthouse";
import Button from "../components/ui/button/Button.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/input/Input.vue";
import LighthouseMark from "../components/LighthouseMark.vue";

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
  position: relative;
  display: flex;
  min-height: 100%;
  width: 100%;
  flex: 1;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  padding: 1.5rem;
  background:
    linear-gradient(180deg, rgba(12, 10, 14, 0.62), rgba(12, 10, 14, 0.8)),
    url("../assets/lighthouse-wallpaper.png");
  background-position: center;
  background-repeat: no-repeat;
  background-size: cover;
  animation: loginFadeDown 420ms ease-out both;
}

.login-shell::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background:
    radial-gradient(circle at 50% 44%, transparent 0 12rem, rgba(12, 10, 14, 0.18) 34rem),
    linear-gradient(90deg, rgba(12, 10, 14, 0.16), transparent 36%, rgba(12, 10, 14, 0.12));
}

.login-panel {
  position: relative;
  z-index: 1;
  width: min(100%, 25rem);
  border: 1px solid color-mix(in oklab, var(--border) 78%, transparent);
  border-radius: 1.15rem;
  padding: 2rem;
  background: color-mix(in oklab, var(--card) 88%, transparent);
  box-shadow:
    0 28px 80px rgba(0, 0, 0, 0.42),
    inset 0 1px 0 rgba(255, 255, 255, 0.055);
  backdrop-filter: blur(18px) saturate(0.86);
}

.login-brand {
  display: flex;
  justify-content: center;
}

.login-brand-mark {
  width: 4rem;
  height: 4rem;
  filter: drop-shadow(0 12px 28px rgba(0, 0, 0, 0.34));
}

.login-heading {
  margin-top: 1.1rem;
  text-align: center;
}

.login-heading h1 {
  font-size: 1.45rem;
  font-weight: 650;
  letter-spacing: -0.035em;
  color: var(--foreground);
}

.login-heading p {
  margin: 0.45rem auto 0;
  max-width: 21rem;
  font-size: 0.875rem;
  line-height: 1.55;
  color: var(--muted-foreground);
}

.login-form {
  display: grid;
  gap: 1.15rem;
  margin-top: 1.75rem;
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

@media (max-width: 520px) {
  .login-shell {
    padding: 1rem;
    background-position: 38% center;
  }

  .login-panel {
    padding: 1.5rem;
  }
}
</style>
