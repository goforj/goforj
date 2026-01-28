<template>
  <button
    type="button"
    class="button-group-button"
    :class="active ? 'button-group-button-active' : ''"
    :disabled="disabled"
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
  color: rgba(255, 255, 255, 0.6);
  background: transparent;
  transition: background-color 0.15s ease, color 0.15s ease, transform 0.15s ease;
}

.button-group-button + .button-group-button {
  border-left: 1px solid rgba(255, 255, 255, 0.08);
}

.button-group-button:hover:not(:disabled) {
  background-color: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.85);
}

.button-group-button:active:not(:disabled) {
  transform: translateY(0.5px) scale(0.98);
}

.button-group-button:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.button-group-button-active {
  background-color: rgba(255, 255, 255, 0.18);
  color: #ffffff;
}
</style>
