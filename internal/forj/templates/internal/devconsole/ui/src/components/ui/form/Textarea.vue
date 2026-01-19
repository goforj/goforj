<template>
  <textarea
    ref="areaRef"
    v-bind="textareaAttrs"
    :class="classes"
    :value="modelValue"
    @input="handleInput"
    rows="1"
  />
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, useAttrs, watch } from "vue";
import type { ClassValue } from "clsx";
import { cn } from "../../../lib/utils";

const props = defineProps<{
  modelValue?: string;
}>();
const attrs = useAttrs();
const emit = defineEmits<{
  (e: "update:modelValue", value: string): void;
}>();
const areaRef = ref<HTMLTextAreaElement | null>(null);

const textareaAttrs = computed(() => {
  const copy = { ...attrs } as Record<string, unknown>;
  delete copy.class;
  if (!copy.rows) {
    copy.rows = 1;
  }
  return copy;
});

const classes = computed(() =>
  cn(
    "w-full resize-none overflow-hidden rounded-lg border border-border/70 bg-white/5 px-3 py-2 text-sm text-white transition focus:border-white/80 focus:outline-none",
    (attrs.class as ClassValue) ?? ""
  )
);

const adjustHeight = () => {
  const area = areaRef.value;
  if (!area) {
    return;
  }
  area.style.height = "auto";
  area.style.height = `${area.scrollHeight}px`;
};

const handleInput = (event: Event) => {
  const target = event.target as HTMLTextAreaElement;
  emit("update:modelValue", target.value);
  nextTick(() => adjustHeight());
};

onMounted(() => {
  nextTick(() => adjustHeight());
});

watch(() => props.modelValue, () => {
  nextTick(() => adjustHeight());
});
</script>
