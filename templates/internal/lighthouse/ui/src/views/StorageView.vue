<template>
  <div>
    <section class="grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle class="inline-flex items-center gap-2">
              <FolderTree class="h-4 w-4 text-muted-foreground" />
              Storage
            </CardTitle>
          </template>
          <template #description>
            <CardDescription>Browse configured disks and download files from the connected app runtime.</CardDescription>
          </template>
          <template #action>
            <div class="flex items-center gap-2">
              <input ref="fileInputRef" type="file" class="hidden" @change="handleFileInput" />
              <Button variant="outline" size="sm" @click="createFolder">
                <FolderPlus class="mr-1 h-3.5 w-3.5" />
                New folder
              </Button>
              <Button variant="outline" size="sm" :disabled="uploading" @click="openFilePicker">
                <Upload class="mr-1 h-3.5 w-3.5" />
                {{ uploading ? "Uploading..." : "Upload" }}
              </Button>
              <RefreshButton :refreshing="loading" :on-click="refresh" />
            </div>
          </template>
        </CardHeader>
        <CardContent class="space-y-4">
          <div v-if="storageAgents.length === 0" class="rounded-xl border border-border/60 bg-muted/30 p-4 text-sm text-muted-foreground">
            No storage-capable agents are connected.
          </div>

          <template v-else>
            <div class="grid gap-4 text-xs sm:grid-cols-2 lg:grid-cols-3">
              <FormField v-if="showAgentFilter" label="Target agent">
                <Select v-model="target">
                  <option value="">Select agent</option>
                  <option v-for="agent in storageAgents" :key="agent.source" :value="agent.source">
                    {{ agent.source }}
                  </option>
                </Select>
              </FormField>
              <FormField>
                <template #label>
                  <span class="inline-flex items-center gap-1.5">
                    <HardDrive class="h-3.5 w-3.5" />
                    Disk ({{ explorer.disks.length }})
                  </span>
                </template>
                <Select v-model="selectedDisk">
                  <option v-for="disk in explorer.disks" :key="disk.name" :value="disk.name">
                    {{ diskLabel(disk) }}
                  </option>
                </Select>
              </FormField>
              <FormField>
                <template #label>
                  <span class="inline-flex items-center gap-1.5">
                    <Search class="h-3.5 w-3.5" />
                    Filter current folder
                  </span>
                </template>
                <Input v-model="query" placeholder="Filter entries..." />
              </FormField>
            </div>

            <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/60 bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
              <div>
                Showing {{ displayedEntries.length }} entries
                <span v-if="query.trim()">
                  (filtered)
                </span>
              </div>
              <div class="flex items-center gap-2">
                <span>Page {{ currentPage }}</span>
                <Button variant="outline" size="sm" :disabled="currentPage <= 1" @click="goToPage(currentPage - 1)">
                  Prev
                </Button>
                <Button variant="outline" size="sm" :disabled="query.trim() !== '' || !explorer.has_more" @click="goToPage(currentPage + 1)">
                  Next
                </Button>
              </div>
            </div>

            <div class="flex flex-wrap items-center gap-2 rounded-xl border border-border/60 bg-card/60 px-3 py-2 text-xs">
              <Button variant="outline" size="sm" :disabled="currentPath === ''" @click="goUp">
                <ArrowUp class="mr-1 h-3.5 w-3.5" />
                Up
              </Button>
              <button
                class="rounded-md px-2 py-1 font-medium text-foreground transition hover:bg-muted/50"
                @click="openPath('')"
              >
                {{ selectedDisk || "disk" }}
              </button>
              <template v-for="segment in pathSegments" :key="segment.path">
                <ChevronRight class="h-3.5 w-3.5 text-muted-foreground" />
                <button
                  class="rounded-md px-2 py-1 text-muted-foreground transition hover:bg-muted/50 hover:text-foreground"
                  @click="openPath(segment.path)"
                >
                  {{ segment.label }}
                </button>
              </template>
            </div>

            <div v-if="loadError" class="rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {{ loadError }}
            </div>
            <div v-else-if="unavailableDiskNotice" class="rounded-xl border border-amber-400/30 bg-amber-400/10 px-4 py-3 text-sm text-amber-200">
              {{ unavailableDiskNotice }}
            </div>

            <div class="max-h-[68vh] overflow-auto rounded-xl border border-border/60">
              <table class="w-full text-xs">
                <thead class="bg-muted/40 text-muted">
                  <tr>
                    <th v-if="showAgentColumn" class="px-4 py-3 text-left">
                      <span class="inline-flex items-center gap-1">
                        <Server class="h-3.5 w-3.5" />
                        Agent
                      </span>
                    </th>
                    <th class="px-4 py-3 text-left">
                      <span class="inline-flex items-center gap-1">
                        <FolderTree class="h-3.5 w-3.5" />
                        Name
                      </span>
                    </th>
                    <th class="px-4 py-3 text-left">
                      <span class="inline-flex items-center gap-1">
                        <Route class="h-3.5 w-3.5" />
                        Path
                      </span>
                    </th>
                    <th class="px-4 py-3 text-left">
                      <span class="inline-flex items-center gap-1">
                        <FileType class="h-3.5 w-3.5" />
                        Type
                      </span>
                    </th>
                    <th class="px-4 py-3 text-left">
                      <span class="inline-flex items-center gap-1">
                        <HardDrive class="h-3.5 w-3.5" />
                        Size
                      </span>
                    </th>
                    <th class="px-4 py-3 text-left">
                      <span class="inline-flex items-center gap-1">
                        <Download class="h-3.5 w-3.5" />
                        Actions
                      </span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-if="displayedEntries.length === 0" class="border-t border-border/60">
                    <td :colspan="showAgentColumn ? 6 : 5" class="px-4 py-3 text-muted">
                      No files found.
                    </td>
                  </tr>
                  <tr
                    v-for="entry in displayedEntries"
                    :key="`${selectedDisk}:${entry.path}`"
                    class="group border-t border-border/60 transition hover:bg-muted/20"
                  >
                    <td v-if="showAgentColumn" class="px-4 py-2.5 text-foreground">
                      {{ target }}
                    </td>
                    <td class="px-4 py-2.5">
                      <button
                        v-if="entry.is_dir"
                        class="inline-flex cursor-pointer items-center gap-2 text-foreground transition hover:text-primary"
                        @click="openPath(entry.path)"
                      >
                        <Folder class="h-4 w-4 text-amber-300" />
                        {{ entry.name }}
                      </button>
                      <span v-else class="inline-flex items-center gap-2 text-foreground">
                        <button
                          v-if="thumbnailURL(entry)"
                          class="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md border border-border/60 bg-muted/20 p-0.5 transition hover:bg-muted/40"
                          @click="previewEntry(entry)"
                        >
                          <img
                            :src="thumbnailURL(entry)"
                            :alt="entry.name"
                            class="block h-5 w-5 rounded-sm bg-background/80 object-contain"
                          />
                        </button>
                        <span
                          v-else
                          class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border/60 bg-muted/20 text-muted-foreground"
                        >
                          <File class="h-4 w-4" />
                        </span>
                        {{ entry.name }}
                      </span>
                    </td>
                    <td class="px-4 py-2.5 font-mono text-muted">{{ entry.path }}</td>
                    <td class="px-4 py-2.5 text-muted">{{ entry.is_dir ? "Directory" : "File" }}</td>
                    <td class="px-4 py-2.5 text-muted">{{ entry.is_dir ? "—" : formatBytes(entry.size) }}</td>
                    <td class="px-4 py-2.5">
                      <div class="flex items-center gap-2">
                        <Button v-if="entry.is_dir" variant="outline" size="sm" @click="openPath(entry.path)">
                          <Folder class="mr-1 h-3.5 w-3.5" />
                          Open
                        </Button>
                        <Button
                          v-else
                          variant="outline"
                          size="sm"
                          @click="previewEntry(entry)"
                        >
                          <Eye class="mr-1 h-3.5 w-3.5" />
                          Preview
                        </Button>
                        <Button
                          v-if="!entry.is_dir && entry.url_capable"
                          variant="outline"
                          size="sm"
                          :disabled="urlPath === entry.path"
                          @click="copyURL(entry)"
                        >
                          <Link2 class="mr-1 h-3.5 w-3.5" />
                          {{ urlPath === entry.path ? "Resolving..." : "Copy URL" }}
                        </Button>
                        <Button
                          v-if="!entry.is_dir"
                          variant="outline"
                          size="sm"
                          :disabled="downloadingPath === entry.path"
                          @click="downloadEntry(entry)"
                        >
                          <Download class="mr-1 h-3.5 w-3.5" />
                          {{ downloadingPath === entry.path ? "Downloading..." : "Download" }}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          :disabled="movingPath === entry.path"
                          @click="renameEntry(entry)"
                        >
                          <Pencil class="mr-1 h-3.5 w-3.5" />
                          {{ movingPath === entry.path ? "Renaming..." : "Rename" }}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          :disabled="deletingPath === entry.path"
                          @click="deleteEntry(entry)"
                        >
                          <Trash2 class="mr-1 h-3.5 w-3.5" />
                          {{ deletingPath === entry.path ? "Deleting..." : "Delete" }}
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
            <DialogTitle>{{ previewName || "Preview" }}</DialogTitle>
            <DialogDescription class="font-mono text-xs">
              {{ previewPath }}
            </DialogDescription>
          </DialogHeader>
          <div v-if="!previewLoading && !previewError && previewKind === 'text'" class="flex justify-end">
            <Button variant="outline" size="sm" @click="copyPreviewText">
              <Copy class="mr-1 h-3.5 w-3.5" />
              Copy text
            </Button>
          </div>
          <div v-if="previewError" class="rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {{ previewError }}
          </div>
          <div v-else-if="previewLoading" class="rounded-xl border border-border/60 bg-muted/30 px-4 py-8 text-center text-sm text-muted-foreground">
            Loading preview...
          </div>
          <div v-else-if="previewKind === 'image'" class="overflow-auto rounded-xl border border-border/60 bg-card/60 p-4">
            <img :src="previewImageSrc" :alt="previewName" class="mx-auto max-h-[70vh] rounded-lg object-contain" />
          </div>
          <div v-else-if="previewKind === 'text'" class="overflow-auto rounded-xl border border-border/60 bg-card/60 p-4">
            <pre
              class="storage-preview whitespace-pre-wrap break-words text-xs text-foreground"
              v-html="previewTextHTML"
            ></pre>
          </div>
          <div v-else class="rounded-xl border border-border/60 bg-muted/30 px-4 py-8 text-center text-sm text-muted-foreground">
            Preview is not available for this file type.
          </div>
        </DialogContent>
      </Dialog>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { toast } from "vue-sonner";
