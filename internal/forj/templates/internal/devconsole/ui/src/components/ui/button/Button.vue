<template>
  <component :is="asChild ? Slot : 'button'" :class="classes" :disabled="disabled">
    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { Slot } from "radix-vue";
import { cn } from "../../../lib/utils";

type Variant = "default" | "outline" | "ghost";
type Size = "default" | "sm" | "icon";

const props = withDefaults(
  defineProps<
    {
      variant?: Variant;
      size?: Size;
      asChild?: boolean;
      disabled?: boolean;
    }
  >(),
  {
    variant: "default",
    size: "default",
    asChild: false,
    disabled: false,
  }
);

const variantClasses: Record<Variant, string> = {
  default: "bg-primary text-primary-foreground shadow hover:bg-primary/90",
  outline:
    "border border-input bg-background shadow-sm hover:bg-accent hover:text-accent-foreground",
  ghost: "hover:bg-accent hover:text-accent-foreground",
};

const sizeClasses: Record<Size, string> = {
  default: "h-9 px-4 py-2",
  sm: "h-8 rounded-md px-3 text-xs",
  icon: "h-9 w-9",
};

const classes = computed(() =>
  cn(
    "inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 active:scale-[0.98] active:translate-y-[0.5px]",
    variantClasses[props.variant],
    sizeClasses[props.size],
    props.disabled ? "pointer-events-none cursor-not-allowed opacity-50" : ""
  )
);
</script>
