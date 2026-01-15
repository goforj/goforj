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
          <div>
            <label class="text-xs text-muted">Username</label>
            <input
              v-model="username"
              class="mt-2 w-full rounded-xl border border-border/70 bg-white/5 px-3 py-2 text-sm text-white"
              placeholder="admin"
            />
          </div>
          <div>
            <label class="text-xs text-muted">Token</label>
            <div class="relative mt-2">
              <input
                v-model="token"
                :type="showToken ? 'text' : 'password'"
                class="w-full rounded-xl border border-border/70 bg-white/5 px-3 py-2 pr-16 text-sm text-white"
                placeholder="DEVCONSOLE_TOKEN"
              />
              <button
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 rounded-md border border-border/70 bg-white/5 px-2 py-1 text-[10px] uppercase tracking-[0.2em] text-muted hover:text-white"
                @click="showToken = !showToken"
              >
                {{ showToken ? "Hide" : "Show" }}
              </button>
            </div>
          </div>
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
