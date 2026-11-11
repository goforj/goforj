import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

// Compile the source with the pinned compiler so tests also run on Vite's Node 20 baseline.
async function loadSource(name) {
  const source = await readFile(new URL(name, import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } });
  const code = outputText.replace('"lucide-vue-next"', JSON.stringify(import.meta.resolve("lucide-vue-next")));
  return import(`data:text/javascript;base64,${Buffer.from(code).toString("base64")}`);
}

const { inspectLogTarget, selectInspectID } = await loadSource("./inspect-links.ts");
const { appNavSections } = await loadSource("./navigation.ts");

test("Events captures have navigation and an Events-filtered inspect route", async () => {
  const inspect = appNavSections.find((section) => section.title === "Inspect");
  assert.deepEqual(inspect.items.map(({ title, url }) => [title, url]), [
    ["Requests", "/inspect/requests"],
    ["Commands", "/inspect/commands"],
    ["Jobs", "/inspect/jobs"],
    ["Events", "/inspect/events"],
    ["Schedule", "/inspect/schedule"],
  ]);
  const router = await readFile(new URL("../router.ts", import.meta.url), "utf8");
  assert.match(router, /path: "\/inspect\/events", component: InspectsView, meta: inspectMeta\("\/inspect\/events", "Events", "events"\)/);
  const view = await readFile(new URL("../views/InspectsView.vue", import.meta.url), "utf8");
  assert.match(view, /case "events":\s+return "Topic"/);
  assert.match(view, /case "events":\s+return Radio/);
  assert.match(view, /case "events":\s+return String\(inspect.labels\?\.topic/);
});

test("correlated logs link to the local capture for every inspect source", () => {
  for (const [source, path] of Object.entries({ http: "requests", cli: "commands", jobs: "jobs", events: "events", scheduler: "schedule" })) {
    assert.deepEqual(inspectLogTarget({ source, inspect_id: "capture-42", trace_id: "shared-otel-trace" }), {
      path: `/inspect/${path}`, query: { inspect: "capture-42" },
    });
  }
  assert.deepEqual(inspectLogTarget({ source: " Events ", inspect_id: " capture-42 " }), {
    path: "/inspect/events", query: { inspect: "capture-42" },
  });
});

test("logs without a supported local capture cannot create misleading links", () => {
  for (const fields of [undefined, {}, { source: "events", trace_id: "otel-only" }, { source: "events", inspect_id: " " }, { source: "startup", inspect_id: "capture" }, { source: "constructor", inspect_id: "capture" }, { source: "__proto__", inspect_id: "capture" }, { source: "toString", inspect_id: "capture" }, { source: "events", inspect_id: 42 }]) {
    assert.equal(inspectLogTarget(fields), undefined);
  }
});


test("explicit capture links survive list windows, filtering, and unavailable details", () => {
  assert.equal(selectInspectID("older-capture", "newest", ["newest"]), "older-capture");
  assert.equal(selectInspectID("in-flight", "", []), "in-flight");
  assert.equal(selectInspectID("expired", "expired", ["unrelated"], ["newest"]), "expired");
  assert.equal(selectInspectID("filtered-out", "other", ["visible"]), "filtered-out");
});

test("unlinked lists preserve a visible selection and choose a default when needed", () => {
  assert.equal(selectInspectID("", "existing", ["newest", "existing"]), "existing");
  assert.equal(selectInspectID("", "removed", ["newest"]), "newest");
  assert.equal(selectInspectID("", "", [], ["default"]), "default");
  assert.equal(selectInspectID("", "removed", []), "");
});
