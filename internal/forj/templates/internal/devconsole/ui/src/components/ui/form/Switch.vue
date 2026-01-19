<template>
  <label :class="classes">
    <input
      type="checkbox"
      :checked="modelValue"
      class="peer sr-only"
      @change="handleChange"
      :disabled="disabled"
    />
    <span
      class="inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border border-border/70 bg-white/5 transition duration-150 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white/70"
      :class="{
        'border-accent bg-accent/10': modelValue,
      }"
    >
      <span
        class="mx-[0.125rem] h-5 w-5 rounded-full bg-white shadow transition duration-150"
        :class="{
          'translate-x-5': modelValue,
        }"
      ></span>
    </span>
    <span class="text-xs text-white"><slot /></span>
  </label>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { cn } from "../../../lib/utils";

const props = defineProps<{
  modelValue?: boolean;
  disabled?: boolean;
  class?: string;
}>();
const emit = defineEmits<{
  (e: "update:modelValue", value: boolean): void;
}>();

const classes = computed(() =>
  cn("flex items-center gap-3", props.class)
);

const handleChange = (event: Event) => {
  const target = event.target as HTMLInputElement;
  emit("update:modelValue", target.checked);
};
</script>
