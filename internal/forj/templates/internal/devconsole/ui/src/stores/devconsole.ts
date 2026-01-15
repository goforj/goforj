import { reactive } from "vue";

type AgentInfo = {
  id: string;
  source: string;
  env: string;
  capabilities: string[];
};

type RouteInfo = {
  path: string;
  handler: string;
  methods: string[];
  middlewares: string[];
};

type ScheduleInfo = {
  id: string;
  name: string;
  next_run: string;
  tags: string[];
};

type LogEntry = {
  level: string;
  message: string;
  source: string;
  time: string;
  fields: Record<string, any>;
};

type DevconsoleState = {
  agents: AgentInfo[];
  routes: RouteInfo[];
  schedules: ScheduleInfo[];
  logs: LogEntry[];
  authenticated: boolean;
  bootstrapped: boolean;
};

const state = reactive<DevconsoleState>({
  agents: [],
  routes: [],
  schedules: [],
  logs: [],
  authenticated: false,
  bootstrapped: false,
});

let socket: WebSocket | null = null;

const fetchAgents = async () => {
  const res = await fetch("/__devconsole/api/agents");
  if (res.status === 401) {
    state.authenticated = false;
    disconnectSocket();
    return;
  }
  if (!res.ok) return;
  state.agents = await res.json();
  state.authenticated = true;
};

const requestRoutes = () => {
  if (!socket || socket.readyState !== WebSocket.OPEN) return;
  socket.send(
    JSON.stringify({
      type: "command",
      id: String(Date.now()),
      target: "api",
      payload: { name: "routes:list", params: {} },
    })
  );
};

const requestSchedules = () => {
  if (!socket || socket.readyState !== WebSocket.OPEN) return;
  socket.send(
    JSON.stringify({
      type: "command",
      id: String(Date.now()) + "-schedule",
      target: "scheduler",
      payload: { name: "schedule:list", params: {} },
    })
  );
};

const handleResponse = (payload: any) => {
  if (!payload.ok || !payload.data) return;
  const data = typeof payload.data === "string" ? JSON.parse(payload.data) : payload.data;
  if (data.routes) {
    state.routes = data.routes.map((route: RouteInfo) => ({
      ...route,
      methods: route.methods || [],
      middlewares: route.middlewares || [],
    }));
  }
  if (data.schedules) {
    state.schedules = data.schedules.map((schedule: ScheduleInfo) => ({
      ...schedule,
      tags: schedule.tags || [],
    }));
  }
};

const handleEvent = (payload: any) => {
  if (!payload.log) return;
  state.logs = [payload.log as LogEntry, ...state.logs].slice(0, 200);
};

const connectSocket = () => {
  if (!state.authenticated) {
    return null;
  }
  if (socket && socket.readyState === WebSocket.OPEN) {
    return socket;
  }
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  const url = `${scheme}://${window.location.host}/__devconsole/ws/console`;
  socket = new WebSocket(url);
  socket.addEventListener("open", () => {
    fetchAgents();
    requestRoutes();
    requestSchedules();
  });
  socket.addEventListener("message", (event) => {
    try {
      const msg = JSON.parse(event.data);
      if (msg.type === "response") {
        const payload =
          typeof msg.payload === "string" ? JSON.parse(msg.payload || "{}") : msg.payload || {};
        handleResponse(payload);
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

const disconnectSocket = () => {
  if (!socket) return;
  socket.close();
  socket = null;
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
  state.routes = [];
  state.schedules = [];
  state.logs = [];
};

export const useDevconsoleStore = () => ({
  state,
  fetchAgents,
  connectSocket,
  requestRoutes,
  requestSchedules,
  bootstrap,
  logout,
  disconnectSocket,
});