import { useRoute, useRouter } from "vue-router";
import {
  ArrowUp,
  ChevronRight,
  Copy,
  Download,
  Eye,
  File,
  FileType,
  Search,
  Folder,
  FolderPlus,
  FolderTree,
  HardDrive,
  Link2,
  Pencil,
  Route,
  Server,
  Trash2,
  Upload,
} from "lucide-vue-next";
import { lighthousePath } from "../lib/base-path";
import { useLighthouseStore } from "../stores/lighthouse";
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
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "../components/ui/dialog";

type StorageAgent = {
  source: string;
  capabilities: string[];
};

type StorageDisk = {
  name: string;
  driver?: string;
  url_capable?: boolean;
  is_default: boolean;
};

type StorageEntry = {
  name: string;
  path: string;
  size: number;
  is_dir: boolean;
  downloadable: boolean;
  url_capable: boolean;
};

type ExplorerPayload = {
  disks: StorageDisk[];
  current_disk: string;
  path: string;
  parent: string;
  offset?: number;
  limit?: number;
  has_more?: boolean;
  backend_paged?: boolean;
  entries: StorageEntry[];
};

const STORAGE_PREVIEW_MAX_BYTES = 2 * 1024 * 1024;
const STORAGE_UPLOAD_MAX_BYTES = 10 * 1024 * 1024;

