<template>
  <component :is="asChild ? Slot : 'button'" :class="classes">
    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { Slot } from "radix-vue";
import { cn } from "../../../lib/utils";

type Variant = "default" | "outline";
type Size = "default" | "sm";

const props = withDefaults(
  defineProps<
    {
      variant?: Variant;
      size?: Size;
      asChild?: boolean;
    }
  >(),
  {
    variant: "default",
    size: "default",
    asChild: false,
  }
);

const variantClasses: Record<Variant, string> = {
  default: "border-transparent bg-accent/20 text-white hover:bg-accent/30",
  outline: "border-border text-white/80 hover:text-white hover:border-accent",
};

const sizeClasses: Record<Size, string> = {
  default: "px-4 py-2",
  sm: "px-3 py-1",
};

const classes = computed(() =>
  cn(
    "inline-flex items-center justify-center whitespace-nowrap rounded-full border text-xs font-medium transition",
    variantClasses[props.variant],
    sizeClasses[props.size]
  )
);
</script>
