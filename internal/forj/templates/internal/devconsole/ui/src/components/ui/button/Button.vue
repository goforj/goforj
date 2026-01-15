<template>
  <component :is="asChild ? Slot : 'button'" :class="classes">
    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-vue";
import { cn } from "../../../lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center whitespace-nowrap rounded-full border text-xs font-medium transition",
  {
    variants: {
      variant: {
        default: "border-transparent bg-accent/20 text-white hover:bg-accent/30",
        outline: "border-border text-white/80 hover:text-white hover:border-accent",
      },
      size: {
        default: "px-4 py-2",
        sm: "px-3 py-1",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
);

type ButtonVariants = VariantProps<typeof buttonVariants>;

const props = withDefaults(
  defineProps<
    ButtonVariants & {
      asChild?: boolean;
    }
  >(),
  {
    asChild: false,
  }
);

const classes = computed(() => cn(buttonVariants({ variant: props.variant, size: props.size })));
</script>
