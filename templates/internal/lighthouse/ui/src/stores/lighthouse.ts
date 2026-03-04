import { reactive } from "vue";
import { lighthousePath, lighthouseWSURL } from "../lib/base-path";

type AgentInfo = {
  id: string;
  source: string;
  env: string;
  capabilities: string[];
  last_seen?: string;
  started_at?: string;
  connected_at?: string;
  host?: string;
  instance_id?: string;
  instance_kind?: string;
  app?: string;
  version?: string;
};

type RouteEntry = {
  path: string;
  handler: string;
  methods: string[];
  middlewares: string[];
  source?: string;
};

type ScheduleInfo = {
  id: string;
  name: string;
  type: string;
  schedule: string;
  handler: string;
  handler_raw?: string;
  next: string;
  next_run: string;
  tags: string[];
  paused?: boolean;
  source?: string;
};

type LogEntry = {
  level: string;
  message: string;
  source: string;
  time: string;
  fields: Record<string, any>;
  _fieldsText?: string;
  _levelLower?: string;
  _messageLower?: string;
  _sourceLower?: string;
  _timeLabel?: string;
};

export type DevwatchLine = {
  line: string;
  stream: string;
  timestamp: string;
  id?: number;
  watcher?: string;
};

export type EditorTarget = "vscode" | "goland";
type EditorRequest = {
  path?: string;
  line?: number;
  target: EditorTarget;
  symbol?: string;
};

export type DevwatchSnapshot = {
  lines: DevwatchLine[];
  watchers?: Record<string, DevwatchLine[]>;
  watcherOrder?: string[];
};

type LighthouseState = {
  agents: AgentInfo[];
  selectedAgent: string;
  routes: RouteEntry[];
  routesByAgent: Record<string, RouteEntry[]>;
  schedules: ScheduleInfo[];
  schedulesByAgent: Record<string, ScheduleInfo[]>;
  schedulesPausedAllByAgent: Record<string, boolean>;
  logs: LogEntry[];
  devwatch: DevwatchLine[];
  authenticated: boolean;
  bootstrapped: boolean;
  localClient: boolean;
  socketConnected: boolean;
  devwatchConnected: boolean;
  logLimit: number;
  devwatchLimit: number;
  devwatchWatchers: string[];
};

const state = reactive<LighthouseState>({
  agents: [],
  selectedAgent: "",
  routes: [],
  routesByAgent: {},
  schedules: [],
  schedulesByAgent: {},
  schedulesPausedAllByAgent: {},
  logs: [],
  devwatch: [],
  authenticated: false,
  bootstrapped: false,
  localClient: false,
  socketConnected: false,
  devwatchConnected: false,
  logLimit: 5000,
  devwatchLimit: 400,
  devwatchWatchers: [],
});

let socket: WebSocket | null = null;
let devwatchSocket: WebSocket | null = null;
const pending: Record<string, (payload: any) => void> = {};
let socketReady: Promise<void> | null = null;
let reconnectTimer: number | null = null;
let reconnectAttempts = 0;
let devwatchReady: Promise<void> | null = null;
let devwatchReconnectTimer: number | null = null;
let devwatchReconnectAttempts = 0;
let agentsPollTimer: number | null = null;
const devwatchQueue: DevwatchLine[] = [];
let devwatchFlushHandle: number | null = null;
export type DevwatchUpdate =
  | { kind: "snapshot"; snapshot: DevwatchSnapshot }
  | { kind: "append"; line: DevwatchLine };
const devwatchSubscribers = new Set<(update: DevwatchUpdate) => void>();

const emitDevwatchUpdate = (update: DevwatchUpdate) => {
  devwatchSubscribers.forEach((subscriber) => subscriber(update));
};

const subscribeDevwatch = (subscriber: (update: DevwatchUpdate) => void) => {
  devwatchSubscribers.add(subscriber);
  return () => devwatchSubscribers.delete(subscriber);
};

const devwatchWatcherSet = new Set<string>();

const formatLogTime = (value: string) => {
  if (!value) return "--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
};

