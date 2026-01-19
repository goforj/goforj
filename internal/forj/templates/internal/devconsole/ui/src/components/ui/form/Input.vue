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
    "w-full rounded-lg border border-border/70 bg-white/5 px-3 py-2 text-sm text-white transition focus:border-white/80 focus:outline-none",
    (attrs.class as ClassValue) ?? ""
  )
);

const handleInput = (event: Event) => {
  const target = event.target as HTMLInputElement;
  emit("update:modelValue", target.value);
};
</script>
