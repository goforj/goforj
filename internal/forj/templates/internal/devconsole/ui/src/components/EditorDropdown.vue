<template>
  <div v-if="isVisible" class="editor-dropdown relative inline-flex">
    <button
      class="flex items-center gap-1 rounded-md border border-border bg-muted/60 px-2 py-1 text-[10px] text-muted-foreground transition active:scale-95 active:bg-muted"
      type="button"
      :aria-expanded="open"
      @click.stop="toggle"
      ref="buttonRef"
    >
      <Code2 class="h-3.5 w-3.5" />
      <span>{{ label }}</span>
      <ChevronDown class="h-3.5 w-3.5" />
    </button>
    <Teleport to="body">
      <div
        v-if="open"
        class="fixed z-50 min-w-[140px] rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md"
        :style="menuStyle"
      >
        <button
          class="flex w-full items-center gap-2 rounded-sm px-2 py-1 text-left text-xs text-muted-foreground hover:bg-muted"
          type="button"
          @click="openIn('vscode')"
        >
          <Code2 class="h-3.5 w-3.5" />
          VS Code
        </button>
        <button
          class="flex w-full items-center gap-2 rounded-sm px-2 py-1 text-left text-xs text-muted-foreground hover:bg-muted"
          type="button"
          @click="openIn('goland')"
        >
          <Rocket class="h-3.5 w-3.5" />
          GoLand
        </button>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from "vue";
import { toast } from "vue-sonner";
import { Code2, ChevronDown, Rocket } from "lucide-vue-next";
import { useDevconsoleStore } from "../stores/devconsole";
import type { EditorTarget } from "../stores/devconsole";

const props = withDefaults(
  defineProps<{
    symbol?: string;
    path?: string;
    line?: number;
    label?: string;
    localOnly?: boolean;
  }>(),
  {
    label: "Editor",
    localOnly: true,
  }
);

const store = useDevconsoleStore();
const open = ref(false);
const buttonRef = ref<HTMLElement | null>(null);
const menuStyle = ref<Record<string, string>>({});

const hasTarget = computed(() => Boolean(props.symbol || props.path));
const isVisible = computed(() =>
  props.localOnly ? store.state.localClient && hasTarget.value : hasTarget.value
);

const close = () => {
  open.value = false;
};

const updatePosition = () => {
  const button = buttonRef.value;
  if (!button) return;
  const rect = button.getBoundingClientRect();
  menuStyle.value = {
    top: `${rect.bottom + 8}px`,
    left: `${Math.max(8, rect.right - 160)}px`,
  };
};

const toggle = async () => {
  open.value = !open.value;
  if (open.value) {
    await nextTick();
    updatePosition();
  }
};

const openIn = async (target: EditorTarget) => {
  if (!hasTarget.value) return;
  try {
    await store.openEditor({
      target,
      symbol: props.symbol,
      path: props.path,
      line: props.line,
    });
    toast(`Opened in ${target === "goland" ? "GoLand" : "VS Code"}`);
  } catch (error: any) {
    const message = error?.message || "Unable to open editor.";
    toast(`Failed to open ${target === "goland" ? "GoLand" : "VS Code"}: ${message}`);
  } finally {
    close();
  }
};

const handleWindowClick = (event: MouseEvent) => {
  if (!open.value) return;
  const target = event.target as HTMLElement | null;
  if (!target) return;
  if (target.closest(".editor-dropdown")) {
    return;
  }
  close();
};

window.addEventListener("click", handleWindowClick);
window.addEventListener("resize", updatePosition);
window.addEventListener("scroll", updatePosition, true);
onBeforeUnmount(() => {
  window.removeEventListener("click", handleWindowClick);
  window.removeEventListener("resize", updatePosition);
  window.removeEventListener("scroll", updatePosition, true);
});
</script>
