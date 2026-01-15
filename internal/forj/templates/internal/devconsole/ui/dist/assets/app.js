(function () {
  const agentsEl = document.getElementById("agents");
  const routesEl = document.getElementById("routes");
  const schedulesEl = document.getElementById("schedules");
  const refreshBtn = document.getElementById("refresh");
  const refreshSchedulesBtn = document.getElementById("refresh-schedules");
  const agentsCount = document.getElementById("agents-count");
  const routesCount = document.getElementById("routes-count");
  const schedulesCount = document.getElementById("schedules-count");
  const token = new URLSearchParams(window.location.search).get("token");

  function headers() {
    if (!token) return {};
    return { Authorization: "Bearer " + token };
  }

  function renderAgents(agents) {
    if (agentsCount) {
      agentsCount.textContent = String(agents ? agents.length : 0);
    }
    if (!agents || agents.length === 0) {
      agentsEl.innerHTML = '<div class="list-empty">No agents connected.</div>';
      return;
    }
    agentsEl.innerHTML = agents
      .map(function (agent) {
        return (
          '<div class="list-item">' +
          '<div class="list-label">' +
          agent.source +
          "</div>" +
          '<div class="list-meta">' +
          (agent.env || "unknown") +
          " · " +
          (agent.capabilities || []).join(", ") +
          "</div>" +
          "</div>"
        );
      })
      .join("");
  }

  function renderRoutes(routes) {
    if (routesCount) {
      routesCount.textContent = String(routes ? routes.length : 0);
    }
    if (!routes || routes.length === 0) {
      routesEl.innerHTML =
        '<tr><td colspan="4" class="muted">No route data yet.</td></tr>';
      return;
    }
    routesEl.innerHTML = routes
      .map(function (route) {
        return (
          "<tr>" +
          "<td>" +
          route.path +
          "</td>" +
          "<td>" +
          (route.methods || []).join(", ") +
          "</td>" +
          "<td>" +
          route.handler +
          "</td>" +
          "<td>" +
          (route.middlewares || []).join(", ") +
          "</td>" +
          "</tr>"
        );
      })
      .join("");
  }

  function renderSchedules(schedules) {
    if (schedulesCount) {
      schedulesCount.textContent = String(schedules ? schedules.length : 0);
    }
    if (!schedules || schedules.length === 0) {
      schedulesEl.innerHTML =
        '<tr><td colspan="3" class="muted">No schedule data yet.</td></tr>';
      return;
    }
    schedulesEl.innerHTML = schedules
      .map(function (schedule) {
        return (
          "<tr>" +
          "<td>" +
          schedule.name +
          "</td>" +
          "<td>" +
          schedule.next_run +
          "</td>" +
          "<td>" +
          (schedule.tags || []).join(", ") +
          "</td>" +
          "</tr>"
        );
      })
      .join("");
  }

  function fetchAgents() {
    fetch("/__devconsole/api/agents", { headers: headers() })
      .then(function (res) {
        return res.json();
      })
      .then(renderAgents)
      .catch(function () {});
  }

  function createSocket() {
    const scheme = location.protocol === "https:" ? "wss" : "ws";
    const wsUrl =
      scheme +
      "://" +
      location.host +
      "/__devconsole/ws/console" +
      (token ? "?token=" + encodeURIComponent(token) : "");
    const ws = new WebSocket(wsUrl);
    ws.onopen = function () {
      requestRoutes(ws);
      requestSchedules(ws);
    };
    ws.onmessage = function (event) {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type !== "response") return;
        const payload =
          typeof msg.payload === "string"
            ? JSON.parse(msg.payload || "{}")
            : msg.payload || {};
        if (!payload.ok || !payload.data) return;
        const data =
          typeof payload.data === "string"
            ? JSON.parse(payload.data)
            : payload.data;
        if (data.routes) {
          renderRoutes(
            data.routes.map(function (route) {
              route.methods = route.methods || [];
              route.middlewares = route.middlewares || [];
              return route;
            })
          );
        }
        if (data.schedules) {
          renderSchedules(
            data.schedules.map(function (schedule) {
              schedule.tags = schedule.tags || [];
              return schedule;
            })
          );
        }
      } catch (err) {
        return;
      }
    };
    return ws;
  }

  function requestRoutes(ws) {
    const msg = {
      type: "command",
      id: String(Date.now()),
      target: "api",
      payload: { name: "routes:list", params: {} },
    };
    ws.send(JSON.stringify(msg));
  }

  function requestSchedules(ws) {
    const msg = {
      type: "command",
      id: String(Date.now()) + "-schedule",
      target: "scheduler",
      payload: { name: "schedule:list", params: {} },
    };
    ws.send(JSON.stringify(msg));
  }

  fetchAgents();
  const ws = createSocket();

  refreshBtn.addEventListener("click", function () {
    fetchAgents();
    if (ws && ws.readyState === WebSocket.OPEN) {
      requestRoutes(ws);
    }
  });

  refreshSchedulesBtn.addEventListener("click", function () {
    if (ws && ws.readyState === WebSocket.OPEN) {
      requestSchedules(ws);
    }
  });
})();
