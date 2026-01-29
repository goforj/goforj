<template>
  <div class="w-full max-w-lg login-intro">
    <img :src="logoFull" alt="GoForj" class="login-logo" />

    <div class="login-form-wrap">
      <Card class="card-texture login-form">
      <CardHeader>
        <template #title>
          <div>
            <p class="mt-2 text-xs uppercase tracking-[0.3em] text-muted">Dev Console</p>
            <CardTitle>Sign in</CardTitle>
          </div>
        </template>
        <template #description>
          <CardDescription>Use the admin username and project token.</CardDescription>
        </template>
      </CardHeader>
      <CardContent>
        <form class="space-y-4" @submit.prevent="submit">
          <FormField label="Username">
            <Input v-model="username" placeholder="admin" />
          </FormField>
          <FormField label="Token">
            <div class="relative">
              <Input
                v-model="token"
                :type="showToken ? 'text' : 'password'"
                placeholder="DEVCONSOLE_TOKEN"
                class="pr-16"
              />
              <button
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 rounded-md border border-border/60 bg-muted/40 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-muted-foreground hover:text-foreground"
                @click="showToken = !showToken"
              >
                {{ showToken ? "Hide" : "Show" }}
              </button>
            </div>
          </FormField>
          <div v-if="error" class="text-xs text-red-300">{{ error }}</div>
          <Button type="submit" variant="default" class="w-full">Sign in</Button>
        </form>
      </CardContent>
    </Card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useDevconsoleStore } from "../stores/devconsole";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import logoFull from "../assets/goforj-full.png";

const router = useRouter();
const store = useDevconsoleStore();

const username = ref("admin");
const token = ref("");
const error = ref("");
const showToken = ref(false);

const submit = async () => {
  error.value = "";
  const res = await fetch("/__devconsole/auth", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      username: username.value,
      token: token.value,
    }),
  });
  if (!res.ok) {
    error.value = "Invalid credentials.";
    return;
  }
  await store.fetchAgents();
  store.connectSocket();
  router.replace("/");
};
</script>

<style scoped>
.login-intro {
  position: relative;
  animation: loginFadeDown 420ms ease-out both;
}

.login-logo {
  position: absolute;
  left: 50%;
  top: -130px;
  transform: translateX(-50%);
  height: 140px;
  animation: loginLogoDown 420ms ease-out 40ms both;
}

.login-form-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 360px;
}

.login-form {
  width: 100%;
  animation: loginFadeDown 420ms ease-out 80ms both;
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

@keyframes loginLogoDown {
  from {
    opacity: 0;
    transform: translateX(-50%) translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateX(-50%) translateY(0);
  }
}
</style>