const formatLogFieldValue = (value: any) => {
  if (value === null || value === undefined) return "";
  if (typeof value === "object") {
    try {
      return JSON.stringify(value);
    } catch {
      return String(value);
    }
  }
  return String(value);
};

const formatLogFields = (fields: Record<string, any>) => {
  if (!fields) return "";
  return Object.entries(fields)
    .map(([key, value]) => `${key}=${formatLogFieldValue(value)}`)
    .join(" · ");
};

const normalizeLog = (log: LogEntry): LogEntry => {
  const level = log.level || "";
  const message = log.message || "";
  const source = log.source || "";
  const fieldsText = formatLogFields(log.fields || {});
  return {
    ...log,
    _fieldsText: fieldsText,
    _levelLower: level.toLowerCase(),
    _messageLower: message.toLowerCase(),
    _sourceLower: source.toLowerCase(),
    _timeLabel: formatLogTime(log.time),
  };
};

const flushDevwatchQueue = () => {
  devwatchFlushHandle = null;
  if (devwatchQueue.length === 0) {
    return;
  }
  const linesToAppend = devwatchQueue.splice(0, devwatchQueue.length);
  state.devwatch = [...state.devwatch, ...linesToAppend].slice(-state.devwatchLimit);
  linesToAppend.forEach((line) => emitDevwatchUpdate({ kind: "append", line }));
};

const scheduleDevwatchFlush = () => {
  if (devwatchFlushHandle !== null) {
    return;
  }
  devwatchFlushHandle = window.requestAnimationFrame(flushDevwatchQueue);
};

const fetchAgents = async () => {
  const res = await fetch(lighthousePath("/api/agents"));
  if (res.status === 401) {
    state.authenticated = false;
    stopAgentsPoll();
    disconnectSocket();
    return;
  }
  if (!res.ok) return;
  const agents = (await res.json()) as AgentInfo[];
  syncAgents(agents);
  state.authenticated = true;
};

const fetchLocal = async () => {
  const res = await fetch(lighthousePath("/api/local"));
  if (res.status === 401) {
    state.authenticated = false;
    stopAgentsPoll();
    disconnectSocket();
    return;
  }
  if (!res.ok) return;
  const payload = (await res.json()) as { local?: boolean };
  state.localClient = Boolean(payload.local);
};

const stopAgentsPoll = () => {
  if (agentsPollTimer === null) return;
  window.clearInterval(agentsPollTimer);
  agentsPollTimer = null;
};

const requestRoutes = (target = "api") => {
  if (!socket || socket.readyState !== WebSocket.OPEN) return;
  socket.send(
    JSON.stringify({
      type: "command",
      id: String(Date.now()),
      target,
      payload: { name: "routes:list", params: {} },
    })
  );
};

const requestSchedules = (target = "scheduler") => {
  if (!socket || socket.readyState !== WebSocket.OPEN) return;
  socket.send(
    JSON.stringify({
      type: "command",
      id: String(Date.now()) + "-schedule",
      target,
      payload: { name: "schedule:list", params: {} },
    })
  );
};

const handleResponse = (payload: any, source?: string) => {
  if (!payload.ok || !payload.data) return;
  const data = typeof payload.data === "string" ? JSON.parse(payload.data) : payload.data;
  if (data.routes) {
    const routes = data.routes.map((route: RouteEntry) => ({
      ...route,
      source,
      methods: route.methods || [],
      middlewares: route.middlewares || [],
    }));
    if (source) {
      state.routesByAgent[source] = routes;
      state.routes = Object.values(state.routesByAgent).flat();
    } else {
      state.routes = routes;
    }
  }
  if (data.schedules) {
    const schedules = data.schedules.map((schedule: ScheduleInfo) => ({
      ...schedule,
      source,
      tags: schedule.tags || [],
    }));
    if (source) {
      state.schedulesByAgent[source] = schedules;
      state.schedules = Object.values(state.schedulesByAgent).flat();
    } else {
      state.schedules = schedules;
    }
  }
  if (Object.prototype.hasOwnProperty.call(data, "paused_all")) {
    const key = source || "scheduler";
    state.schedulesPausedAllByAgent[key] = Boolean(data.paused_all);
  }
  if (data.logs) {
    state.logs = data.logs.map((log: LogEntry) => normalizeLog(log));
  }
};