const { state, sendCommand } = useLighthouseStore();
const route = useRoute();
const router = useRouter();
const loading = ref(false);
const loadError = ref("");
const query = ref("");
const fileInputRef = ref<HTMLInputElement | null>(null);
const target = ref(state.selectedAgent || "");
const selectedDisk = ref("");
const currentPath = ref("");
const downloadingPath = ref("");
const uploading = ref(false);
const urlPath = ref("");
const deletingPath = ref("");
const movingPath = ref("");
const syncingState = ref(false);
const hydrated = ref(false);
const currentPage = ref(1);
const previewOpen = ref(false);
const previewLoading = ref(false);
const previewError = ref("");
const previewName = ref("");
const previewPath = ref("");
const previewText = ref("");
const previewImageSrc = ref("");
const previewContentType = ref("");
const previewKind = ref<"none" | "text" | "image">("none");
const imageThumbs = ref<Record<string, string>>({});
const skippedThumbs = ref<Record<string, true>>({});
const unavailableDiskNotice = ref("");
const explorer = ref<ExplorerPayload>({
  disks: [],
  current_disk: "",
  path: "",
  parent: "",
  offset: 0,
  limit: 0,
  has_more: false,
  backend_paged: false,
  entries: [],
});

const storageAgents = computed<StorageAgent[]>(() =>
  state.agents.filter((agent) => agent.capabilities.includes("storage"))
);

const showAgentFilter = computed(() => storageAgents.value.length > 1);
const showAgentColumn = computed(() => storageAgents.value.length > 1);

const filteredEntries = computed(() => {
  const needle = query.value.trim().toLowerCase();
  if (!needle) {
    return explorer.value.entries;
  }
  return explorer.value.entries.filter((entry) => {
    return entry.name.toLowerCase().includes(needle) || entry.path.toLowerCase().includes(needle);
  });
});

const pageSize = 100;

const displayedEntries = computed(() => (query.value.trim() ? filteredEntries.value : explorer.value.entries));

const currentPathLabel = computed(() => (currentPath.value.trim() === "" ? "/" : `/${currentPath.value}`));

const pathSegments = computed(() => {
  if (!currentPath.value) {
    return [];
  }
  const parts = currentPath.value.split("/").filter(Boolean);
  return parts.map((label, index) => ({
    label,
    path: parts.slice(0, index + 1).join("/"),
  }));
});

