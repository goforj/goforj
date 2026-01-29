<template>
  <button
    type="button"
    class="button-group-button"
    :class="active ? 'button-group-button-active' : ''"
    :disabled="disabled"
    :aria-pressed="active"
    :data-state="active ? 'on' : 'off'"
  >
    <slot />
  </button>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    active?: boolean;
    disabled?: boolean;
  }>(),
  {
    active: false,
    disabled: false,
  }
);
</script>

<style scoped>
.button-group-button {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.28rem 0.65rem;
  font-size: 11px;
  line-height: 1.1;
  text-transform: uppercase;
  letter-spacing: 0.18em;
  color: hsl(var(--muted-foreground));
  background: transparent;
  transition: background-color 0.15s ease, color 0.15s ease, transform 0.15s ease, box-shadow 0.15s ease;
}

.button-group-button + .button-group-button {
  border-left: 1px solid hsl(var(--border));
}

.button-group-button:hover:not(:disabled) {
  background-color: hsl(var(--muted));
  color: hsl(var(--foreground));
}

.button-group-button:active:not(:disabled) {
  transform: translateY(0.5px) scale(0.98);
}

.button-group-button:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.button-group-button-active {
  background-color: hsl(var(--background));
  color: hsl(var(--foreground));
  font-weight: 600;
  box-shadow: inset 0 0 0 1px hsl(var(--border)), 0 1px 2px hsl(var(--foreground) / 0.08);
}
</style>
