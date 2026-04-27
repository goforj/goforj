<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from "vue"
import Command from "./Command.vue"

const props = withDefaults(defineProps<{
  open?: boolean
  title?: string
  description?: string
}>(), {
  open: false,
  title: "Command Menu",
  description: "Search for a command to run...",
})

const emit = defineEmits<{
  "update:open": [value: boolean]
}>()

const dialogRef = ref<HTMLDialogElement | null>(null)
const surfaceRef = ref<HTMLElement | null>(null)

async function focusCommandInput() {
  await nextTick()
  requestAnimationFrame(() => {
    const input = document.querySelector<HTMLInputElement>('[data-slot="command-input"]')
    input?.focus({ preventScroll: true })
  })
}

async function syncDialog(open: boolean) {
  const dialog = dialogRef.value
  if (!dialog) {
    return
  }

  if (open) {
    if (!dialog.open) {
      dialog.showModal()
    }
    await focusCommandInput()
    return
  }

  if (dialog.open) {
    dialog.close()
  }
}

function closeDialog() {
  emit("update:open", false)
}

function handleCancel(event: Event) {
  event.preventDefault()
  closeDialog()
}

function handleBackdropClick(event: MouseEvent) {
  const target = event.target
  if (!(target instanceof Node)) {
    return
  }
  if (!surfaceRef.value?.contains(target)) {
    closeDialog()
  }
}

watch(() => props.open, value => {
  void syncDialog(value)
})

onMounted(() => {
  void syncDialog(props.open)
})
</script>

<template>
  <dialog
    ref="dialogRef"
    class="command-dialog-root"
    aria-label="Command Menu"
    @cancel="handleCancel"
    @click="handleBackdropClick"
  >
    <div class="command-dialog-shell">
      <div
        ref="surfaceRef"
        class="bg-background flex max-h-[80vh] w-[min(calc(100vw-2rem),42rem)] flex-col overflow-hidden rounded-lg border shadow-lg"
      >
        <div class="sr-only">
          <h2>{{ title }}</h2>
          <p>{{ description }}</p>
        </div>
        <Command>
          <slot />
        </Command>
      </div>
    </div>
  </dialog>
</template>

<style scoped>
.command-dialog-root {
  position: fixed;
  inset: 0;
  width: 100dvw;
  height: 100dvh;
  max-width: none;
  max-height: none;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  overflow: visible;
}

.command-dialog-root::backdrop {
  background: rgb(0 0 0 / 0.8);
}

.command-dialog-shell {
  display: grid;
  min-height: 100%;
  width: 100%;
  place-items: center;
  padding: 1rem;
}
</style>
