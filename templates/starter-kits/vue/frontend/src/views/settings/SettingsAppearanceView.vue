<template>
  <div class="grid gap-6">
    <div class="grid gap-1">
      <h2 class="text-xl font-semibold tracking-tight">Appearance settings</h2>
      <p class="text-sm text-muted-foreground">Update your account's appearance settings</p>
    </div>

    <ToggleGroup
      :model-value="preference"
      type="single"
      variant="outline"
      class="justify-start"
      aria-label="Theme"
      @update:model-value="selectTheme"
    >
      <ToggleGroupItem v-for="option in options" :key="option.value" :value="option.value">
        <component :is="option.icon" class="size-4" />
        {{ option.label }}
      </ToggleGroupItem>
    </ToggleGroup>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Monitor, Moon, Sun } from '@lucide/vue'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { setThemePreference, themePreference, type ThemePreference } from '@/lib/theme'

const preference = ref<ThemePreference>(themePreference())

const options = [
  { label: 'Light', value: 'light' as ThemePreference, icon: Sun },
  { label: 'Dark', value: 'dark' as ThemePreference, icon: Moon },
  { label: 'System', value: 'system' as ThemePreference, icon: Monitor },
]

// A single-select toggle group clears its value when the active item is
// pressed again. Theme always has a value, so ignore the empty case.
function selectTheme(value: unknown) {
  if (typeof value !== 'string' || !value) {
    return
  }
  preference.value = value as ThemePreference
  setThemePreference(value as ThemePreference)
}
</script>
