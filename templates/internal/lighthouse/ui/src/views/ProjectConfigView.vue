<template>
  <div><section class="space-y-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <Settings class="h-4 w-4 text-muted-foreground" />
              Project Config
            </CardTitle>
          </template>
          <template #description>
            <CardDescription>Watchers and lifecycle tasks used by <code>forj dev</code>.</CardDescription>
          </template>
          <template #action>
            <div class="flex items-center gap-2">
              <Button variant="outline" size="sm" :disabled="!dirty || saving" @click="saveConfig">
                <Save class="h-3.5 w-3.5" />
                Save Config
              </Button>
              <Button variant="outline" size="sm" :disabled="loading || saving" @click="reloadConfig">
                Reload
              </Button>
            </div>
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 flex flex-wrap items-center gap-3">
            <span v-if="dirty" class="text-xs text-amber-200/80">Unsaved changes</span>
          </div>
          <div v-if="statusMessage" class="mb-4 rounded-xl border border-border/60 px-3 py-2 text-xs" :class="statusTone">
            {{ statusMessage }}
          </div>

          <div class="mb-6 rounded-xl border border-border/60 bg-muted/20">
            <table class="w-full text-xs">
              <colgroup>
                <col class="w-[20%]" />
                <col class="w-[1%]" />
                <col />
              </colgroup>
              <thead class="border-b border-border/60 text-xs text-muted">
                <tr>
                  <th class="px-3 py-2 text-right font-medium">Setting</th>
                  <th class="px-3 py-2 text-left font-medium">Value</th>
                  <th class="px-3 py-2 text-left font-medium">Description</th>
                </tr>
              </thead>
              <tbody>
                <tr class="border-t border-border/60">
                  <td class="px-3 py-3 align-middle text-right text-foreground">Down on exit</td>
                  <td class="px-3 py-3 align-middle">
                    <Switch v-model="downOnExit" aria-label="Down on exit" />
                  </td>
                  <td class="px-3 py-3 align-middle text-muted">
                    Run <code>dev.down</code> tasks when the dev session ends. This is useful to ensure that any services started during development are properly cleaned up when you exit the dev session.
                  </td>
                </tr>
                <tr class="border-t border-border/60">
                  <td class="px-3 py-3 align-middle text-right text-foreground">Auto migrate</td>
                  <td class="px-3 py-3 align-middle">
                    <Switch v-model="autoMigrate" aria-label="Auto migrate" />
                  </td>
                  <td class="px-3 py-3 align-middle text-muted">
                    Run <code>./bin/app migrate</code> after pre-dev setup and before watchers start. This keeps database schema up to date before services boot.
                  </td>
                </tr>
                <tr class="border-t border-border/60">
                  <td class="px-3 py-3 align-middle text-right text-foreground">Sound on watcher errors</td>
                  <td class="px-3 py-3 align-middle">
                    <Switch v-model="soundOnWatchError" aria-label="Sound on watcher errors" />
                  </td>
                  <td class="px-3 py-3 align-middle text-muted">
                    Play a sound when a watcher process exits with an error. This is useful during development when you want to be notified of issues without constantly monitoring the terminal.
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="space-y-6">
            <div>
              <p class="text-xs uppercase tracking-[0.3em] text-muted">Pre-dev</p>
              <p class="mt-1 text-xs text-muted">Commands run before dev watchers start.</p>
              <div class="mt-3 space-y-2">
                <div class="rounded-xl border border-border/60 bg-muted/20">
                  <table class="w-full text-xs">
                    <colgroup>
                      <col class="w-[20%]" />
                      <col />
                      <col class="w-28" />
                    </colgroup>
                    <thead class="border-b border-border/60 text-xs text-muted">
                      <tr>
                        <th class="px-3 py-2 text-left font-medium">Name</th>
                        <th class="px-3 py-2 text-left font-medium">Command</th>
                        <th class="px-3 py-2 text-right font-medium">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="preTasks.length === 0" class="border-t border-border/60">
                        <td class="px-3 py-3 text-muted" colspan="3">No pre-dev tasks defined.</td>
                      </tr>
                      <tr
                        v-for="(task, index) in preTasks"
                        :key="task.id"
                        class="border-t border-border/60"
                      >
                        <td class="px-3 py-2 align-middle">
                          <Input v-model="task.name" placeholder="Task name" aria-label="Pre-dev task name" />
                        </td>
                        <td class="px-3 py-2 align-middle">
                          <Input v-model="task.cmd" placeholder="Command" aria-label="Pre-dev task command" />
                        </td>
                        <td class="px-3 py-2 align-middle text-right">
                          <Button
                            variant="outline"
                            size="sm"
                            class="text-xs text-red-300 hover:text-red-500"
                            @click="removePreTask(index)"
                          >
                            <Trash2 class="h-3.5 w-3.5" />
                            Remove
                          </Button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <Button variant="outline" size="sm" class="text-xs text-sky-300" @click="addPreTask">+ Add</Button>
              </div>
            </div>

            <div>
              <p class="text-xs uppercase tracking-[0.3em] text-muted">Dev Watchers</p>
              <p class="mt-1 text-xs text-muted">
                Development watchers below are sane defaults created by the project renderer during initial project setup and they all run simultaneously when you run <code>forj dev</code>.
                You can add your own watchers at any time. For flag settings, see:
              </p>
              <Button variant="outline" size="xs" class="mt-2" as-child>
                <a href="https://github.com/goforj/wgo" target="_blank" rel="noreferrer">
                  wgo flag reference
                  <ExternalLink class="h-3.5 w-3.5" />
                </a>
              </Button>

              <div class="mt-3 space-y-2">
                <div class="rounded-xl border border-border/60 bg-muted/20">
                  <table class="w-full text-xs">
                    <colgroup>
                      <col class="w-[10%]" />
                      <col class="w-[15%]" />
                      <col class="w-[75%]" />
                      <col class="w-28" />
                    </colgroup>
                    <thead class="border-b border-border/60 text-xs text-muted">
                      <tr>
                        <th class="px-3 py-2 text-left font-medium">Name</th>
                        <th class="px-3 py-2 text-left font-medium">Exec command</th>
                        <th class="px-3 py-2 text-left font-medium">Watch flags</th>
                        <th class="px-3 py-2 text-right font-medium">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="watchers.length === 0" class="border-t border-border/60">
                        <td class="px-3 py-3 text-muted" colspan="4">No watchers defined.</td>
                      </tr>
                      <tr
                        v-for="(watcher, index) in watchers"
                        :key="watcher.id"
                        class="border-t border-border/60"
                      >
                        <td class="px-3 py-2 align-middle">
                          <Input v-model="watcher.name" placeholder="e.g. API" aria-label="Watcher name" />
                        </td>
                        <td class="px-3 py-2 align-middle">
                          <Input v-model="watcher.exec" placeholder="go run ./cmd/forj/main.go serve" aria-label="Exec command" />
                        </td>
                        <td class="px-3 py-2 align-middle">
                          <Input v-model="watcher.watch" placeholder="-file .env -file .go" aria-label="Watch flags" />
                        </td>
                        <td class="px-3 py-2 align-middle text-right">
                          <Button
                            variant="outline"
                            size="sm"
                            class="text-xs text-red-300 hover:text-red-500"
                            @click="removeWatcher(index)"
                          >
                            <Trash2 class="h-3.5 w-3.5" />
                            Remove
                          </Button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <Button variant="outline" @click="addWatcher" size="sm">+ Add watcher</Button>
              </div>
            </div>

            <div>
              <p class="text-xs uppercase tracking-[0.3em] text-muted">Down</p>
              <p class="mt-1 text-xs text-muted">Commands run during cleanup.</p>
              <div class="mt-3 space-y-2">
                <div class="rounded-xl border border-border/60 bg-muted/20">
                  <table class="w-full text-xs">
                    <colgroup>
                      <col class="w-[20%]" />
                      <col />
                      <col class="w-28" />
                    </colgroup>
                    <thead class="border-b border-border/60 text-xs text-muted">
                      <tr>
                        <th class="px-3 py-2 text-left font-medium">Name</th>
                        <th class="px-3 py-2 text-left font-medium">Command</th>
                        <th class="px-3 py-2 text-right font-medium">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="downTasks.length === 0" class="border-t border-border/60">
                        <td class="px-3 py-3 text-muted" colspan="3">No down tasks defined.</td>
                      </tr>
                      <tr
                        v-for="(task, index) in downTasks"
                        :key="task.id"
                        class="border-t border-border/60"
                      >
                        <td class="px-3 py-2 align-middle">
                          <Input v-model="task.name" placeholder="Task name" aria-label="Down task name" />
                        </td>
                        <td class="px-3 py-2 align-middle">
                          <Input v-model="task.cmd" placeholder="Command" aria-label="Down task command" />
                        </td>
                        <td class="px-3 py-2 align-middle text-right">
                          <Button
                            variant="outline"
                            size="sm"
                            class="text-xs text-red-300 hover:text-red-500"
                            @click="removeDownTask(index)"
                          >
                            <Trash2 class="h-3.5 w-3.5" />
                            Remove
                          </Button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <Button variant="outline" size="sm" class="text-xs text-sky-300" @click="addDownTask">+ Add</Button>
              </div>
            </div>
          </div>
          <div class="mt-6 flex items-center justify-between rounded-xl border border-border/60 bg-muted/20 px-3 py-2">
            <span class="text-xs text-muted">Save your changes to apply them to <code>forj dev</code>.</span>
            <div class="flex items-center gap-2">
              <Button variant="outline" size="sm" :disabled="!dirty || saving" @click="saveConfig">
                <Save class="h-3.5 w-3.5" />
                Save Config
              </Button>
              <Button variant="outline" size="sm" :disabled="loading || saving" @click="reloadConfig">
                Reload
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ExternalLink, Save, Settings, Trash2 } from "lucide-vue-next";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import Textarea from "../components/ui/form/Textarea.vue";
import Switch from "../components/ui/form/Switch.vue";