const parsePayload = (result: any) => {
  if (!result?.ok || !result.data) {
    return null;
  }
  return typeof result.data === "string" ? JSON.parse(result.data) : result.data;
};

const imageExtensionPattern = /\.(avif|bmp|gif|ico|jpe?g|png|svg|webp)$/i;

const isImageEntry = (entry: StorageEntry) => !entry.is_dir && imageExtensionPattern.test(entry.path);

const thumbnailURL = (entry: StorageEntry) => imageThumbs.value[entry.path] || "";

const readQueryValue = (value: unknown) => {
  if (Array.isArray(value)) {
    return String(value[0] || "").trim();
  }
  return typeof value === "string" ? value.trim() : "";
};

const syncURL = () => {
  const nextQuery: Record<string, string> = {};
  if (target.value) nextQuery.agent = target.value;
  if (selectedDisk.value) nextQuery.disk = selectedDisk.value;
  if (currentPath.value) nextQuery.path = currentPath.value;
  if (query.value.trim()) nextQuery.q = query.value.trim();
  if (currentPage.value > 1) nextQuery.page = String(currentPage.value);
  const current = route.query;
  const currentAgent = readQueryValue(current.agent);
  const currentDisk = readQueryValue(current.disk);
  const currentPathValue = readQueryValue(current.path);
  const currentSearch = readQueryValue(current.q);
  const currentPageValue = readQueryValue(current.page);
  const nextAgent = nextQuery.agent || "";
  const nextDisk = nextQuery.disk || "";
  const nextPathValue = nextQuery.path || "";
  const nextSearch = nextQuery.q || "";
  const nextPageValue = nextQuery.page || "";

  if (
    currentAgent === nextAgent &&
    currentDisk === nextDisk &&
    currentPathValue === nextPathValue &&
    currentSearch === nextSearch &&
    currentPageValue === nextPageValue
  ) {
    return;
  }

  const onlySearchChanged =
    currentAgent === nextAgent &&
    currentDisk === nextDisk &&
    currentPathValue === nextPathValue &&
    currentSearch !== nextSearch &&
    currentPageValue === nextPageValue;

  const navigate = onlySearchChanged ? router.replace : router.push;
  navigate.call(router, {
    path: route.path,
    query: nextQuery,
  });
};

const applyURLState = () => {
  const nextAgent = readQueryValue(route.query.agent);
  const nextDisk = readQueryValue(route.query.disk);
  const nextPath = readQueryValue(route.query.path);
  const nextQuery = readQueryValue(route.query.q);
  const nextPage = Number.parseInt(readQueryValue(route.query.page) || "1", 10);

  syncingState.value = true;
  if (nextAgent) {
    target.value = nextAgent;
  }
  if (nextDisk) {
    selectedDisk.value = nextDisk;
  }
  currentPath.value = nextPath;
  query.value = nextQuery;
  currentPage.value = Number.isFinite(nextPage) && nextPage > 0 ? nextPage : 1;
  syncingState.value = false;
};

const ensureDefaultAgent = () => {
  if (storageAgents.value.length === 0) {
    target.value = "";
    return;
  }
  if (target.value && storageAgents.value.some((agent) => agent.source === target.value)) {
    return;
  }
  const selected = state.selectedAgent;
  if (selected && storageAgents.value.some((agent) => agent.source === selected)) {
    target.value = selected;
    return;
  }
  target.value = storageAgents.value[0].source;
};

const refresh = async () => {
  ensureDefaultAgent();
  loadError.value = "";
  unavailableDiskNotice.value = "";
  if (!target.value) {
    explorer.value = { disks: [], current_disk: "", path: "", parent: "", offset: 0, limit: 0, has_more: false, backend_paged: false, entries: [] };
    syncingState.value = true;
    selectedDisk.value = "";
    currentPath.value = "";
    syncingState.value = false;
    return;
  }
  loading.value = true;
  try {
    const requestedDisk = selectedDisk.value.trim();
    const usingBackendPaging = query.value.trim() === "";
    const result = await sendCommand(target.value, "storage:list", {
      disk: requestedDisk,
      path: currentPath.value,
      offset: usingBackendPaging ? (currentPage.value - 1) * pageSize : 0,
      limit: usingBackendPaging ? pageSize : 0,
    });
    const payload = parsePayload(result) as ExplorerPayload | null;
    if (!payload) {
      if (requestedDisk) {
        syncingState.value = true;
        selectedDisk.value = "";
        currentPath.value = "";
        explorer.value = { disks: [], current_disk: "", path: "", parent: "", offset: 0, limit: 0, has_more: false, backend_paged: false, entries: [] };
        syncingState.value = false;
        unavailableDiskNotice.value = `Disk "${requestedDisk}" is unavailable right now.`;
        syncURL();
        return;
      }
      throw new Error("Storage explorer returned no payload.");
    }
    syncingState.value = true;
    explorer.value = payload;
    const nextDisk = payload.current_disk || payload.disks[0]?.name || "";
    selectedDisk.value = nextDisk;
    currentPath.value = nextDisk ? payload.path || "" : "";
    syncingState.value = false;
    if (requestedDisk && !nextDisk) {
      unavailableDiskNotice.value = `Disk "${requestedDisk}" is unavailable right now.`;
    }
    syncURL();
  } catch (err: any) {
    const message = err?.message || "Unable to load storage explorer.";
    if (message.includes("storage: disk")) {
      syncingState.value = true;
      selectedDisk.value = "";
      currentPath.value = "";
      explorer.value = { disks: [], current_disk: "", path: "", parent: "", offset: 0, limit: 0, has_more: false, backend_paged: false, entries: [] };
      syncingState.value = false;
      syncURL();
    }
    loadError.value = message;
  } finally {
    loading.value = false;
  }
};

