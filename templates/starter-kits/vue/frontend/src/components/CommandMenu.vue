<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Settings } from 'lucide-vue-next'
import { appNavMain } from '@/lib/navigation'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (event: 'update:open', value: boolean): void
  (event: 'logout'): void
}>()

const router = useRouter()
const route = useRoute()

const routes = computed(() => {
  const navigationEntries = appNavMain.flatMap((item) => {
    if (!item.items?.length) {
      return [{ path: item.url, title: item.title, icon: item.icon }]
    }
    return item.items.map((child) => ({
      path: child.url,
      title: `${item.title} ${child.title}`,
      icon: item.icon,
    }))
  })

  const settingsEntries = [
    { path: '/settings/profile', title: 'Profile settings', icon: Settings },
    { path: '/settings/password', title: 'Password settings', icon: Settings },
    { path: '/settings/appearance', title: 'Appearance settings', icon: Settings },
  ]

  return [...navigationEntries, ...settingsEntries].sort((a, b) => a.title.localeCompare(b.title))
})

async function goTo(path: string) {
  if (route.path !== path) {
    await router.push(path)
  }
  emit('update:open', false)
}

function logout() {
  emit('update:open', false)
  emit('logout')
}
</script>

<template>
  <CommandDialog :open="props.open" @update:open="(value) => emit('update:open', value)">
    <CommandInput placeholder="Search pages..." />
    <CommandList>
      <CommandEmpty>No results found.</CommandEmpty>
      <CommandGroup heading="Navigation">
        <CommandItem
          v-for="entry in routes"
          :key="entry.path"
          :value="entry.title"
          @select="() => goTo(entry.path)"
        >
          <component :is="entry.icon" v-if="entry.icon" class="size-4 text-muted-foreground" />
          {{ entry.title }}
        </CommandItem>
      </CommandGroup>
      <CommandSeparator />
      <CommandGroup heading="Actions">
        <CommandItem value="Log out" @select="logout">Log out</CommandItem>
      </CommandGroup>
    </CommandList>
  </CommandDialog>
</template>
