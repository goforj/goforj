<template>
  <div class="space-y-6">
    <div class="space-y-1">
      <h2 class="text-xl font-semibold tracking-tight">Appearance settings</h2>
      <p class="text-sm text-muted-foreground">Update your account's appearance settings</p>
    </div>

    <div class="inline-flex rounded-lg border bg-muted p-1">
      <button
        v-for="option in options"
        :key="option.value"
        type="button"
        class="inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
        :class="preference === option.value ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
        @click="selectTheme(option.value)"
      >
        <component :is="option.icon" class="size-4" />
        {{ option.label }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Monitor, Moon, Sun } from '@lucide/vue'
import { setThemePreference, themePreference, type ThemePreference } from '@/lib/theme'

const preference = ref<ThemePreference>(themePreference())

const options = [
  { label: 'Light', value: 'light' as ThemePreference, icon: Sun },
  { label: 'Dark', value: 'dark' as ThemePreference, icon: Moon },
  { label: 'System', value: 'system' as ThemePreference, icon: Monitor },
]

function selectTheme(value: ThemePreference) {
  preference.value = value
  setThemePreference(value)
}
</script>
