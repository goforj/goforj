<template>
  <div class="relative" ref="root">
    <button
      type="button"
      class="inline-flex items-center gap-2 rounded-md border border-input bg-background px-3 py-1.5 text-sm font-medium text-foreground shadow-sm transition hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      @click="toggle"
      aria-haspopup="dialog"
      :aria-expanded="open"
    >
      <Palette class="h-4 w-4" />
      <span class="hidden sm:inline">Themes</span>
    </button>
    <div
      v-if="open"
      class="theme-selector-panel"
      role="dialog"
      aria-label="Theme selector"
    >
      <div class="flex items-center justify-between gap-3">
        <div>
          <p class="text-sm font-semibold text-foreground">Theme presets</p>
          <p class="text-xs text-muted-foreground">Pick a look for the admin panel.</p>
        </div>
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-input bg-background text-muted-foreground transition hover:text-foreground"
          @click="open = false"
          aria-label="Close"
        >
          <X class="h-4 w-4" />
        </button>
      </div>
      <div class="mt-4 grid gap-3 sm:grid-cols-2">
        <button
          v-for="theme in themes"
          :key="theme.id"
          type="button"
          class="theme-preview-card"
          :class="theme.id === modelValue ? 'theme-preview-card-active' : ''"
          @click="select(theme.id)"
        >
          <div class="theme-preview dark" :data-theme="theme.id">
            <div class="theme-preview-sidebar">
              <div class="theme-preview-pill"></div>
              <div class="theme-preview-line"></div>
              <div class="theme-preview-line short"></div>
              <div class="theme-preview-line"></div>
            </div>
            <div class="theme-preview-main">
              <div class="theme-preview-header">
                <div class="theme-preview-dot"></div>
                <div class="theme-preview-dot"></div>
                <div class="theme-preview-dot"></div>
              </div>
              <div class="theme-preview-card-surface">
                <div class="theme-preview-title"></div>
                <div class="theme-preview-row"></div>
                <div class="theme-preview-row short"></div>
              </div>
            </div>
          </div>
          <div class="mt-3 text-left">
            <p class="text-sm font-semibold text-foreground">{{ theme.name }}</p>
            <p class="text-xs text-muted-foreground">{{ theme.description }}</p>
          </div>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { Palette, X } from "lucide-vue-next";

const props = defineProps<{
  modelValue: string;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", value: string): void;
}>();

const themes = [
  {
    id: "default",
    name: "Default Dark",
    description: "Base admin styling with no theme overrides.",
  },
  {
    id: "discord",
    name: "Discord Dark",
    description: "Soft panels, deep inputs, familiar contrast.",
  },
  {
    id: "vitepress",
    name: "VitePress Dark",
    description: "Quiet contrast with editorial depth.",
  },
];

const open = ref(false);
const root = ref<HTMLElement | null>(null);

const toggle = () => {
  open.value = !open.value;
};

const select = (id: string) => {
  emit("update:modelValue", id);
  open.value = false;
};

const handleClick = (event: MouseEvent) => {
  if (!open.value) return;
  if (!root.value) return;
  if (!root.value.contains(event.target as Node)) {
    open.value = false;
  }
};

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === "Escape") {
    open.value = false;
  }
};

onMounted(() => {
  window.addEventListener("click", handleClick);
  window.addEventListener("keydown", handleKeydown);
});

onBeforeUnmount(() => {
  window.removeEventListener("click", handleClick);
  window.removeEventListener("keydown", handleKeydown);
});
</script>
