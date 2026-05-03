<template>
  <div><section class="grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <FileText class="h-4 w-4 text-muted-foreground" />
              Env
            </CardTitle>
          </template>
          <template #description>
            <CardDescription>Edits apply immediately; your dev watcher reloads immediately.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 flex flex-wrap items-center gap-3 text-xs">
            <div class="min-w-[220px] flex-1">
              <FormField label="Env file">
                <Select v-model="selectedModel">
                  <SelectTrigger class="w-full">
                    <SelectValue placeholder="Select file" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem :value="emptySelectValue">Select file</SelectItem>
                    <SelectItem v-for="file in files" :key="file" :value="file">{{ file }}</SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
            </div>
            <Button variant="secondary" :disabled="!selected || loading" @click="reloadFile">Reload</Button>
            <Button variant="default" :disabled="!dirty || saving" @click="saveFile">Save</Button>
            <span v-if="dirty && !statusMessage" class="text-xs text-amber-200/80">Unsaved changes</span>
          </div>

          <div v-if="statusMessage" class="mb-3 rounded-xl border border-border/60 px-3 py-2 text-xs" :class="statusTone">
            {{ statusMessage }}
          </div>
          <FormField label="Content">
            <Textarea
              v-model="content"
              rows="24"
              class="h-[65vh] max-h-[65vh] min-h-[28rem]"
              placeholder="Select an env file to edit..."
            />
          </FormField>
        </CardContent>
      </Card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useLighthouseStore } from "../stores/lighthouse";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import Textarea from "../components/ui/textarea/Textarea.vue";
import { FileText } from "lucide-vue-next";

const files = ref<string[]>([]);
const store = useLighthouseStore();
const emptySelectValue = "__empty__";
const selected = ref("");
const content = ref("");
const original = ref("");
const statusMessage = ref("");
const statusTone = ref("text-muted");
const loading = ref(false);
const saving = ref(false);

const dirty = computed(() => content.value !== original.value);

const selectedModel = computed({
  get: () => selected.value || emptySelectValue,
  set: (value: string) => {
    selected.value = value === emptySelectValue ? "" : value;
  },
});

const refreshFiles = async () => {
  statusMessage.value = "";
  statusTone.value = "text-muted";
  loading.value = true;
  try {
    const res = await fetch("/lighthouse/api/env");
    if (!res.ok) {
      statusMessage.value = "Unable to load env files.";
      statusTone.value = "text-red-300/80";
      return;
    }
    const payload = (await res.json()) as { files: string[] };
    files.value = payload.files || [];
    if (!selected.value && files.value.length > 0) {
      selected.value = files.value[0];
      await loadSelected();
    }
  } finally {
    loading.value = false;
  }
};

const loadSelected = async () => {
  if (!selected.value) {
    content.value = "";
    original.value = "";
    return;
  }
  statusMessage.value = "";
  statusTone.value = "text-muted";
  loading.value = true;
  try {
    const res = await fetch(`/lighthouse/api/env?file=${encodeURIComponent(selected.value)}`);
    if (!res.ok) {
      statusMessage.value = "Unable to load file.";
      statusTone.value = "text-red-300/80";
      return;
    }
    const payload = (await res.json()) as { content: string };
    content.value = payload.content || "";
    original.value = payload.content || "";
  } finally {
    loading.value = false;
  }
};

const reloadFile = async () => {
  await loadSelected();
};

const saveFile = async () => {
  if (!selected.value) return;
  statusMessage.value = "Saving...";
  statusTone.value = "text-sky-200/80";
  saving.value = true;
  try {
    const res = await fetch(`/lighthouse/api/env?file=${encodeURIComponent(selected.value)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content: content.value }),
    });
    if (!res.ok) {
      statusMessage.value = "Failed to save file.";
      statusTone.value = "text-red-300/80";
      return;
    }
    original.value = content.value;
    statusMessage.value = "Saved.";
    statusTone.value = "text-emerald-200/80";
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

onMounted(refreshFiles);

watch(selected, async (value, previous) => {
  if (value === previous) {
    return;
  }
  if (!value) {
    content.value = "";
    original.value = "";
    return;
  }
  await loadSelected();
});
</script>
