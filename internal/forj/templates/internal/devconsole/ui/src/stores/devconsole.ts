import { reactive } from "vue";

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

type RouteInfo = {
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
  next: string;
  next_run: string;
  tags: string[];
  source?: string;
};

type LogEntry = {
  level: string;
  message: string;
  source: string;
  time: string;
  fields: Record<string, any>;
};

type DevwatchLine = {
  line: string;
  stream: string;
  timestamp: string;
};

type DevconsoleState = {
  agents: AgentInfo[];
  selectedAgent: string;
  routes: RouteInfo[];
  routesByAgent: Record<string, RouteInfo[]>;
  schedules: ScheduleInfo[];
  schedulesByAgent: Record<string, ScheduleInfo[]>;
  logs: LogEntry[];
  devwatch: DevwatchLine[];
  authenticated: boolean;
  bootstrapped: boolean;
  socketConnected: boolean;
  devwatchConnected: boolean;
  logLimit: number;
  devwatchLimit: number;
};

const state = reactive<DevconsoleState>({
  agents: [],
  selectedAgent: "",
  routes: [],
  routesByAgent: {},
  schedules: [],
  schedulesByAgent: {},
  logs: [],
  devwatch: [],
  authenticated: false,
  bootstrapped: false,
  socketConnected: false,
  devwatchConnected: false,
  logLimit: 5000,
  devwatchLimit: 2000,
});

let socket: WebSocket | null = null;
let devwatchSocket: WebSocket | null = null;
const pending: Record<string, (payload: any) => void> = {};
let socketReady: Promise<void> | null = null;
let reconnectTimer: number | null = null;
let reconnectAttempts = 0;
let devwatchReady: Promise<void> | null = null;

const fetchAgents = async () => {
  const res = await fetch("/__devconsole/api/agents");
  if (res.status === 401) {
    state.authenticated = false;
    disconnectSocket();
    return;
  }
  if (!res.ok) return;
  const agents = (await res.json()) as AgentInfo[];
  syncAgents(agents);
  state.authenticated = true;
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
    const routes = data.routes.map((route: RouteInfo) => ({
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
  if (data.logs) {
    state.logs = data.logs;
  }
};

const handleEvent = (payload: any) => {
  if (payload.agents) {
    syncAgents(payload.agents as AgentInfo[]);
    return;
  }
  if (!payload.log) return;
  state.logs = [payload.log as LogEntry, ...state.logs].slice(0, state.logLimit);
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
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  const url = `${scheme}://${window.location.host}/__devconsole/ws/console`;
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
    state.agents = [];
    state.selectedAgent = "";
    scheduleReconnect();
  });
  socket.addEventListener("error", () => {
    socketReady = null;
    socket = null;
    state.socketConnected = false;
    state.agents = [];
    state.selectedAgent = "";
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
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  const url = `${scheme}://${window.location.host}/__devconsole/ws/devwatch`;
  devwatchSocket = new WebSocket(url);
  devwatchReady = new Promise((resolve) => {
    devwatchSocket?.addEventListener("open", () => {
      devwatchReady = null;
      resolve();
    });
  });
  devwatchSocket.addEventListener("open", () => {
    state.devwatchConnected = true;
  });
  devwatchSocket.addEventListener("close", () => {
    devwatchReady = null;
    devwatchSocket = null;
    state.devwatchConnected = false;
  });
  devwatchSocket.addEventListener("error", () => {
    devwatchReady = null;
    devwatchSocket = null;
    state.devwatchConnected = false;
  });
  devwatchSocket.addEventListener("message", (event) => {
    try {
      const msg = JSON.parse(event.data);
      if (msg.type !== "devwatch") return;
      const payload =
        typeof msg.payload === "string" ? JSON.parse(msg.payload || "{}") : msg.payload || {};
      if (payload.lines) {
        state.devwatch = payload.lines;
        return;
      }
      if (payload.line) {
        state.devwatch = [...state.devwatch, payload.line].slice(-state.devwatchLimit);
      }
    } catch {
      return;
    }
  });
  return devwatchSocket;
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

const disconnectSocket = () => {
  if (!socket) return;
  socket.close();
  socket = null;
  socketReady = null;
  state.socketConnected = false;
  state.agents = [];
  state.selectedAgent = "";
  if (reconnectTimer) {
    window.clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  reconnectAttempts = 0;
  disconnectDevwatch();
};

const disconnectDevwatch = () => {
  if (!devwatchSocket) return;
  devwatchSocket.close();
  devwatchSocket = null;
  devwatchReady = null;
  state.devwatchConnected = false;
  state.devwatch = [];
};

const bootstrap = async () => {
  if (state.bootstrapped) {
    return;
  }
  await fetchAgents();
  state.bootstrapped = true;
};

const logout = async () => {
  await fetch("/__devconsole/auth/logout", { method: "POST" });
  state.authenticated = false;
  state.bootstrapped = false;
  disconnectSocket();
  state.agents = [];
  state.selectedAgent = "";
  state.routes = [];
  state.routesByAgent = {};
  state.schedules = [];
  state.schedulesByAgent = {};
  state.logs = [];
  state.devwatch = [];
};

export const useDevconsoleStore = () => ({
  state,
  fetchAgents,
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
  bootstrap,
  logout,
  disconnectSocket,
  disconnectDevwatch,
});