const openPath = async (nextPath: string) => {
  currentPath.value = nextPath || "";
  currentPage.value = 1;
  await refresh();
};

const goUp = async () => {
  currentPath.value = explorer.value.parent || "";
  currentPage.value = 1;
  await refresh();
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

const encodeBase64 = (bytes: Uint8Array) => {
  let binary = "";
  for (let i = 0; i < bytes.length; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }
  return window.btoa(binary);
};

const resetPreview = () => {
  previewLoading.value = false;
  previewError.value = "";
  previewName.value = "";
  previewPath.value = "";
  previewText.value = "";
  previewContentType.value = "";
  if (previewImageSrc.value) {
    window.URL.revokeObjectURL(previewImageSrc.value);
  }
  previewImageSrc.value = "";
  previewKind.value = "none";
};

const revokeThumbnail = (url: string) => {
  if (!url || !url.startsWith("blob:")) {
    return;
  }
  window.URL.revokeObjectURL(url);
};

const bytesToText = (bytes: Uint8Array) => {
  try {
    return new TextDecoder("utf-8").decode(bytes);
  } catch {
    return "";
  }
};

const sniffImageContentType = (bytes: Uint8Array, entryPath: string, reportedType: string) => {
  if (reportedType.startsWith("image/")) {
    return reportedType;
  }
  if (bytes.length >= 8) {
    if (
      bytes[0] === 0x89 &&
      bytes[1] === 0x50 &&
      bytes[2] === 0x4e &&
      bytes[3] === 0x47 &&
      bytes[4] === 0x0d &&
      bytes[5] === 0x0a &&
      bytes[6] === 0x1a &&
      bytes[7] === 0x0a
    ) {
      return "image/png";
    }
  }
  if (bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) {
    return "image/jpeg";
  }
  if (bytes.length >= 6) {
    const header = String.fromCharCode(...bytes.slice(0, 6));
    if (header === "GIF87a" || header === "GIF89a") {
      return "image/gif";
    }
  }
  if (
    bytes.length >= 12 &&
    String.fromCharCode(...bytes.slice(0, 4)) === "RIFF" &&
    String.fromCharCode(...bytes.slice(8, 12)) === "WEBP"
  ) {
    return "image/webp";
  }
  if (bytes.length >= 2 && bytes[0] === 0x42 && bytes[1] === 0x4d) {
    return "image/bmp";
  }
  if (
    bytes.length >= 4 &&
    bytes[0] === 0x00 &&
    bytes[1] === 0x00 &&
    bytes[2] === 0x01 &&
    bytes[3] === 0x00
  ) {
    return "image/x-icon";
  }
  const textPrefix = bytesToText(bytes.slice(0, Math.min(bytes.length, 512))).trimStart().toLowerCase();
  if (textPrefix.startsWith("<svg") || textPrefix.startsWith("<?xml")) {
    return "image/svg+xml";
  }
  if (imageExtensionPattern.test(entryPath)) {
    const normalized = entryPath.toLowerCase();
    if (normalized.endsWith(".png")) return "image/png";
    if (normalized.endsWith(".jpg") || normalized.endsWith(".jpeg")) return "image/jpeg";
    if (normalized.endsWith(".gif")) return "image/gif";
    if (normalized.endsWith(".webp")) return "image/webp";
    if (normalized.endsWith(".bmp")) return "image/bmp";
    if (normalized.endsWith(".svg")) return "image/svg+xml";
    if (normalized.endsWith(".ico")) return "image/x-icon";
    if (normalized.endsWith(".avif")) return "image/avif";
  }
  return "";
};

const blobCanRenderAsImage = (blob: Blob) =>
  new Promise<boolean>((resolve) => {
    const objectURL = window.URL.createObjectURL(blob);
    const image = new Image();
    image.onload = () => {
      window.URL.revokeObjectURL(objectURL);
      resolve(true);
    };
    image.onerror = () => {
      window.URL.revokeObjectURL(objectURL);
      resolve(false);
    };
    image.src = objectURL;
  });

const isTextContentType = (value: string) => {
  return (
    value.startsWith("text/") ||
    value.includes("json") ||
    value.includes("xml") ||
    value.includes("yaml") ||
    value.includes("javascript")
  );
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
  if (previewKind.value !== "text") {
    return "";
  }
  if (previewContentType.value.includes("json")) {
    return highlightJSON(formatJSONPreview(previewText.value));
  }
  return escapeHTML(previewText.value);
});

