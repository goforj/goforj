<template>
  <section class="grid gap-4">
    <Card>
      <CardHeader>
        <CardTitle>Settings</CardTitle>
        <CardDescription>Use this route as the foundation for account, workspace, and security configuration.</CardDescription>
      </CardHeader>
      <CardContent class="grid gap-4 md:grid-cols-3">
        <div class="rounded-lg border p-4">
          <p class="font-medium">Profile</p>
          <p class="mt-1 text-sm text-muted-foreground">Add display name, avatar, locale, and timezone controls here.</p>
        </div>
        <div class="rounded-lg border p-4">
          <p class="font-medium">Security</p>
          <p class="mt-1 text-sm text-muted-foreground">Connect recovery, 2FA, and session controls to <code>/api/v1/auth/*</code> when auth is enabled.</p>
        </div>
        <div class="rounded-lg border p-4">
          <p class="font-medium">Workspace preferences</p>
          <p class="mt-1 text-sm text-muted-foreground">Use this section for product defaults, environment settings, and user-level UI preferences.</p>
        </div>
      </CardContent>
    </Card>
  </section>
</template>

<script setup lang="ts">
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
</script>
