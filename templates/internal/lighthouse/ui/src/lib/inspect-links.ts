const inspectPaths: Record<string, string> = {
  http: "/inspect/requests",
  cli: "/inspect/commands",
  jobs: "/inspect/jobs",
  events: "/inspect/events",
  scheduler: "/inspect/schedule",
};

// inspectLogTarget uses the local capture ID, since the OTel trace can contain many captures.
export function inspectLogTarget(fields?: Record<string, unknown>) {
  const source = typeof fields?.source === "string" ? fields.source.trim().toLowerCase() : "";
  const inspectID = typeof fields?.inspect_id === "string" ? fields.inspect_id.trim() : "";
  const path = inspectPaths[source];
  return typeof path === "string" && inspectID ? { path, query: { inspect: inspectID } } : undefined;
}

// selectInspectID preserves explicit deep links even when a capture is outside the current list window.
export function selectInspectID(requested: string, current: string, visible: string[], fallback: string[] = []) {
  if (requested) return requested;
  if (visible.includes(current)) return current;
  return visible[0] || fallback[0] || "";
}