type DevWatch = {
  name?: string;
  watch?: string;
  exec?: string;
};

type DevTask = {
  name?: string;
  cmd?: string;
};

type ProjectConfigResponse = {
  dev?: {
    pre?: DevTask[];
    down?: DevTask[];
    auto_migrate?: boolean;
    down_on_exit?: boolean;
    sound_on_watch_error?: boolean;
    watches?: DevWatch[];
  };
};

type EditableWatch = {
  id: number;
  name: string;
  watch: string;
  exec: string;
};

type EditableTask = {
  id: number;
  name: string;
  cmd: string;
};

const downOnExit = ref(false);
const autoMigrate = ref(false);
const soundOnWatchError = ref(false);
const statusMessage = ref("");
const statusTone = ref("text-muted");
const loading = ref(false);
const saving = ref(false);
const nextWatcherId = ref(1);
const nextTaskId = ref(1);
const snapshot = ref("");
const watchers = ref<EditableWatch[]>([]);
const preTasks = ref<EditableTask[]>([]);
const downTasks = ref<EditableTask[]>([]);

const createWatcher = (data: DevWatch = {}) => ({
  id: nextWatcherId.value++,
  name: data.name || "",
  watch: data.watch || "",
  exec: data.exec || "",
});

const createTask = (data: DevTask = {}) => ({
  id: nextTaskId.value++,
  name: data.name || "",
  cmd: data.cmd || "",
});

