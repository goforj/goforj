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
      class="inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border border-input bg-muted transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
      :class="{
        'bg-primary border-primary': modelValue,
      }"
    >
      <span
        class="mx-[0.125rem] h-5 w-5 rounded-full bg-background shadow transition duration-150"
        :class="{
          'translate-x-5': modelValue,
        }"
      ></span>
    </span>
    <span class="text-xs text-foreground"><slot /></span>
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
