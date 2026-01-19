<template>
  <div>
    <PageHeader label="Platform" title="Project Config">
      <template #right>
        <AgentPills />
        <LivePill />
      </template>
    </PageHeader>

    <section class="mt-8 space-y-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Application</p>
            <CardTitle>Edit .goforj.yml</CardTitle>
          </template>
          <template #description>
            <CardDescription>Project metadata, watchers, and components that drive your dev flow.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 flex flex-wrap items-center gap-3">
            <Button :disabled="!dirty || saving" @click="saveConfig">Save Config</Button>
            <Button variant="outline" size="sm" :disabled="loading || saving" @click="reloadConfig">
              Reload
            </Button>
            <span v-if="dirty" class="text-xs text-amber-200/80">Unsaved changes</span>
          </div>
          <div v-if="statusMessage" class="mb-4 rounded-xl border border-border/70 px-3 py-2 text-xs" :class="statusTone">
            {{ statusMessage }}
          </div>

          <div class="grid gap-4 lg:grid-cols-2">
            <FormField label="Project Name">
              <Input
                v-model="projectName"
                placeholder="Your project name"
              />
            </FormField>
            <FormField label="Go Module">
              <Input
                v-model="moduleName"
                placeholder="github.com/you/your-app"
              />
            </FormField>
          </div>

          <div class="mt-4 grid gap-4 md:grid-cols-2">
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Updated</p>
            <p class="text-xs text-muted">{{ updatedAt || "Not recorded yet" }}</p>
          </div>

          <div class="mt-5 flex flex-wrap gap-4 text-xs">
            <Switch v-model="downOnExit">Down on exit</Switch>
            <Switch v-model="soundOnWatchError">Sound on watcher errors</Switch>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Components</p>
            <CardTitle>Toggle project features</CardTitle>
          </template>
          <template #description>
            <CardDescription>Flip the switches for the services your project scaffolds.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <Switch
              v-for="option in componentOptions"
              :key="option.key"
              v-model="components[option.key]"
              class="rounded-lg border border-border/70 px-3 py-2 text-xs text-white"
            >
              {{ option.label }}
            </Switch>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Dev Watchers</p>
            <CardTitle>Define wgo watchers</CardTitle>
          </template>
          <template #description>
            <CardDescription>Each watcher is spun up via <code>forj dev</code>.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="space-y-3">
            <div
              v-for="(watcher, index) in watchers"
              :key="watcher.id"
              class="rounded-xl border border-border/70 bg-slate-900/10 px-3 py-3 space-y-3"
            >
                <div class="mb-1 flex items-center justify-between">
                <span class="text-xs font-semibold uppercase tracking-[0.3em] text-muted">
                  Watcher {{ index + 1 }}
                </span>
                <Button variant="outline" size="sm" class="text-xs text-red-300 hover:text-red-500" @click="removeWatcher(index)">
                  Remove
                </Button>
              </div>
              <div class="space-y-2 text-xs">
                <div class="flex items-center gap-3">
                  <span class="w-24 text-right text-[11px] uppercase tracking-[0.3em] text-muted">Name</span>
                  <Input
                    v-model="watcher.name"
                    placeholder="e.g. API"
                    class="flex-1"
                  />
                </div>
                <div class="flex items-center gap-3">
                  <span class="w-24 text-right text-[11px] uppercase tracking-[0.3em] text-muted">Watch Flags</span>
                  <Textarea
                    v-model="watcher.watch"
                    placeholder="-file .env -file .go"
                    class="flex-1"
                    rows="1"
                  />
                </div>
                <div class="flex items-center gap-3">
                  <span class="w-24 text-right text-[11px] uppercase tracking-[0.3em] text-muted">Exec Command</span>
                  <Textarea
                    v-model="watcher.exec"
                    placeholder="go run ./cmd/forj/main.go serve"
                    class="flex-1"
                    rows="1"
                  />
                </div>
              </div>
            </div>
          <Button variant="outline" @click="addWatcher" size="sm">+ Add watcher</Button>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Tasks</p>
            <CardTitle>Pre / Down tasks</CardTitle>
          </template>
          <template #description>
            <CardDescription>Run commands before dev mode or when cleaning up.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="grid gap-6 lg:grid-cols-2">
            <div>
              <div class="mb-3 flex items-center justify-between">
                <h3 class="text-sm font-semibold text-white">Pre-dev</h3>
                <Button variant="outline" size="sm" class="text-xs text-sky-300" @click="addPreTask">+ Add</Button>
              </div>
              <div class="space-y-3">
                <div
                  v-for="(task, index) in preTasks"
                  :key="task.id"
                  class="rounded-xl border border-border/70 bg-slate-900/30 p-3"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-xs text-muted">Task {{ index + 1 }}</span>
                  <Button variant="outline" size="sm" class="text-[11px] text-red-300 hover:text-red-500" @click="removePreTask(index)">
                    Remove
                  </Button>
                  </div>
                  <FormField label="Name">
                    <Input v-model="task.name" />
                  </FormField>
                  <FormField label="Command">
                    <Textarea rows="2" v-model="task.cmd" />
                  </FormField>
                </div>
              </div>
            </div>
            <div>
              <div class="mb-3 flex items-center justify-between">
                <h3 class="text-sm font-semibold text-white">Down</h3>
                <Button variant="outline" size="sm" class="text-xs text-sky-300" @click="addDownTask">+ Add</Button>
              </div>
              <div class="space-y-3">
                <div
                  v-for="(task, index) in downTasks"
                  :key="task.id"
                  class="rounded-xl border border-border/70 bg-slate-900/30 p-3"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-xs text-muted">Task {{ index + 1 }}</span>
                  <Button variant="outline" size="sm" class="text-[11px] text-red-300 hover:text-red-500" @click="removeDownTask(index)">
                    Remove
                  </Button>
                  </div>
                  <FormField label="Name">
                    <Input v-model="task.name" />
                  </FormField>
                  <FormField label="Command">
                    <Textarea rows="2" v-model="task.cmd" />
                  </FormField>
                </div>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import AgentPills from "../components/AgentPills.vue";
