<template>
  <div>
    <section class="grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <KeyRound class="h-4 w-4 text-muted-foreground" />
              Cache
            </CardTitle>
          </template>
          <template #description>
            <CardDescription>Inspect configured cache stores, preview values, and delete keys from the connected app runtime.</CardDescription>
          </template>
          <template #action>
            <div class="flex items-center gap-2">
              <Button variant="outline" size="sm" :disabled="!selectedStore || creatingKey" @click="openCreateDialog">
                <Plus class="mr-1 h-3.5 w-3.5" />
                New key
              </Button>
              <RefreshButton :refreshing="loading" :on-click="refresh" />
            </div>
          </template>
        </CardHeader>
        <CardContent class="space-y-4">
          <div v-if="cacheAgents.length === 0" class="rounded-xl border border-border/60 bg-muted/30 p-4 text-sm text-muted-foreground">
            No cache-capable agents are connected.
          </div>

          <template v-else>
            <div class="grid gap-4 text-xs sm:grid-cols-2 lg:grid-cols-3">
              <FormField v-if="showAgentFilter" label="Target agent">
                <Select v-model="targetModel">
                  <SelectTrigger class="w-full">
                    <SelectValue placeholder="Select agent" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem :value="emptySelectValue">Select agent</SelectItem>
                    <SelectItem v-for="agent in cacheAgents" :key="agent.source" :value="agent.source">
                    {{ agent.source }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
              <FormField>
                <template #label>
                  <span class="inline-flex items-center gap-1.5">
                    <KeyRound class="h-3.5 w-3.5" />
                    Cache ({{ explorer.stores.length }})
                  </span>
                </template>
                <Select v-model="selectedStore">
                  <SelectTrigger class="w-full">
                    <SelectValue placeholder="Select cache" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="store in explorer.stores" :key="store.name" :value="store.name">
                    {{ storeLabel(store) }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
              <FormField>
                <template #label>
                  <span class="inline-flex items-center gap-1.5">
                    <Search class="h-3.5 w-3.5" />
                    Key filter
                  </span>
                </template>
                <Input v-model="prefix" placeholder="Match any part of the key..." />
              </FormField>
            </div>

            <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/60 bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
              <div>Showing {{ explorer.entries.length }} keys</div>
              <div class="flex items-center gap-2">
                <span>Page {{ currentPage }}</span>
                <Button variant="outline" size="sm" :disabled="currentPage <= 1" @click="goToPage(currentPage - 1)">
                  Prev
                </Button>
                <Button variant="outline" size="sm" :disabled="!explorer.has_more" @click="goToPage(currentPage + 1)">
                  Next
                </Button>
              </div>
            </div>

            <div v-if="activeStore" class="flex flex-wrap items-center gap-2 rounded-xl border border-border/60 bg-muted/15 px-3 py-2 text-[11px] text-muted-foreground">
              <span class="font-medium text-foreground">{{ storeLabel(activeStore) }}</span>
              <span class="rounded-md border border-border/70 px-2 py-1">browse: {{ activeStore.inspectable ? "yes" : "no" }}</span>
              <span class="rounded-md border border-border/70 px-2 py-1">read: {{ activeStore.can_read ? "yes" : "no" }}</span>
              <span class="rounded-md border border-border/70 px-2 py-1">delete: {{ activeStore.can_delete ? "yes" : "no" }}</span>
              <span class="rounded-md border border-border/70 px-2 py-1">ttl: {{ activeStore.can_ttl ? "yes" : "no" }}</span>
            </div>

            <div v-if="loadError" class="rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {{ loadError }}
            </div>
            <div v-else-if="unsupportedNotice" class="rounded-xl border border-amber-400/30 bg-amber-400/10 px-4 py-3 text-sm text-amber-200">
              {{ unsupportedNotice }}
            </div>
            <div v-else-if="unavailableStoreNotice" class="rounded-xl border border-amber-400/30 bg-amber-400/10 px-4 py-3 text-sm text-amber-200">
              {{ unavailableStoreNotice }}
            </div>

            <div class="max-h-[68vh] overflow-auto rounded-xl border border-border/60">
              <table class="w-full text-xs">
                <thead class="bg-muted/40 text-muted">
                  <tr>
                    <th v-if="showAgentColumn" class="px-4 py-3 text-left">Agent</th>
                    <th class="px-4 py-3 text-left">
                      <span class="inline-flex items-center gap-1">
                        <KeyRound class="h-3.5 w-3.5" />
                        Key
                      </span>
                    </th>
                    <th class="px-4 py-3 text-left">
                      <span class="inline-flex items-center gap-1">
                        <Clock3 class="h-3.5 w-3.5" />
                        Expires
                      </span>
                    </th>
                    <th class="px-4 py-3 text-left">
                      <span class="inline-flex items-center gap-1">
                        <HardDrive class="h-3.5 w-3.5" />
                        Size
                      </span>
                    </th>
                    <th class="px-4 py-3 text-left">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-if="explorer.entries.length === 0" class="border-t border-border/60">
                    <td :colspan="showAgentColumn ? 5 : 4" class="px-4 py-3 text-muted">
                      No cache keys found.
                    </td>
                  </tr>
                  <tr v-for="entry in explorer.entries" :key="`${selectedStore}:${entry.key}`" class="border-t border-border/60 transition hover:bg-muted/20">
                    <td v-if="showAgentColumn" class="px-4 py-2.5 text-foreground">
                      {{ target }}
                    </td>
                    <td class="px-4 py-2.5 font-mono text-foreground">{{ entry.key }}</td>
                    <td class="px-4 py-2.5 text-muted">{{ formatExpiry(entry.expires_at) }}</td>
                    <td class="px-4 py-2.5 text-muted">{{ formatBytes(entry.size) }}</td>
                    <td class="px-4 py-2.5">
                      <div class="flex items-center gap-2">
                        <Button variant="outline" size="sm" :disabled="previewingKey === entry.key" @click="previewEntry(entry)">
                          <Eye class="mr-1 h-3.5 w-3.5" />
                          {{ previewingKey === entry.key ? "Previewing..." : "Preview" }}
                        </Button>
                        <Button variant="outline" size="sm" @click="copyKey(entry.key)">
                          <Copy class="mr-1 h-3.5 w-3.5" />
                          Copy key
                        </Button>
                        <Button variant="outline" size="sm" :disabled="editingKey === entry.key" @click="editEntry(entry)">
                          <Pencil class="mr-1 h-3.5 w-3.5" />
                          {{ editingKey === entry.key ? "Loading..." : "Edit" }}
                        </Button>
                        <Button variant="outline" size="sm" :disabled="deletingKey === entry.key" @click="deleteEntry(entry)">
                          <Trash2 class="mr-1 h-3.5 w-3.5" />
                          {{ deletingKey === entry.key ? "Deleting..." : "Delete" }}
                        </Button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </template>
        </CardContent>
      </Card>

      <Dialog :open="previewOpen" @update:open="(value) => (previewOpen = value)">
        <DialogContent class="max-w-4xl">
          <DialogHeader>
            <DialogTitle>{{ previewKey || "Cache Preview" }}</DialogTitle>
            <DialogDescription class="font-mono text-xs">
              {{ selectedStore }}
            </DialogDescription>
          </DialogHeader>
          <div v-if="previewLoading" class="rounded-xl border border-border/60 bg-muted/20 p-4 text-sm text-muted-foreground">
            Loading cache value...
          </div>
          <div v-else-if="previewError" class="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
            {{ previewError }}
          </div>
          <template v-else>
            <div class="flex items-center justify-between gap-3 rounded-xl border border-border/60 bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
              <div>{{ previewText.length }} chars</div>
              <Button variant="outline" size="sm" :disabled="!previewText" @click="copyPreviewText">
                <Copy class="mr-1 h-3.5 w-3.5" />
                Copy text
              </Button>
            </div>
            <div
              v-if="previewKind === 'text'"
              class="max-h-[70vh] overflow-auto rounded-xl border border-border/60 bg-black/40 p-4 font-mono text-xs leading-6 text-slate-100"
              v-html="previewTextHTML"
            />
            <div v-else class="rounded-xl border border-border/60 bg-muted/20 p-4 text-sm text-muted-foreground">
              Binary cache values are not previewable in the browser yet.
            </div>
          </template>
        </DialogContent>
      </Dialog>

      <Dialog :open="createOpen" @update:open="(value) => (createOpen = value)">
        <DialogContent class="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{{ createMode === "edit" ? "Edit cache key" : "Create cache key" }}</DialogTitle>
            <DialogDescription class="font-mono text-xs">
              {{ selectedStore || "Select a cache store first" }}
            </DialogDescription>
          </DialogHeader>
          <div class="space-y-4">
            <div v-if="createError" class="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
              {{ createError }}
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <FormField label="Key">
                <Input v-model="createKey" :disabled="createMode === 'edit'" placeholder="example:key" />
              </FormField>
              <FormField label="Expiration (seconds)" :description="activeStore?.can_ttl ? 'Leave empty to use the store default TTL.' : 'TTL visibility is limited for this store driver.'">
                <Input v-model="createTTL" type="number" min="0" step="1" placeholder="300" />
              </FormField>
            </div>
            <FormField label="Value (text)" description="Binary values are not editable in the browser yet.">
              <Textarea v-model="createValue" rows="10" placeholder='{"hello":"world"}' />
            </FormField>
            <div class="flex items-center justify-end gap-2">
              <Button variant="outline" size="sm" :disabled="creatingKey" @click="createOpen = false">
                Cancel
              </Button>
              <Button size="sm" :disabled="creatingKey || !selectedStore" @click="createEntry">
                <component :is="createMode === 'edit' ? Pencil : Plus" class="mr-1 h-3.5 w-3.5" />
                {{ creatingKey ? (createMode === "edit" ? "Saving..." : "Creating...") : (createMode === "edit" ? "Save key" : "Create key") }}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { toast } from "vue-sonner";
import { useRoute, useRouter } from "vue-router";
import {
  Clock3,
  Copy,
  Eye,
  HardDrive,
  KeyRound,
  Pencil,
  Plus,
  Search,
  Trash2,
} from "lucide-vue-next";
import { useLighthouseStore } from "../stores/lighthouse";
import Button from "../components/ui/button/Button.vue";
import RefreshButton from "../components/ui/button/RefreshButton.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/input/Input.vue";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../components/ui/select";
import Textarea from "../components/ui/textarea/Textarea.vue";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "../components/ui/dialog";

type CacheAgent = {
  source: string;
  capabilities: string[];
};

type CacheStore = {
  name: string;
  driver?: string;
  inspectable?: boolean;
  can_read?: boolean;
  can_delete?: boolean;
  can_ttl?: boolean;
  is_default: boolean;
};

type CacheEntry = {
  key: string;
  size: number;
  expires_at?: number;
};

type CacheExplorerPayload = {
  stores: CacheStore[];
  current_store: string;
  query?: string;
  inspectable: boolean;
  offset?: number;
  limit?: number;
  has_more?: boolean;
  entries: CacheEntry[];
};

const CACHE_PREVIEW_MAX_BYTES = 1024 * 1024;
const PAGE_SIZE = 100;
const emptySelectValue = "__empty__";

const { state, sendCommand } = useLighthouseStore();
const route = useRoute();
const router = useRouter();
const loading = ref(false);
const loadError = ref("");
const unsupportedNotice = ref("");
const unavailableStoreNotice = ref("");
const target = ref(state.selectedAgent || "");
const selectedStore = ref("");
const prefix = ref("");
const currentPage = ref(1);
const syncingState = ref(false);
const hydrated = ref(false);
const previewOpen = ref(false);
const previewLoading = ref(false);
const previewError = ref("");
const previewKey = ref("");
const previewText = ref("");
const previewKind = ref<"none" | "text">("none");
const previewingKey = ref("");
const deletingKey = ref("");
const createOpen = ref(false);
const createKey = ref("");
const createValue = ref("");
const createTTL = ref("");
const createError = ref("");
const creatingKey = ref(false);
const createMode = ref<"create" | "edit">("create");
const editingKey = ref("");
const explorer = ref<CacheExplorerPayload>({
  stores: [],
  current_store: "",
  query: "",
  inspectable: true,
  offset: 0,
  limit: PAGE_SIZE,
  has_more: false,
  entries: [],
});

const cacheAgents = computed<CacheAgent[]>(() =>
  state.agents.filter((agent) => agent.capabilities.includes("cache"))
);

const targetModel = computed({
  get: () => target.value || emptySelectValue,
  set: (value: string) => {
    target.value = value === emptySelectValue ? "" : value;
  },
});

const showAgentFilter = computed(() => cacheAgents.value.length > 1);
const showAgentColumn = computed(() => cacheAgents.value.length > 1);
const activeStore = computed(() =>
  explorer.value.stores.find((store) => store.name === selectedStore.value) || null
);

const parsePayload = (result: any) => {
  if (!result?.ok || !result.data) {
    return null;
  }
  return typeof result.data === "string" ? JSON.parse(result.data) : result.data;
};

const readQueryValue = (value: unknown) => {
  if (Array.isArray(value)) {
    return String(value[0] || "").trim();
  }
  return typeof value === "string" ? value.trim() : "";
};

const syncURL = () => {
  const nextQuery: Record<string, string> = {};
  if (target.value) nextQuery.agent = target.value;
  if (selectedStore.value) nextQuery.store = selectedStore.value;
  if (prefix.value.trim()) nextQuery.prefix = prefix.value.trim();
  if (currentPage.value > 1) nextQuery.page = String(currentPage.value);
  const same =
    readQueryValue(route.query.agent) === (nextQuery.agent || "") &&
    readQueryValue(route.query.store) === (nextQuery.store || "") &&
    readQueryValue(route.query.prefix) === (nextQuery.prefix || "") &&
    readQueryValue(route.query.page) === (nextQuery.page || "");
  if (same) {
    return;
  }
  const onlyPrefixChanged =
    readQueryValue(route.query.agent) === (nextQuery.agent || "") &&
    readQueryValue(route.query.store) === (nextQuery.store || "") &&
    readQueryValue(route.query.page) === (nextQuery.page || "") &&
    readQueryValue(route.query.prefix) !== (nextQuery.prefix || "");
  const navigate = onlyPrefixChanged ? router.replace : router.push;
  navigate.call(router, { path: route.path, query: nextQuery });
};

const applyURLState = () => {
  syncingState.value = true;
  const nextAgent = readQueryValue(route.query.agent);
  const nextStore = readQueryValue(route.query.store);
  const nextPrefix = readQueryValue(route.query.prefix);
  const nextPage = Number.parseInt(readQueryValue(route.query.page) || "1", 10);
  if (nextAgent) {
    target.value = nextAgent;
  }
  if (nextStore) {
    selectedStore.value = nextStore;
  }
  prefix.value = nextPrefix;
  currentPage.value = Number.isFinite(nextPage) && nextPage > 0 ? nextPage : 1;
  syncingState.value = false;
};

const ensureDefaultAgent = () => {
  if (cacheAgents.value.length === 0) {
    target.value = "";
    return;
  }
  if (target.value && cacheAgents.value.some((agent) => agent.source === target.value)) {
    return;
  }
  const selected = state.selectedAgent;
  if (selected && cacheAgents.value.some((agent) => agent.source === selected)) {
    target.value = selected;
    return;
  }
  target.value = cacheAgents.value[0].source;
};

const refresh = async () => {
  ensureDefaultAgent();
  loadError.value = "";
  unsupportedNotice.value = "";
  unavailableStoreNotice.value = "";
  if (!target.value) {
    explorer.value = { stores: [], current_store: "", prefix: "", inspectable: true, offset: 0, limit: PAGE_SIZE, has_more: false, entries: [] };
    syncingState.value = true;
    selectedStore.value = "";
    syncingState.value = false;
    return;
  }
  loading.value = true;
  try {
    const requestedStore = selectedStore.value.trim();
    const result = await sendCommand(target.value, "cache:list", {
      store: requestedStore,
      query: prefix.value.trim(),
      offset: (currentPage.value - 1) * PAGE_SIZE,
      limit: PAGE_SIZE,
    });
    const payload = parsePayload(result) as CacheExplorerPayload | null;
    if (!payload) {
      throw new Error("Cache explorer returned no payload.");
    }
    explorer.value = payload;
    syncingState.value = true;
    const nextStore = payload.current_store || payload.stores[0]?.name || "";
    selectedStore.value = nextStore;
    syncingState.value = false;
    if (requestedStore && !nextStore) {
      unavailableStoreNotice.value = `Cache store "${requestedStore}" is unavailable right now.`;
    } else if (nextStore && payload.inspectable === false) {
      unsupportedNotice.value = `Key browsing is unavailable for cache store "${nextStore}".`;
    }
    syncURL();
  } catch (err: any) {
    loadError.value = err?.message || "Unable to load cache explorer.";
  } finally {
    loading.value = false;
  }
};

const goToPage = async (page: number) => {
  const bounded = Math.max(1, page);
  if (bounded === currentPage.value) {
    return;
  }
  currentPage.value = bounded;
  syncURL();
  await refresh();
};

const decodeBase64 = (value: string) => {
  const binary = window.atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
};

const bytesToText = (bytes: Uint8Array) => {
  try {
    return new TextDecoder("utf-8").decode(bytes);
  } catch {
    return "";
  }
};

const escapeHTML = (value: string) =>
  value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");

const formatJSONPreview = (value: string) => {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
};

const highlightJSON = (value: string) =>
  escapeHTML(value).replace(
    /("(?:\\u[\da-fA-F]{4}|\\[^u]|[^\\"])*")(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][+\-]?\d+)?/g,
    (match, stringToken, isKey, primitiveToken) => {
      if (stringToken) {
        if (isKey) {
          return `<span class="storage-preview-key">${stringToken}</span>${isKey}`;
        }
        return `<span class="storage-preview-string">${stringToken}</span>`;
      }
      if (primitiveToken === "true" || primitiveToken === "false") {
        return `<span class="storage-preview-boolean">${match}</span>`;
      }
      if (primitiveToken === "null") {
        return `<span class="storage-preview-null">${match}</span>`;
      }
      return `<span class="storage-preview-number">${match}</span>`;
    }
  );

const previewTextHTML = computed(() => {
  if (!previewText.value) {
    return "";
  }
  const trimmed = previewText.value.trim();
  if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
    return highlightJSON(formatJSONPreview(previewText.value));
  }
  return escapeHTML(previewText.value);
});