const handleEvent = (payload: any) => {
  if (payload.agents) {
    syncAgents(payload.agents as AgentInfo[]);
    return;
  }
  if (!payload.log) return;
  const normalized = normalizeLog(payload.log as LogEntry);
  state.logs = [normalized, ...state.logs].slice(0, state.logLimit);
};

const connectSocket = () => {
  if (!state.authenticated) {
    return null;
  }
  if (socket && socket.readyState === WebSocket.OPEN) {
    return socket;
  }
  if (socket && socket.readyState === WebSocket.CONNECTING) {
    return socket;
  }
  const url = lighthouseWSURL("/ws/console");
  socket = new WebSocket(url);
  socketReady = new Promise((resolve) => {
    socket?.addEventListener("open", () => {
      socketReady = null;
      resolve();
    });
  });
  socket.addEventListener("open", () => {
    reconnectAttempts = 0;
    state.socketConnected = true;
    fetchAgents();
    requestRoutesAll();
    requestSchedulesAll();
    requestLogHistory();
  });
  socket.addEventListener("close", () => {
    socketReady = null;
    socket = null;
    state.socketConnected = false;
    scheduleReconnect();
  });
  socket.addEventListener("error", () => {
    socketReady = null;
    socket = null;
    state.socketConnected = false;
    scheduleReconnect();
  });
  socket.addEventListener("message", (event) => {
    try {
      const msg = JSON.parse(event.data);
      if (msg.type === "response") {
        const payload =
          typeof msg.payload === "string" ? JSON.parse(msg.payload || "{}") : msg.payload || {};
        if (msg.reply_to && pending[msg.reply_to]) {
          pending[msg.reply_to](payload);
          delete pending[msg.reply_to];
        }
        handleResponse(payload, msg.source);
        return;
      }
      if (msg.type === "event") {
        const payload =
          typeof msg.payload === "string" ? JSON.parse(msg.payload || "{}") : msg.payload || {};
        handleEvent(payload);
      }
    } catch {
      return;
    }
  });
  return socket;
};

const connectDevwatch = () => {
  if (!state.authenticated) {
    return null;
  }
  if (devwatchSocket && devwatchSocket.readyState === WebSocket.OPEN) {
    return devwatchSocket;
  }
  if (devwatchSocket && devwatchSocket.readyState === WebSocket.CONNECTING) {
    return devwatchSocket;
  }
  const url = lighthouseWSURL("/ws/devwatch");
  devwatchSocket = new WebSocket(url);
  devwatchReady = new Promise((resolve) => {
    devwatchSocket?.addEventListener("open", () => {
      devwatchReady = null;
      resolve();
    });
  });
  devwatchSocket.addEventListener("open", () => {
    state.devwatchConnected = true;
    devwatchReconnectAttempts = 0;
    if (!state.socketConnected) {
      connectSocket();
    }
    fetchAgents();
    requestRoutesAll();
    requestSchedulesAll();
    requestLogHistory();
  });
  devwatchSocket.addEventListener("close", () => {
    devwatchReady = null;
    devwatchSocket = null;
    state.devwatchConnected = false;
    scheduleDevwatchReconnect();
  });
  devwatchSocket.addEventListener("error", () => {
    devwatchReady = null;
    devwatchSocket = null;
    state.devwatchConnected = false;
    scheduleDevwatchReconnect();
  });
  devwatchSocket.addEventListener("message", (event) => {
    try {
      const msg = JSON.parse(event.data);
      if (msg.type !== "devwatch") return;
      const payload =
        typeof msg.payload === "string" ? JSON.parse(msg.payload || "{}") : msg.payload || {};
      if (payload.lines) {
        const watchers =
          payload.watchers && typeof payload.watchers === "object"
            ? (payload.watchers as Record<string, DevwatchLine[]>)
            : undefined;
        const watcherOrder = Array.isArray(payload.watcher_order)
          ? (payload.watcher_order as string[])
          : undefined;
        state.devwatch = payload.lines;
        const watchersList =
          (watcherOrder && watcherOrder.length > 0
            ? watcherOrder
            : watchers
            ? Object.keys(watchers)
            : []
          ).filter(Boolean);
        state.devwatchWatchers = watchersList;
        devwatchWatcherSet.clear();
        watchersList.forEach((name) => devwatchWatcherSet.add(name));
        emitDevwatchUpdate({
          kind: "snapshot",
          snapshot: {
            lines: payload.lines,
            watchers,
            watcherOrder,
          },
        });
        devwatchQueue.length = 0;
        if (devwatchFlushHandle !== null) {
          window.cancelAnimationFrame(devwatchFlushHandle);
          devwatchFlushHandle = null;
        }
        return;
      }
      if (payload.line) {
        devwatchQueue.push(payload.line);
        const watcherName = payload.line.watcher ?? "";
        if (watcherName && !devwatchWatcherSet.has(watcherName)) {
          devwatchWatcherSet.add(watcherName);
          state.devwatchWatchers = [...state.devwatchWatchers, watcherName];
        }
        if (devwatchQueue.length >= 64) {
          flushDevwatchQueue();
          return;
        }
        scheduleDevwatchFlush();
      }
    } catch {
      return;
    }
  });
  return devwatchSocket;
};