import LivePill from "../components/LivePill.vue";
import PageHeader from "../components/PageHeader.vue";
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

type ComponentKey =
  | "CLI"
  | "WebAPI"
  | "WebUI"
  | "Docker"
  | "DatabaseMySQL"
  | "DatabasePostgres"
  | "DatabaseSQLite"
  | "Scheduler"
  | "Jobs";

type ComponentsState = Record<ComponentKey, boolean>;

type ProjectConfigResponse = {
  project_name?: string;
  module_name?: string;
  updated_at?: string;
  dev?: {
    pre?: DevTask[];
    down?: DevTask[];
    down_on_exit?: boolean;
    sound_on_watch_error?: boolean;
    watches?: DevWatch[];
  };
  components?: Partial<Record<ComponentKey, boolean>>;
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

const projectName = ref("");
const moduleName = ref("");
const updatedAt = ref("");
const downOnExit = ref(false);
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
const components = reactive<ComponentsState>({
  CLI: false,
  WebAPI: false,
  WebUI: false,
  Docker: false,
  DatabaseMySQL: false,
  DatabasePostgres: false,
  DatabaseSQLite: false,
  Scheduler: false,
  Jobs: false,
});

const componentOptions: { key: ComponentKey; label: string }[] = [
  { key: "CLI", label: "CLI" },
  { key: "WebAPI", label: "Web API" },
  { key: "WebUI", label: "Web UI" },
  { key: "Docker", label: "Docker" },
  { key: "DatabaseMySQL", label: "MySQL" },
  { key: "DatabasePostgres", label: "Postgres" },
  { key: "DatabaseSQLite", label: "SQLite" },
  { key: "Scheduler", label: "Scheduler" },
  { key: "Jobs", label: "Jobs" },
];

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
    projectName: projectName.value,
    moduleName: moduleName.value,
    watchers: watchers.value.map((watcher) => ({
      name: watcher.name,
      watch: watcher.watch,
      exec: watcher.exec,
    })),
    pre: preTasks.value.map((task) => ({ name: task.name, cmd: task.cmd })),
    down: downTasks.value.map((task) => ({ name: task.name, cmd: task.cmd })),
    downOnExit: downOnExit.value,
    soundOnWatchError: soundOnWatchError.value,
    components: componentOptions.reduce<Record<ComponentKey, boolean>>((acc, option) => {
      acc[option.key] = components[option.key];
      return acc;
    }, {} as Record<ComponentKey, boolean>),
  });

const dirty = computed(() => snapshot.value !== buildSnapshot());

const populateComponents = (source?: Partial<Record<ComponentKey, boolean>>) => {
  componentOptions.forEach((option) => {
    components[option.key] = source?.[option.key] ?? false;
  });
};

const loadConfig = async (options: { skipStatusReset?: boolean } = {}) => {
  if (!options.skipStatusReset) {
    statusMessage.value = "";
    statusTone.value = "text-muted";
  }
  loading.value = true;
  try {
    const res = await fetch("/__devconsole/api/goforj");
    if (!res.ok) {
      statusMessage.value = "Unable to load project configuration.";
      statusTone.value = "text-red-300/80";
      return;
    }
    const payload = (await res.json()) as ProjectConfigResponse;
    projectName.value = payload.project_name || "";
    moduleName.value = payload.module_name || "";
    updatedAt.value = payload.updated_at || "";
    downOnExit.value = payload.dev?.down_on_exit ?? false;
    soundOnWatchError.value = payload.dev?.sound_on_watch_error ?? false;
    nextWatcherId.value = 1;
    watchers.value = (payload.dev?.watches || []).map((watch) => createWatcher(watch));
    nextTaskId.value = 1;
    preTasks.value = (payload.dev?.pre || []).map((task) => createTask(task));
    downTasks.value = (payload.dev?.down || []).map((task) => createTask(task));
    populateComponents(payload.components);
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
    project_name: projectName.value,
    module_name: moduleName.value,
    dev: {
      pre: preTasks.value.map((task) => ({ name: task.name, cmd: task.cmd })),
      down: downTasks.value.map((task) => ({ name: task.name, cmd: task.cmd })),
      down_on_exit: downOnExit.value,
      sound_on_watch_error: soundOnWatchError.value,
      watches: watchers.value.map((watcher) => ({
        name: watcher.name,
        watch: watcher.watch,
        exec: watcher.exec,
      })),
    },
    components: componentOptions.reduce<Record<ComponentKey, boolean>>((acc, option) => {
      acc[option.key] = components[option.key];
      return acc;
    }, {} as Record<ComponentKey, boolean>),
  };
  try {
    const res = await fetch("/__devconsole/api/goforj", {
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