const resetPreview = () => {
  previewLoading.value = false;
  previewError.value = "";
  previewKey.value = "";
  previewText.value = "";
  previewKind.value = "none";
};

const isProbablyText = (bytes: Uint8Array) => {
  if (bytes.length === 0) {
    return true;
  }
  let printable = 0;
  const sample = bytes.slice(0, Math.min(bytes.length, 256));
  for (const byte of sample) {
    if (byte === 9 || byte === 10 || byte === 13 || (byte >= 32 && byte <= 126)) {
      printable += 1;
    }
  }
  return printable / sample.length >= 0.85;
};

const previewEntry = async (entry: CacheEntry) => {
  if (entry.size > CACHE_PREVIEW_MAX_BYTES) {
    toast.error(`Preview limit exceeded (${formatBytes(entry.size)} > ${formatBytes(CACHE_PREVIEW_MAX_BYTES)}).`);
    return;
  }
  previewingKey.value = entry.key;
  resetPreview();
  previewOpen.value = true;
  previewLoading.value = true;
  previewKey.value = entry.key;
  try {
    const result = await sendCommand(target.value, "cache:get", {
      store: selectedStore.value,
      key: entry.key,
    });
    const payload = parsePayload(result) as { data?: string | null } | null;
    if (!payload || payload.data == null) {
      throw new Error("Cache preview returned no payload.");
    }
    const bytes = decodeBase64(payload.data);
    if (isProbablyText(bytes)) {
      previewText.value = bytesToText(bytes);
      previewKind.value = "text";
    } else {
      previewKind.value = "none";
    }
  } catch (err: any) {
    previewError.value = err?.message || "Unable to preview cache value.";
  } finally {
    previewLoading.value = false;
    previewingKey.value = "";
  }
};

