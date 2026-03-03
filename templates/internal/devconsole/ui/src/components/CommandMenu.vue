<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useDevconsoleStore } from "../stores/devconsole";
import { findAppNavItem } from "../lib/navigation";
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
const store = useDevconsoleStore();

const routes = computed(() =>
  router
    .getRoutes()
    .filter((entry) => !entry.meta?.public)
    .filter((entry) => entry.path !== "/login")
    .map((entry) => ({
      path: entry.path,
      title: (entry.meta?.title as string) || entry.name?.toString() || entry.path,
      icon: findAppNavItem(entry.path)?.icon,
    }))
    .sort((a, b) => a.title.localeCompare(b.title))
);

const actions = computed(() => [
  { id: "logout", title: "Log out" },
]);

const goTo = async (path: string) => {
  if (route.path === path) {
    emit("update:open", false);
    return;
  }
  await router.push(path);
  emit("update:open", false);
};

const runAction = async (id: string) => {
  if (id === "logout") {
    await store.logout();
    emit("update:open", false);
    router.replace("/login");
  }
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
          <component :is="entry.icon" v-if="entry.icon" class="size-4 text-muted-foreground" />
          {{ entry.title }}
        </CommandItem>
      </CommandGroup>
      <CommandSeparator />
      <CommandGroup heading="Actions">
        <CommandItem
          v-for="action in actions"
          :key="action.id"
          :value="action.title"
          @select="() => runAction(action.id)"
        >
          {{ action.title }}
        </CommandItem>
      </CommandGroup>
      <CommandSeparator />
    </CommandList>
  </CommandDialog>
</template>
