<template>
  <input
    v-bind="$attrs"
    :class="classes"
    :value="modelValue"
    @input="handleInput"
  />
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
  (e: "update:modelValue", value: string): void;
}>();

const classes = computed(() =>
  cn(
    "flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50",
    (attrs.class as ClassValue) ?? ""
  )
);

const handleInput = (event: Event) => {
  const target = event.target as HTMLInputElement;
  emit("update:modelValue", target.value);
};
</script>