const copyPreviewText = async () => {
  if (!previewText.value) {
    return;
  }
  try {
    await navigator.clipboard.writeText(previewText.value);
    toast.success("Cache value copied");
  } catch (err: any) {
    toast.error(err?.message || "Unable to copy cache value.");
  }
};

const copyKey = async (key: string) => {
  try {
    await navigator.clipboard.writeText(key);
    toast.success("Cache key copied");
  } catch (err: any) {
    toast.error(err?.message || "Unable to copy cache key.");
  }
};

const openCreateDialog = () => {
  createMode.value = "create";
  createError.value = "";
  createKey.value = "";
  createValue.value = "";
  createTTL.value = "";
  createOpen.value = true;
};

const editEntry = async (entry: CacheEntry) => {
  editingKey.value = entry.key;
  createMode.value = "edit";
  createError.value = "";
  createKey.value = entry.key;
  createTTL.value = "";
  createValue.value = "";
  createOpen.value = true;
  try {
    const result = await sendCommand(target.value, "cache:get", {
      store: selectedStore.value,
      key: entry.key,
    });
    const payload = parsePayload(result) as { data?: string | null } | null;
    if (!payload || payload.data == null) {
      throw new Error("Cache preview returned no payload.");
    }
    const bytes = decodeBase64(payload.data);
    if (!isProbablyText(bytes)) {
      throw new Error("Binary cache values are not editable in the browser yet.");
    }
    createValue.value = bytesToText(bytes);
  } catch (err: any) {
    createOpen.value = false;
    toast.error(err?.message || "Unable to load cache key for editing.");
  } finally {
    editingKey.value = "";
  }
};

