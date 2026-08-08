<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Monitor, Moon, Sun } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { type ThemePreference, setThemePreference, themePreference } from '@/lib/theme'

const options = [
  { value: 'light' as const, label: 'Light', icon: Sun },
  { value: 'dark' as const, label: 'Dark', icon: Moon },
  { value: 'system' as const, label: 'System', icon: Monitor },
]

// localStorage is unavailable during SSR-style prerender, so resolve on mount.
const preference = ref<ThemePreference>('system')
onMounted(() => {
  preference.value = themePreference()
})

function select(value: ThemePreference) {
  preference.value = value
  setThemePreference(value)
}
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" size="icon" aria-label="Change theme">
        <Sun class="size-4 dark:hidden" />
        <Moon class="hidden size-4 dark:block" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" class="w-36">
      <DropdownMenuItem
        v-for="option in options"
        :key="option.value"
        :data-active="preference === option.value ? '' : undefined"
        class="data-[active]:bg-accent data-[active]:text-accent-foreground"
        @select="select(option.value)"
      >
        <component :is="option.icon" class="size-4" />
        {{ option.label }}
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
