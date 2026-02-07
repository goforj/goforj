<script setup lang="ts">
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
</script>

<template>
  <div class="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
    <div class="px-4 lg:px-6">
      <Card>
        <CardHeader>
          <CardTitle>Metrics Notes</CardTitle>
        </CardHeader>
        <CardContent class="text-sm text-muted-foreground">
          Grafana/Loki/Prometheus integration can be wired here in the next pass.
        </CardContent>
      </Card>
    </div>
  </div>
</template>
