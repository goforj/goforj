# Lighthouse Base URL Configuration

This guide explains how to change the Lighthouse base URL from the default:

- Backend route base: `/lighthouse`
- Frontend base path: `/lighthouse/`
- Agent websocket URL path: `/lighthouse/ws/agent`

## 1) Backend Route Base

In your rendered app, update:

- `internal/lighthouse/server.go`

Change the route base constant from:

```go
const lighthouseRouteBase = "/lighthouse"
```

to your desired path (example):

```go
const lighthouseRouteBase = "/ops"
```

This controls API/auth/ws/static endpoints under that prefix.

## 2) Frontend Base Path

In your rendered app, update:

- `internal/lighthouse/ui/vite.config.ts`

Change:

```ts
base: "/lighthouse/",
```

to match the backend prefix:

```ts
base: "/ops/",
```

The Lighthouse frontend uses `import.meta.env.BASE_URL` via `src/lib/base-path.ts`, so fetch and websocket paths follow the Vite `base` value.

## 3) Agent URL (Backend Services -> Lighthouse)

In `.env` (or process env), set:

```bash
LIGHTHOUSE_URL=ws://localhost:3000/ops/ws/agent
```

Also ensure:

```bash
LIGHTHOUSE_ENABLED=true
LIGHTHOUSE_TOKEN=your-token
```

## 4) Rebuild Frontend Assets

After changing frontend base path, rebuild UI assets:

```bash
cd internal/lighthouse/ui
npm install
npm run build
```

Then run your normal app build/render flow.

## 5) Quick Validation

After restart, confirm:

- UI loads at `http://localhost:3000/ops`
- Agents endpoint works at `http://localhost:3000/ops/api/agents`
- Devwatch websocket connects to `/ops/ws/devwatch`
- Agent logs show successful dial to `/ops/ws/agent`

