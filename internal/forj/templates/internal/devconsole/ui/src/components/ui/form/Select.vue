<template>
  <select v-bind="selectAttrs" :class="classes" :value="modelValue" @change="handleChange">
    <slot />
  </select>
</template>

<script setup lang="ts">
import { computed, useAttrs } from "vue";
import type { ClassValue } from "clsx";
import { cn } from "../../../lib/utils";

const props = defineProps<{
  modelValue?: string | number;
}>();
const attrs = useAttrs();
const emit = defineEmits<{
  (e: "update:modelValue", value: string | number): void;
}>();

const modelModifiers = computed(() => (attrs.modelModifiers as Record<string, boolean>) ?? {});

const selectAttrs = computed(() => {
  const copy = { ...attrs } as Record<string, unknown>;
  delete copy.class;
  delete copy.modelModifiers;
  return copy;
});

const classes = computed(() =>
  cn(
    "flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50",
    (attrs.class as ClassValue) ?? ""
  )
);

const handleChange = (event: Event) => {
  const target = event.target as HTMLSelectElement;
  let value: string | number = target.value;
  if (modelModifiers.value.number) {
    const parsed = Number(value);
    if (!Number.isNaN(parsed)) {
      value = parsed;
    }
  }
  emit("update:modelValue", value);
};
</script>