const fetchStorageFile = async (entryPath: string) => {
  const result = await sendCommand(target.value, "storage:get", {
    disk: selectedDisk.value,
    path: entryPath,
  });
  const payload = parsePayload(result) as
    | { name: string; content_type?: string; data: string }
    | null;
  if (!payload || typeof payload.data !== "string") {
    throw new Error("Storage file returned no contents.");
  }
  return payload;
};

const copyPreviewText = async () => {
  if (previewKind.value !== "text" || !previewText.value) {
    return;
  }
  try {
    await navigator.clipboard.writeText(previewText.value);
    toast.success("Preview text copied");
  } catch (err: any) {
    toast.error(err?.message || "Unable to copy preview text.");
  }
};

const buildStorageDownloadURL = (entryPath: string) => {
  const params = new URLSearchParams({
    disk: selectedDisk.value,
    path: entryPath,
  });
  return `${lighthousePath("/api/storage/download")}?${params.toString()}`;
};

const loadImageThumbs = async () => {
  const candidateEntries = explorer.value.entries.filter((entry) => !entry.is_dir).slice(0, 24);
  const keep = new Set(candidateEntries.map((entry) => entry.path));
  const nextThumbs: Record<string, string> = {};
  const nextSkipped: Record<string, true> = {};

  Object.entries(imageThumbs.value).forEach(([path, url]) => {
    if (keep.has(path)) {
      nextThumbs[path] = url;
      return;
    }
    revokeThumbnail(url);
  });

  if (candidateEntries.length === 0) {
    imageThumbs.value = nextThumbs;
    skippedThumbs.value = nextSkipped;
    return;
  }

  await Promise.all(
    candidateEntries.map(async (entry) => {
      if (nextThumbs[entry.path] || nextSkipped[entry.path]) {
        return;
      }
      try {
        const payload = await fetchStorageFile(entry.path);
        const contentType = payload.content_type || "";
        const bytes = decodeBase64(payload.data);
        const imageContentType = sniffImageContentType(bytes, entry.path, contentType);
        if (imageContentType) {
          nextThumbs[entry.path] = `data:${imageContentType};base64,${payload.data}`;
          return;
        }
        const blob = new Blob([bytes], { type: contentType || "application/octet-stream" });
        const isImage = await blobCanRenderAsImage(blob);
        if (!isImage) {
          nextSkipped[entry.path] = true;
          return;
        }
        nextThumbs[entry.path] = window.URL.createObjectURL(blob);
      } catch {
        nextSkipped[entry.path] = true;
        return;
      }
    })
  );
  imageThumbs.value = nextThumbs;
  skippedThumbs.value = nextSkipped;
};

const previewEntry = async (entry: StorageEntry) => {
  if (entry.is_dir) {
    return;
  }
  if (entry.size > STORAGE_PREVIEW_MAX_BYTES) {
    toast.error(`Preview limit exceeded (${formatBytes(entry.size)} > ${formatBytes(STORAGE_PREVIEW_MAX_BYTES)}).`);
    return;
  }
  resetPreview();
  previewOpen.value = true;
  previewLoading.value = true;
  previewName.value = entry.name;
  previewPath.value = entry.path;
  try {
    const payload = await fetchStorageFile(entry.path);
    const bytes = decodeBase64(payload.data);
    const contentType = payload.content_type || "application/octet-stream";
    previewContentType.value = contentType;
    if (contentType.startsWith("image/")) {
      const blob = new Blob([bytes], { type: contentType });
      previewImageSrc.value = window.URL.createObjectURL(blob);
      previewKind.value = "image";
    } else if (isTextContentType(contentType)) {
      previewText.value = bytesToText(bytes);
      previewKind.value = "text";
    } else {
      previewKind.value = "none";
    }
  } catch (err: any) {
    previewError.value = err?.message || "Unable to preview file.";
  } finally {
    previewLoading.value = false;
  }
};

const copyURL = async (entry: StorageEntry) => {
  if (entry.is_dir) {
    return;
  }
  urlPath.value = entry.path;
  try {
    const result = await sendCommand(target.value, "storage:url", {
      disk: selectedDisk.value,
      path: entry.path,
    });
    const payload = parsePayload(result) as { url?: string } | null;
    if (!payload?.url) {
      throw new Error("Public URLs are unavailable for this storage disk.");
    }
    await navigator.clipboard.writeText(payload.url);
    toast.success("Public URL copied");
  } catch (err: any) {
    toast.error(err?.message || "Unable to copy file URL.");
  } finally {
    urlPath.value = "";
  }
};