const buildSnapshot = () =>
  JSON.stringify({
    watchers: watchers.value.map((watcher) => ({
      name: watcher.name,
      watch: watcher.watch,
      exec: watcher.exec,
    })),
    pre: preTasks.value.map((task) => ({ name: task.name, cmd: task.cmd })),
    down: downTasks.value.map((task) => ({ name: task.name, cmd: task.cmd })),
    autoMigrate: autoMigrate.value,
    downOnExit: downOnExit.value,
    soundOnWatchError: soundOnWatchError.value,
  });

const dirty = computed(() => snapshot.value !== buildSnapshot());

const loadConfig = async (options: { skipStatusReset?: boolean } = {}) => {
  if (!options.skipStatusReset) {
    statusMessage.value = "";
    statusTone.value = "text-muted";
  }
  loading.value = true;
  try {
    const res = await fetch("/__lighthouse/api/goforj");
    if (!res.ok) {
      statusMessage.value = "Unable to load project configuration.";
      statusTone.value = "text-red-300/80";
      return;
    }
    const payload = (await res.json()) as ProjectConfigResponse;
    autoMigrate.value = payload.dev?.auto_migrate ?? false;
    downOnExit.value = payload.dev?.down_on_exit ?? false;
    soundOnWatchError.value = payload.dev?.sound_on_watch_error ?? false;
    nextWatcherId.value = 1;
    watchers.value = (payload.dev?.watches || []).map((watch) => createWatcher(watch));
    nextTaskId.value = 1;
    preTasks.value = (payload.dev?.pre || []).map((task) => createTask(task));
    downTasks.value = (payload.dev?.down || []).map((task) => createTask(task));
    snapshot.value = buildSnapshot();
  } catch (error) {
    statusMessage.value = "Unable to load project configuration.";
    statusTone.value = "text-red-300/80";
  } finally {
    loading.value = false;
  }
};

const reloadConfig = () => loadConfig();

const saveConfig = async () => {
  if (!dirty.value) {
    return;
  }
  saving.value = true;
  statusMessage.value = "Saving...";
  statusTone.value = "text-sky-200/80";
  const payload: ProjectConfigResponse = {
    dev: {
      pre: preTasks.value.map((task) => ({ name: task.name, cmd: task.cmd })),
      down: downTasks.value.map((task) => ({ name: task.name, cmd: task.cmd })),
      auto_migrate: autoMigrate.value,
      down_on_exit: downOnExit.value,
      sound_on_watch_error: soundOnWatchError.value,
      watches: watchers.value.map((watcher) => ({
        name: watcher.name,
        watch: watcher.watch,
        exec: watcher.exec,
      })),
    },
  };
  try {
    const res = await fetch("/__lighthouse/api/goforj", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      statusMessage.value = "Failed to save project configuration.";
      statusTone.value = "text-red-300/80";
      return;
    }
    statusMessage.value = "Saved.";
    statusTone.value = "text-emerald-200/80";
    await loadConfig({ skipStatusReset: true });
    window.setTimeout(() => {
      if (statusMessage.value === "Saved.") {
        statusMessage.value = "";
        statusTone.value = "text-muted";
      }
    }, 2500);
  } finally {
    saving.value = false;
  }
};

const addWatcher = () => {
  watchers.value.push(createWatcher());
};

const removeWatcher = (index: number) => {
  watchers.value.splice(index, 1);
};

const addPreTask = () => {
  preTasks.value.push(createTask());
};

const addDownTask = () => {
  downTasks.value.push(createTask());
};

const removePreTask = (index: number) => {
  preTasks.value.splice(index, 1);
};

const removeDownTask = (index: number) => {
  downTasks.value.splice(index, 1);
};

onMounted(() => {
  loadConfig();
});
</script>
