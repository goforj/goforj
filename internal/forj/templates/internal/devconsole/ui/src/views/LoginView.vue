<template>
  <div class="w-full max-w-md">
    <Card class="card-texture">
      <CardHeader>
        <template #title>
          <div>
            <img :src="logoFull" alt="GoForj" class="h-10" />
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
                class="absolute right-2 top-1/2 -translate-y-1/2 rounded-md border border-border/70 bg-white/5 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-muted hover:text-white"
                @click="showToken = !showToken"
              >
                {{ showToken ? "Hide" : "Show" }}
              </button>
            </div>
          </FormField>
          <div v-if="error" class="text-xs text-red-300">{{ error }}</div>
          <Button type="submit" class="w-full">Sign in</Button>
        </form>
      </CardContent>
    </Card>
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