const renameEntry = async (entry: StorageEntry) => {
  const currentName = entry.name.trim();
  const noun = entry.is_dir ? "folder" : "file";
  const nextName = window.prompt(`Rename ${noun}`, currentName)?.trim();
  if (!nextName || nextName === currentName) {
    return;
  }
  if (nextName.includes("/")) {
    toast.error("Rename only supports changing the final name segment.");
    return;
  }
  const parent = entry.path.includes("/") ? entry.path.slice(0, entry.path.lastIndexOf("/")) : "";
  const nextPath = parent ? `${parent}/${nextName}` : nextName;
  const existingEntry = explorer.value.entries.find((candidate) => candidate.path === nextPath);
  if (existingEntry) {
    const existingNoun = existingEntry.is_dir ? "folder" : "file";
    toast.error(`A ${existingNoun} already exists at ${nextPath}.`);
    return;
  }
  movingPath.value = entry.path;
  try {
    const result = await sendCommand(target.value, "storage:move", {
      disk: selectedDisk.value,
      from: entry.path,
      to: nextPath,
    });
    const payload = parsePayload(result);
    if (!payload) {
      throw new Error("Rename returned no response payload.");
    }
    toast.success(`Renamed ${noun} ${currentName} to ${nextName}`);
    await refresh();
  } catch (err: any) {
    toast.error(err?.message || `Unable to rename ${noun}.`);
  } finally {
    movingPath.value = "";
  }
};

const createFolder = async () => {
  if (!target.value || !selectedDisk.value) {
    toast.error("Select an agent and disk before creating a folder.");
    return;
  }
  const folderName = window.prompt("Create folder", "")?.trim();
  if (!folderName) {
    return;
  }
  if (folderName.includes("/")) {
    toast.error("Folder name cannot include path separators.");
    return;
  }
  const nextPath = currentPath.value ? `${currentPath.value.replace(/\/+$/, "")}/${folderName}` : folderName;
  const existingEntry = explorer.value.entries.find((entry) => entry.path === nextPath);
  if (existingEntry) {
    const noun = existingEntry.is_dir ? "folder" : "file";
    toast.error(`A ${noun} already exists at ${nextPath}.`);
    return;
  }
  try {
    const result = await sendCommand(target.value, "storage:mkdir", {
      disk: selectedDisk.value,
      path: nextPath,
    });
    const payload = parsePayload(result);
    if (!payload) {
      throw new Error("Create folder returned no response payload.");
    }
    toast.success(`Created folder ${folderName}`);
    await refresh();
  } catch (err: any) {
    toast.error(err?.message || "Unable to create folder.");
  }
};

const openFilePicker = () => {
  fileInputRef.value?.click();
};

const readFileBytes = async (file: File) => {
  const buffer = await file.arrayBuffer();
  return new Uint8Array(buffer);
};

const uploadFile = async (file: File) => {
  if (!target.value || !selectedDisk.value) {
    toast.error("Select an agent and disk before uploading.");
    return;
  }
  if (file.size > STORAGE_UPLOAD_MAX_BYTES) {
    toast.error(`Upload limit exceeded (${formatBytes(file.size)} > ${formatBytes(STORAGE_UPLOAD_MAX_BYTES)}).`);
    return;
  }
  const existingEntry = explorer.value.entries.find((entry) => entry.name === file.name);
  if (existingEntry) {
    const noun = existingEntry.is_dir ? "directory" : "file";
    const confirmed = window.confirm(`Overwrite existing ${noun} ${existingEntry.path}?`);
    if (!confirmed) {
      if (fileInputRef.value) {
        fileInputRef.value.value = "";
      }
      return;
    }
  }
  uploading.value = true;
  try {
    const bytes = await readFileBytes(file);
    const basePath = currentPath.value ? `${currentPath.value.replace(/\/+$/, "")}/` : "";
    const result = await sendCommand(target.value, "storage:put", {
      disk: selectedDisk.value,
      path: `${basePath}${file.name}`,
      data: encodeBase64(bytes),
      content_type: file.type || "application/octet-stream",
    });
    const payload = parsePayload(result);
    if (!payload) {
      throw new Error("Upload returned no response payload.");
    }
    toast.success(`Uploaded ${file.name}`);
    await refresh();
  } catch (err: any) {
    toast.error(err?.message || "Unable to upload file.");
  } finally {
    uploading.value = false;
    if (fileInputRef.value) {
      fileInputRef.value.value = "";
    }
  }
};

const handleFileInput = async (event: Event) => {
  const targetEl = event.target as HTMLInputElement | null;
  const file = targetEl?.files?.[0];
  if (!file) {
    return;
  }
  await uploadFile(file);
};