const createEntry = async () => {
  const key = createKey.value.trim();
  if (!selectedStore.value) {
    createError.value = "Select a cache store first.";
    return;
  }
  if (!key) {
    createError.value = "Key is required.";
    return;
  }
  const ttlSeconds = Number.parseInt(createTTL.value.trim() || "0", 10);
  if (createTTL.value.trim() !== "" && (!Number.isFinite(ttlSeconds) || ttlSeconds < 0)) {
    createError.value = "TTL seconds must be a non-negative number.";
    return;
  }
  creatingKey.value = true;
  createError.value = "";
  try {
    const result = await sendCommand(target.value, "cache:set", {
      store: selectedStore.value,
      key,
      data: createValue.value,
      ttl_seconds: ttlSeconds > 0 ? ttlSeconds : 0,
    });
    if (!result?.ok) {
      throw new Error(result?.error || result?.message || (createMode.value === "edit" ? "Unable to save cache key." : "Unable to create cache key."));
    }
    toast.success(createMode.value === "edit" ? `Saved cache key ${key}` : `Created cache key ${key}`);
    createOpen.value = false;
    if (!prefix.value.trim() || key.includes(prefix.value.trim())) {
      await refresh();
    }
  } catch (err: any) {
    createError.value = err?.message || "Unable to create cache key.";
  } finally {
    creatingKey.value = false;
  }
};

