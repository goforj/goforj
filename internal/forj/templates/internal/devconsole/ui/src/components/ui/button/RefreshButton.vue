<template>
  <Button :variant="variant" :size="size" :disabled="disabled || refreshing" @click="handleClick">
    <RefreshCw class="mr-1 h-3.5 w-3.5" :class="refreshing ? 'animate-spin' : ''" />
    {{ refreshing ? refreshingLabel : label }}
  </Button>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { RefreshCw } from "lucide-vue-next";
import Button from "./Button.vue";

type Variant = "default" | "outline";
type Size = "default" | "sm";

const props = withDefaults(
  defineProps<{
    label?: string;
    refreshingLabel?: string;
    variant?: Variant;
    size?: Size;
    disabled?: boolean;
    minDurationMs?: number;
    onClick?: () => void | Promise<void>;
  }>(),
  {
    label: "Refresh",
    refreshingLabel: "Refreshing",
    variant: "outline",
    size: "sm",
    disabled: false,
    minDurationMs: 600,
    onClick: undefined,
  }
);

const refreshing = ref(false);

const handleClick = async () => {
  if (refreshing.value || props.disabled) return;
  const started = Date.now();
  refreshing.value = true;
  try {
    await Promise.resolve(props.onClick?.());
  } finally {
    const elapsed = Date.now() - started;
    const remaining = Math.max(0, props.minDurationMs - elapsed);
    window.setTimeout(() => {
      refreshing.value = false;
    }, remaining);
  }
};
</script>
