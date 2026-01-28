<template>
  <div>
    <PageHeader label="Platform" title="Components">
      <template #right>
        <AgentPills />
        <LivePill />
      </template>
    </PageHeader>

    <section class="mt-8 grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Buttons</p>
            <CardTitle>Button Variants</CardTitle>
          </template>
          <template #description>
            <CardDescription>shadcn-vue button styles and sizes.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="grid gap-6 text-xs">
            <div class="grid gap-3">
              <span class="text-[10px] uppercase tracking-[0.2em] text-muted">Variants</span>
              <div class="flex flex-wrap items-center gap-3">
                <Button>Primary</Button>
                <Button variant="outline">Outline</Button>
                <Button variant="ghost">Ghost</Button>
                <Button disabled>Disabled</Button>
              </div>
            </div>
            <div class="grid gap-3">
              <span class="text-[10px] uppercase tracking-[0.2em] text-muted">Sizes</span>
              <div class="flex flex-wrap items-center gap-3">
                <Button size="sm">Small</Button>
                <Button>Default</Button>
                <Button size="icon">
                  <RefreshCw class="h-4 w-4" />
                </Button>
              </div>
            </div>
            <div class="grid gap-3">
              <span class="text-[10px] uppercase tracking-[0.2em] text-muted">With Icons</span>
              <div class="flex flex-wrap items-center gap-3">
                <Button>
                  <Play class="mr-1 h-3.5 w-3.5" />
                  Run
                </Button>
                <Button variant="outline">
                  <Pause class="mr-1 h-3.5 w-3.5" />
                  Pause
                </Button>
                <Button variant="ghost">
                  <Settings class="mr-1 h-3.5 w-3.5" />
                  Settings
                </Button>
              </div>
            </div>
            <div class="grid gap-3">
              <span class="text-[10px] uppercase tracking-[0.2em] text-muted">Refresh Button</span>
              <div class="flex flex-wrap items-center gap-3">
                <RefreshButton :on-click="noop" />
                <RefreshButton variant="outline" :on-click="noop" />
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Forms</p>
            <CardTitle>Inputs & Fields</CardTitle>
          </template>
          <template #description>
            <CardDescription>Input, select, and textarea examples.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="grid gap-4 text-xs sm:grid-cols-2">
            <FormField label="Username">
              <Input placeholder="admin" />
            </FormField>
            <FormField label="Token">
              <Input placeholder="DEVCONSOLE_TOKEN" />
            </FormField>
            <FormField label="Environment">
              <Select>
                <option>Local</option>
                <option>Staging</option>
                <option>Production</option>
              </Select>
            </FormField>
            <FormField label="Notes">
              <Textarea placeholder="Leave a note..." rows="3" />
            </FormField>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Tabs</p>
            <CardTitle>Tabs Variants</CardTitle>
          </template>
          <template #description>
            <CardDescription>Tabs with labels and content.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <Tabs v-model="activeTab">
            <TabsList>
              <TabsTrigger value="first">First</TabsTrigger>
              <TabsTrigger value="second">Second</TabsTrigger>
              <TabsTrigger value="third">Third</TabsTrigger>
            </TabsList>
          </Tabs>
          <div class="mt-4 text-xs text-muted">
            Active tab: <span class="text-white">{{ activeTab }}</span>
          </div>
        </CardContent>
      </Card>

      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <p class="text-xs uppercase tracking-[0.3em] text-muted">Toasts</p>
            <CardTitle>Sonner</CardTitle>
          </template>
          <template #description>
            <CardDescription>Trigger toast notifications.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="flex flex-wrap items-center gap-3">
            <Button @click="toast('Primary toast')">Toast</Button>
            <Button variant="outline" @click="toast('Outline toast')">Outline</Button>
            <Button variant="ghost" @click="toast('Ghost toast')">Ghost</Button>
          </div>
        </CardContent>
      </Card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { toast } from "vue-sonner";
import AgentPills from "../components/AgentPills.vue";
import LivePill from "../components/LivePill.vue";
import PageHeader from "../components/PageHeader.vue";
import Button from "../components/ui/button/Button.vue";
import RefreshButton from "../components/ui/button/RefreshButton.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import Select from "../components/ui/form/Select.vue";
import Textarea from "../components/ui/form/Textarea.vue";
import { Tabs, TabsList, TabsTrigger } from "../components/ui/tabs";
import { Pause, Play, RefreshCw, Settings } from "lucide-vue-next";

const activeTab = ref("first");
const noop = () => {};
</script>
