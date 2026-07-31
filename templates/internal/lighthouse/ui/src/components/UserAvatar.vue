<script setup lang="ts">
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
} from "@/components/ui/avatar";

const props = defineProps<{
  user: {
    name: string;
    avatar?: string;
    avatarDark?: string;
    avatarLight?: string;
  };
}>();
</script>

<template>
  <Avatar class="h-8 w-8 rounded-lg">
    <AvatarImage v-if="props.user.avatar" :src="props.user.avatar" :alt="props.user.name" />
    <template v-else-if="props.user.avatarDark && props.user.avatarLight">
      <img
        :src="props.user.avatarDark"
        :alt="props.user.name"
        class="hidden size-full object-contain p-1 dark:block"
      />
      <img
        :src="props.user.avatarLight"
        :alt="props.user.name"
        class="size-full object-contain p-1 dark:hidden"
      />
    </template>
    <AvatarFallback v-else class="rounded-lg">
      {{ props.user.name.slice(0, 2).toUpperCase() }}
    </AvatarFallback>
  </Avatar>
</template>