const deleteEntry = async (entry: CacheEntry) => {
  const confirmed = window.confirm(`Delete cache key ${entry.key}?`);
  if (!confirmed) {
    return;
  }
  deletingKey.value = entry.key;
  try {
    const result = await sendCommand(target.value, "cache:delete", {
      store: selectedStore.value,
      key: entry.key,
    });
    if (!result?.ok) {
      throw new Error(result?.error || result?.message || "Unable to delete cache key.");
    }
    toast.success(`Deleted cache key ${entry.key}`);
    await refresh();
  } catch (err: any) {
    toast.error(err?.message || "Unable to delete cache key.");
  } finally {
    deletingKey.value = "";
  }
};

const storeLabel = (store: CacheStore) => (store.driver ? `${store.name} (${store.driver})` : store.name);

const formatBytes = (value: number) => {
  if (!Number.isFinite(value) || value <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  const exponent = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  const size = value / 1024 ** exponent;
  return `${size >= 10 || exponent === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[exponent]}`;
};

const normalizeExpiryMs = (value?: number) => {
  if (!value || !Number.isFinite(value) || value <= 0) {
    return null;
  }
  // Some legacy cache backends/records surface Unix nanoseconds.
  if (value > 1_000_000_000_000_000) {
    return Math.trunc(value / 1_000_000);
  }
  if (value > 10_000_000_000_000) {
    return Math.trunc(value / 1_000);
  }
  return Math.trunc(value);
};

const formatExpiry = (value?: number) => {
  const normalized = normalizeExpiryMs(value);
  if (!normalized) {
    return "never";
  }
  const date = new Date(normalized);
  if (Number.isNaN(date.getTime())) {
    return "never";
  }
  const deltaMs = normalized - Date.now();
  if (deltaMs <= 0) {
    return "expired";
  }
  const seconds = Math.round(deltaMs / 1000);
  if (seconds < 60) {
    return `in ${seconds}s`;
  }
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) {
    return `in ${minutes}m`;
  }
  return date.toLocaleString();
};

watch(
  () => cacheAgents.value,
  () => {
    if (!hydrated.value) {
      return;
    }
    ensureDefaultAgent();
    refresh();
  },
  { immediate: true }
);

watch(target, () => {
  if (!hydrated.value || syncingState.value) {
    return;
  }
  selectedStore.value = "";
  currentPage.value = 1;
  syncURL();
  refresh();
});

watch(selectedStore, (value, oldValue) => {
  if (!hydrated.value || syncingState.value) {
    return;
  }
  if (!value || value === oldValue) {
    return;
  }
  currentPage.value = 1;
  syncURL();
  refresh();
});

watch(prefix, async () => {
  if (!hydrated.value || syncingState.value) {
    return;
  }
  currentPage.value = 1;
  syncURL();
  await refresh();
});

watch(
  () => route.query,
  () => {
    if (!hydrated.value) {
      return;
    }
    applyURLState();
    refresh();
  }
);

onMounted(async () => {
  applyURLState();
  hydrated.value = true;
  ensureDefaultAgent();
  await refresh();
});
</script>
