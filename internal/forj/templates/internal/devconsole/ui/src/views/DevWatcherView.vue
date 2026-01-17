<template>
  <div>
    <PageHeader label="Platform" title="Dev Watcher">
      <template #right>
        <Button :disabled="!devwatchConnected" @click="restart">
          Restart Watchers
        </Button>
        <AgentPills />
        <LivePill />
      </template>
    </PageHeader>

    <section class="mt-8">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Watcher Output</p>
            <CardTitle>Streaming `forj dev` output.</CardTitle>
          </template>
          <template #description>
            <CardDescription>Matches the stdout/stderr you see in the dev watcher.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div ref="terminalRef" class="terminal-pane">
            <div v-if="lines.length === 0" class="text-xs text-muted">Waiting for watcher output…</div>
            <div v-else class="terminal-lines">
              <div v-for="(line, idx) in lines" :key="idx" class="terminal-line" v-html="line" />
            </div>
            <div v-if="!followTail" class="terminal-follow-wrap">
              <button class="terminal-follow" @click="resumeFollow">
                Continue Watch
              </button>
            </div>
          </div>
        </CardContent>
      </Card>
    </section>

  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useDevconsoleStore } from "../stores/devconsole";
import { ansiToHtml } from "../lib/ansi";
import { toast } from "vue-sonner";
import AgentPills from "../components/AgentPills.vue";
import LivePill from "../components/LivePill.vue";
import PageHeader from "../components/PageHeader.vue";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";

const store = useDevconsoleStore();
const terminalRef = ref<HTMLElement | null>(null);
const devwatchConnected = computed(() => store.state.devwatchConnected);
const lines = computed(() =>
  store.state.devwatch.map((entry) => ansiToHtml(entry.line || ""))
);
const followTail = ref(true);
const restart = () => {
  store.sendDevwatchControl("restart");
  window.setTimeout(() => {
    store.disconnectDevwatch();
    store.connectDevwatch();
  }, 300);
  toast("Restart request sent", {
    description: "Dev watchers are restarting.",
  });
};
const scrollToBottom = () => {
  const el = terminalRef.value;
  if (!el) return;
  el.scrollTop = el.scrollHeight;
};

const handleScroll = () => {
  const el = terminalRef.value;
  if (!el) return;
  const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
  followTail.value = nearBottom;
};

watch(
  () => lines.value.length,
  async () => {
    await nextTick();
    if (followTail.value) {
      requestAnimationFrame(scrollToBottom);
    }
  }
);

onMounted(() => {
  store.connectDevwatch();
  nextTick(() => {
    const el = terminalRef.value;
    if (el) {
      el.addEventListener("scroll", handleScroll, { passive: true });
    }
  });
});

onUnmounted(() => {
  const el = terminalRef.value;
  if (el) {
    el.removeEventListener("scroll", handleScroll);
  }
  store.disconnectDevwatch();
});

const resumeFollow = () => {
  followTail.value = true;
  requestAnimationFrame(scrollToBottom);
};
</script>