const scheduleDevwatchReconnect = () => {
  if (!state.authenticated) {
    return;
  }
  if (devwatchReconnectTimer) {
    return;
  }
  const delay = Math.min(5000, 1000 * (devwatchReconnectAttempts + 1));
  devwatchReconnectTimer = window.setTimeout(() => {
    devwatchReconnectTimer = null;
    devwatchReconnectAttempts += 1;
    connectDevwatch();
  }, delay);
};

const waitForDevwatch = async () => {
  connectDevwatch();
  if (devwatchSocket && devwatchSocket.readyState === WebSocket.OPEN) {
    return;
  }
  if (devwatchReady) {
    await Promise.race([
      devwatchReady,
      new Promise((_, reject) => setTimeout(() => reject(new Error("devwatch not connected")), 2000)),
    ]);
  }
};

const sendDevwatchControl = (action: string) => {
  if (!devwatchSocket || devwatchSocket.readyState !== WebSocket.OPEN) {
    return waitForDevwatch().then(() => sendDevwatchControl(action));
  }
  devwatchSocket.send(
    JSON.stringify({
      type: "devwatch-control",
      payload: { action },
    })
  );
};

const openEditor = async (payload: EditorRequest) => {
  if (!payload.path && !payload.symbol) {
    throw new Error("missing file path or symbol");
  }
  const res = await fetch(lighthousePath("/api/editor"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      path: payload.path,
      line: payload.line ?? 1,
      target: payload.target,
      symbol: payload.symbol,
    }),
  });
  if (!res.ok) {
    let reason = "failed to open editor";
    try {
      const text = await res.text();
      if (text) {
        reason = text;
      }
    } catch {
      //
    }
    throw new Error(reason);
  }
};

const scheduleReconnect = () => {
  if (!state.authenticated) {
    return;
  }
  if (reconnectTimer) {
    return;
  }
  const delay = Math.min(5000, 1000 * (reconnectAttempts + 1));
  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = null;
    reconnectAttempts += 1;
    connectSocket();
  }, delay);
};

const syncAgents = (agents: AgentInfo[]) => {
  state.agents = agents.sort((a, b) => a.source.localeCompare(b.source));
  if (state.agents.length > 0) {
    const hasSelected = state.agents.some((agent) => agent.source === state.selectedAgent);
    if (!hasSelected) {
      state.selectedAgent = state.agents[0].source;
    }
  } else {
    state.selectedAgent = "";
  }
};