const deleteEntry = async (entry: StorageEntry) => {
  const noun = entry.is_dir ? "folder" : "file";
  const confirmed = window.confirm(`Delete ${noun} ${entry.path}?`);
  if (!confirmed) {
    return;
  }
  deletingPath.value = entry.path;
  try {
    const result = await sendCommand(target.value, "storage:delete", {
      disk: selectedDisk.value,
      path: entry.path,
    });
    if (!result?.ok) {
      throw new Error(result?.error || result?.message || `Unable to delete ${noun}.`);
    }
    toast.success(`Deleted ${noun} ${entry.name}`);
    await refresh();
  } catch (err: any) {
    toast.error(err?.message || `Unable to delete ${noun}.`);
  } finally {
    deletingPath.value = "";
  }
};

const downloadEntry = async (entry: StorageEntry) => {
  if (entry.is_dir) {
    return;
  }
  downloadingPath.value = entry.path;
  try {
    const response = await fetch(buildStorageDownloadURL(entry.path), {
      credentials: "same-origin",
    });
    if (!response.ok) {
      let message = "Unable to download file.";
      const contentType = response.headers.get("content-type") || "";
      if (contentType.includes("application/json")) {
        const payload = await response.json().catch(() => null);
        if (payload?.error) {
          message = String(payload.error);
        }
      }
      throw new Error(message);
    }
    const blob = await response.blob();
    const url = window.URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = entry.name;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    window.URL.revokeObjectURL(url);
    toast.success(`Downloaded ${entry.name}`);
  } catch (err: any) {
    toast.error(err?.message || "Unable to download file.");
  } finally {
    downloadingPath.value = "";
  }
};

const formatBytes = (value: number) => {
  if (!Number.isFinite(value) || value <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  const exponent = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  const size = value / 1024 ** exponent;
  return `${size >= 10 || exponent === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[exponent]}`;
};

const diskLabel = (disk: StorageDisk) => {
  if (!disk.driver) {
    return disk.name;
  }
  return `${disk.name} (${disk.driver})`;
};

watch(
  () => storageAgents.value,
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
  if (!hydrated.value) {
    return;
  }
  if (syncingState.value) {
    return;
  }
  currentPath.value = "";
  selectedDisk.value = "";
  syncURL();
  refresh();
});

watch(selectedDisk, (value, oldValue) => {
  if (!hydrated.value) {
    return;
  }
  if (syncingState.value) {
    return;
  }
  if (!value || value === oldValue) {
    return;
  }
  currentPath.value = "";
  currentPage.value = 1;
  syncURL();
  refresh();
});

watch(currentPath, () => {
  if (!hydrated.value) {
    return;
  }
  if (syncingState.value) {
    return;
  }
  syncURL();
});

watch(query, async (value, oldValue) => {
  if (!hydrated.value) {
    return;
  }
  if (syncingState.value) {
    return;
  }
  currentPage.value = 1;
  syncURL();
  const previous = oldValue.trim();
  const next = value.trim();
  if ((previous === "" && next !== "") || (previous !== "" && next === "")) {
    await refresh();
  }
});

watch(
  () => route.query,
  () => {
    if (!hydrated.value) {
      return;
    }
    const nextAgent = readQueryValue(route.query.agent);
    const nextDisk = readQueryValue(route.query.disk);
    const nextPath = readQueryValue(route.query.path);
    const nextSearch = readQueryValue(route.query.q);
    const nextPage = readQueryValue(route.query.page);
    if (
      nextAgent === (target.value || "") &&
      nextDisk === (selectedDisk.value || "") &&
      nextPath === (currentPath.value || "") &&
      nextSearch === query.value.trim() &&
      nextPage === (currentPage.value > 1 ? String(currentPage.value) : "")
    ) {
      return;
    }
    applyURLState();
    ensureDefaultAgent();
    refresh();
  }
);

onMounted(async () => {
  applyURLState();
  ensureDefaultAgent();
  await refresh();
  await nextTick();
  hydrated.value = true;
});

watch(
  () => [explorer.value.current_disk, explorer.value.path, explorer.value.entries] as const,
  () => {
    loadImageThumbs();
  }
);

watch(previewOpen, (value) => {
  if (!value) {
    resetPreview();
  }
});
</script>

<style scoped>
.storage-preview :deep(.storage-preview-key) {
  color: color-mix(in oklab, var(--primary) 78%, white 22%);
}

.storage-preview :deep(.storage-preview-string) {
  color: #9bd3ff;
}

.storage-preview :deep(.storage-preview-number) {
  color: #f4c97a;
}

.storage-preview :deep(.storage-preview-boolean) {
  color: #a7f3d0;
}

.storage-preview :deep(.storage-preview-null) {
  color: #fda4af;
}
</style>
