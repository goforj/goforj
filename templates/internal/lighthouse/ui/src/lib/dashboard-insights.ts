type RouteEntryLike = {
  path: string;
  handler?: string;
  methods?: string[];
  source?: string;
};

type ScheduleEntryLike = {
  id: string;
  name?: string;
  next?: string;
  next_run?: string;
  tags?: string[];
  paused?: boolean;
};

export const summarizeRoutes = (routes: RouteEntryLike[]) => {
  const methodCounts = new Map<string, number>();
  const prefixCounts = new Map<string, number>();
  const handlers = new Set<string>();
  const agents = new Set<string>();
  let dynamic = 0;

  for (const route of routes) {
    const handler = String(route.handler || "").trim();
    if (handler) {
      handlers.add(handler);
    }
    if (route.source) {
      agents.add(route.source);
    }
    if (route.path.includes(":") || route.path.includes("*") || route.path.includes("{")) {
      dynamic += 1;
    }

    const segments = route.path.split("/").filter(Boolean);
    const prefix = segments[0] ? `/${segments[0]}` : "/";
    prefixCounts.set(prefix, (prefixCounts.get(prefix) || 0) + 1);

    for (const method of route.methods || []) {
      methodCounts.set(method, (methodCounts.get(method) || 0) + 1);
    }
  }

  const methodBreakdown = Array.from(methodCounts.entries())
    .map(([label, count]) => ({ label, count }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label));

  const prefixBreakdown = Array.from(prefixCounts.entries())
    .map(([label, count]) => ({ label, count }))
    .filter((entry) => entry.label !== "/" || entry.count > 1)
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label));

  return {
    total: routes.length,
    handlers: handlers.size,
    agents: agents.size,
    dynamic,
    methodBreakdown: methodBreakdown.slice(0, 5),
    prefixBreakdown: prefixBreakdown.slice(0, 3),
  };
};

export const methodDotClass = (method: string) => {
  switch (method.toUpperCase()) {
    case "GET":
      return "dashboard-metric-dot-get";
    case "POST":
      return "dashboard-metric-dot-post";
    case "PUT":
      return "dashboard-metric-dot-put";
    case "PATCH":
      return "dashboard-metric-dot-patch";
    case "DELETE":
      return "dashboard-metric-dot-delete";
    default:
      return "dashboard-metric-dot-default";
  }
};

const parseRelativeDurationMs = (value: string) => {
  const normalized = value.trim().toLowerCase().replace(/^in\s+/, "");
  if (!normalized) {
    return null;
  }

  let total = 0;
  let matched = false;
  const pattern = /(\d+)\s*(d|h|m|s)/g;

  for (const match of normalized.matchAll(pattern)) {
    matched = true;
    const amount = Number(match[1]);
    const unit = match[2];

    switch (unit) {
      case "d":
        total += amount * 24 * 60 * 60 * 1000;
        break;
      case "h":
        total += amount * 60 * 60 * 1000;
        break;
      case "m":
        total += amount * 60 * 1000;
        break;
      case "s":
        total += amount * 1000;
        break;
    }
  }

  return matched ? total : null;
};

const parseScheduleTime = (schedule: Pick<ScheduleEntryLike, "next" | "next_run">) => {
  const value = schedule.next || schedule.next_run || "";
  const relativeMs = parseRelativeDurationMs(value);
  if (relativeMs !== null) {
    return Date.now() + relativeMs;
  }

  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) {
    return null;
  }
  return parsed;
};

export const summarizeSchedules = (schedules: ScheduleEntryLike[]) => {
  const upcoming = [...schedules]
    .map((schedule) => ({ schedule, parsed: parseScheduleTime(schedule) }))
    .sort((a, b) => {
      if (a.parsed === null && b.parsed === null) return 0;
      if (a.parsed === null) return 1;
      if (b.parsed === null) return -1;
      return a.parsed - b.parsed;
    })
    .slice(0, 5)
    .map(({ schedule }) => ({
      id: schedule.id,
      name: String(schedule.name || "Unnamed schedule").trim() || "Unnamed schedule",
      next: schedule.next || schedule.next_run || "Run time unavailable",
    }));

  let paused = 0;
  let tagged = 0;

  for (const schedule of schedules) {
    if (schedule.paused) {
      paused += 1;
    }
    if ((schedule.tags || []).length > 0) {
      tagged += 1;
    }
  }

  return {
    total: schedules.length,
    active: Math.max(schedules.length - paused, 0),
    paused,
    tagged,
    upcoming,
  };
};
