<template>
  <div><section class="grid gap-6">
      <Card class="card-texture">
        <CardHeader>
          <template #title>
            <CardTitle>Invoke agent commands directly.</CardTitle>
          </template>
          <template #description>
            <CardDescription>Send a command payload to a connected agent.</CardDescription>
          </template>
        </CardHeader>
        <CardContent>
          <div class="mb-4 grid gap-4 text-xs sm:grid-cols-2 lg:grid-cols-3">
            <FormField label="Target agent">
              <Select v-model="target">
                <option value="">Select agent</option>
                <option v-for="agent in state.agents" :key="agent.source" :value="agent.source">
                  {{ agent.source }}
                </option>
              </Select>
            </FormField>
            <FormField label="Command">
              <Select v-model="command">
                <option value="">Select command</option>
                <option v-for="cmd in commands" :key="cmd.name + cmd.group" :value="cmd.name">
                  {{ cmd.name }} - {{ cmd.help }}
                </option>
              </Select>
            </FormField>
            <FormField label="Args">
              <Input
                v-model="args"
                placeholder="e.g. --all --force"
              />
            </FormField>
          </div>
          <div class="flex items-center gap-3">
            <Button variant="default" @click="run">
              <Play class="mr-1 h-3.5 w-3.5" />
              Run
            </Button>
            <span v-if="error" class="text-xs text-red-300">{{ error }}</span>
          </div>
          <div v-if="commandHelp" class="rounded-xl border border-border/60 bg-muted/40 p-4 text-xs text-muted-foreground mt-3">
            <p class="mb-2 text-[10px] uppercase tracking-[0.2em] text-muted">Args & Flags</p>
            <pre class="whitespace-pre-wrap font-mono" v-html="formatAnsi(commandHelp)"></pre>
          </div>
          <div
            v-if="output.stdout || output.stderr"
            class="mt-3 space-y-4"
          >
            <div v-if="output.stdout" class="terminal-output-block">
              <p class="terminal-output-label">Stdout</p>
              <pre class="terminal-output" v-html="formatAnsi(output.stdout).trimEnd()"></pre>
            </div>
            <div v-if="output.stderr" class="terminal-output-block">
              <p class="terminal-output-label">Stderr</p>
              <pre class="terminal-output" v-html="formatAnsi(output.stderr).trimEnd()"></pre>
            </div>
          </div>
        </CardContent>
      </Card>

    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useDevconsoleStore } from "../stores/devconsole";
import Button from "../components/ui/button/Button.vue";
import Card from "../components/ui/card/Card.vue";
import CardContent from "../components/ui/card/CardContent.vue";
import CardDescription from "../components/ui/card/CardDescription.vue";
import CardHeader from "../components/ui/card/CardHeader.vue";
import CardTitle from "../components/ui/card/CardTitle.vue";
import FormField from "../components/ui/form/FormField.vue";
import Input from "../components/ui/form/Input.vue";
import Select from "../components/ui/form/Select.vue";
import { Play } from "lucide-vue-next";

const { state, sendCommand } = useDevconsoleStore();
const target = ref(state.selectedAgent || "");
const command = ref("route:list");
const args = ref("");
const output = ref<{ stdout: string; stderr: string }>({ stdout: "", stderr: "" });
const error = ref("");
const commands = ref<{ group: string; name: string; help: string }[]>([]);
const commandHelp = ref("");


const prefill = (name: string, argsValue: string) => {
  command.value = name;
  args.value = argsValue;
};

const run = async () => {
  error.value = "";
  output.value = { stdout: "", stderr: "" };
  if (!target.value) {
    error.value = "Select an agent.";
    return;
  }
  try {
    const parsedArgs = parseArgs(args.value);
    const result = await sendCommand(target.value, "cli:run", {
      args: [command.value, ...parsedArgs],
    });
    const payload = result?.data ? (typeof result.data === "string" ? JSON.parse(result.data) : result.data) : {};
    output.value = payload.result || { stdout: "", stderr: "" };
  } catch (err: any) {
    error.value = err?.message || "Command failed.";
  }
};

const parseArgs = (raw: string) => {
  const trimmed = raw.trim();
  if (!trimmed) return [];
  const matches = trimmed.match(/(?:[^\s"]+|"[^"]*")+/g);
  if (!matches) return [];
  return matches.map((part) => part.replace(/^"|"$/g, ""));
};

const loadCommands = async () => {
  error.value = "";
  if (!target.value) {
    error.value = "Select an agent.";
    return;
  }
  try {
    const result = await sendCommand(target.value, "cli:list", {});
    if (result?.ok && result.data) {
      const payload = typeof result.data === "string" ? JSON.parse(result.data) : result.data;
      commands.value = payload.commands || [];
      if (!command.value && commands.value.length > 0) {
        command.value = commands.value[0].name;
      }
    }
  } catch (err: any) {
    error.value = err?.message || "Command list failed.";
  }
};

const loadCommandHelp = async () => {
  if (!target.value || !command.value) {
    commandHelp.value = "";
    return;
  }
  try {
    const result = await sendCommand(target.value, "cli:run", {
      args: [command.value, "--help"],
    });
    const payload = result?.data ? (typeof result.data === "string" ? JSON.parse(result.data) : result.data) : {};
    commandHelp.value = payload.result?.stdout || payload.result?.stderr || "";
  } catch {
    commandHelp.value = "";
  }
};

const formatAnsi = (value: string) => {
  if (!value) return "";
  const escapeHtml = (text: string) =>
    text
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");

  const chunks = value.split(/(\u001b\[[0-9;]*m)/g);
  let currentClass = "";
  let bold = false;
  let out = "";

  const wrap = (text: string) => {
    if (!text) return "";
    const classes = [currentClass, bold ? "font-semibold" : ""].filter(Boolean).join(" ");
    if (!classes) return escapeHtml(text);
    return `<span class="${classes}">${escapeHtml(text)}</span>`;
  };

  for (const chunk of chunks) {
    if (chunk.startsWith("\u001b[")) {
      const codes = chunk.replace("\u001b[", "").replace("m", "").split(";").filter(Boolean);
      if (codes.length === 0 || codes.includes("0")) {
        currentClass = "";
        bold = false;
        continue;
      }
      if (codes.includes("1")) {
        bold = true;
      }
      if (codes.includes("31")) currentClass = "text-red-300";
      if (codes.includes("32")) currentClass = "text-emerald-300";
      if (codes.includes("33")) currentClass = "text-amber-300";
      if (codes.includes("34")) currentClass = "text-blue-300";
      if (codes.includes("35")) currentClass = "text-fuchsia-300";
      if (codes.includes("36")) currentClass = "text-cyan-300";
      if (codes.includes("90")) currentClass = "text-muted-foreground";
      if (codes.includes("97")) currentClass = "text-foreground";
      continue;
    }
    out += wrap(chunk);
  }

  return out;
};

onMounted(() => {
  if (target.value) {
    loadCommands();
  }
});

watch(target, (value) => {
  if (value) {
    loadCommands();
  } else {
    commands.value = [];
    commandHelp.value = "";
  }
});

watch(command, () => {
  loadCommandHelp();
});
</script>
