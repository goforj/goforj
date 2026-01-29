<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "./ui/command";

const props = defineProps<{
  open: boolean;
}>();

const emit = defineEmits<{
  (event: "update:open", value: boolean): void;
}>();

const router = useRouter();
const route = useRoute();

const routes = computed(() =>
  router
    .getRoutes()
    .filter((entry) => !entry.meta?.public)
    .filter((entry) => entry.path !== "/login")
    .map((entry) => ({
      path: entry.path,
      title: (entry.meta?.title as string) || entry.name?.toString() || entry.path,
    }))
    .sort((a, b) => a.title.localeCompare(b.title))
);

const goTo = async (path: string) => {
  if (route.path === path) {
    emit("update:open", false);
    return;
  }
  await router.push(path);
  emit("update:open", false);
};
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
          {{ entry.title }}
        </CommandItem>
      </CommandGroup>
      <CommandSeparator />
    </CommandList>
  </CommandDialog>
</template>