const waitForSocket = async () => {
  connectSocket();
  if (socket && socket.readyState === WebSocket.OPEN) {
    return;
  }
  if (socketReady) {
    await Promise.race([
      socketReady,
      new Promise((_, reject) => setTimeout(() => reject(new Error("socket not connected")), 2000)),
    ]);
  }
};

const requestRoutesAll = () => {
  state.agents
    .filter((agent) => agent.capabilities.includes("routes"))
    .forEach((agent) => requestRoutes(agent.source));
};

const requestSchedulesAll = () => {
  state.agents
    .filter((agent) => agent.capabilities.includes("schedule"))
    .forEach((agent) => requestSchedules(agent.source));
};

const requestLogHistory = () => {
  if (!socket || socket.readyState !== WebSocket.OPEN) return;
  const id = `logs-${Date.now()}`;
  socket.send(
    JSON.stringify({
      type: "command",
      id,
      target: "control",
      payload: { name: "logs:history", params: { limit: state.logLimit } },
    })
  );
};

const setLogLimit = (limit: number) => {
  if (!Number.isFinite(limit) || limit <= 0) return;
  state.logLimit = Math.min(limit, 10000);
  requestLogHistory();
};

const selectAgent = (source: string) => {
  state.selectedAgent = source;
};

const sendCommand = (target: string, name: string, params: Record<string, any>) => {
  if (!socket || socket.readyState !== WebSocket.OPEN) {
    return waitForSocket().then(() => sendCommand(target, name, params));
  }
  const id = `${name}-${Date.now()}`;
  socket.send(
    JSON.stringify({
      type: "command",
      id,
      target,
      payload: { name, params },
    })
  );
  return new Promise((resolve) => {
    pending[id] = resolve;
  });
};

const runScheduleAction = (target: string, action: string, id?: string) => {
  const params: Record<string, any> = {};
  if (id) {
    params.id = id;
  }
  return sendCommand(target, `schedule:${action}`, params);
};

const disconnectSocket = () => {
  if (!socket) return;
  socket.close();
  socket = null;
  socketReady = null;
  state.socketConnected = false;
  if (reconnectTimer) {
    window.clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  reconnectAttempts = 0;
  disconnectDevwatch();
};

const disconnectDevwatch = () => {
  state.devwatch = [];
  state.devwatchWatchers = [];
  devwatchWatcherSet.clear();
  if (!devwatchSocket) return;
  devwatchSocket.close();
  devwatchSocket = null;
  devwatchReady = null;
  state.devwatchConnected = false;
  devwatchQueue.length = 0;
  if (devwatchFlushHandle !== null) {
    window.cancelAnimationFrame(devwatchFlushHandle);
    devwatchFlushHandle = null;
  }
  if (devwatchReconnectTimer) {
    window.clearTimeout(devwatchReconnectTimer);
    devwatchReconnectTimer = null;
  }
  devwatchReconnectAttempts = 0;
};

const bootstrap = async () => {
  if (state.bootstrapped) {
    return;
  }
  await fetchAgents();
  if (state.authenticated) {
    await fetchLocal();
  }
  state.bootstrapped = true;
};

const logout = async () => {
  await fetch(lighthousePath("/auth/logout"), { method: "POST" });
  state.authenticated = false;
  state.bootstrapped = false;
  disconnectSocket();
  state.agents = [];
  state.selectedAgent = "";
  state.routes = [];
  state.routesByAgent = {};
  state.schedules = [];
  state.schedulesByAgent = {};
  state.schedulesPausedAllByAgent = {};
  state.logs = [];
  state.devwatch = [];
  state.localClient = false;
};

export const useLighthouseStore = () => ({
  state,
  fetchAgents,
  fetchLocal,
  connectSocket,
  connectDevwatch,
  requestRoutes,
  requestSchedules,
  requestRoutesAll,
  requestSchedulesAll,
  requestLogHistory,
  setLogLimit,
  selectAgent,
  sendCommand,
  runScheduleAction,
  sendDevwatchControl,
  openEditor,
  bootstrap,
  logout,
  disconnectSocket,
  disconnectDevwatch,
  subscribeDevwatch,
});
