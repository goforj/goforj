# Uptime Gopher Scaling Tasks

## Sidebar And Monitor Data Flow

- [x] Replace the current full-list sidebar fetch with a lightweight sidebar list contract.
  - Add `GET /api/v1/monitoring/monitors/sidebar`.
  - Return only sidebar fields: `id`, `name`, `type`, `enabled`, `last_status`, maintenance flags, favicon capability, and any compact display target fields.
  - Support pagination or cursor-based loading from the start.

- [x] Replace the global heartbeat sidebar payload with an ID-scoped batch contract.
  - Add `POST /api/v1/monitoring/heartbeats/sidebar`.
  - Accept `{ ids: string[], limit: number }`.
  - Return heartbeat strips only for the requested monitor IDs.

- [x] Remove the current “all monitors” heartbeat sidebar path from the default UI flow.
  - Do not keep building heartbeat strips for every monitor on every sidebar refresh.
  - Keep any legacy global endpoint only as a temporary compatibility path during rollout, then delete it.

## Frontend Sidebar Rendering

- [x] Virtualize the monitor sidebar list.
  - Render only visible rows plus a small overscan window.
  - Keep row height stable so virtualization remains predictable.
  - Derive visible monitor IDs from the virtualizer.

- [x] Fetch heartbeat strips only for visible sidebar rows.
  - Batch visible IDs into a single request.
  - Do not issue per-row requests.
  - Debounce visibility changes slightly during scroll.

- [x] Add a client-side heartbeat strip cache.
  - Key by `monitor_id + limit`.
  - Reuse cached strips when rows scroll back into view.
  - Revalidate in the background instead of blocking paint.

- [x] Pause or reduce visible-strip refresh while the sidebar is actively scrolling.
  - Refresh once after scroll settles.
  - Avoid churn during large scroll traversals.

## Detail View Separation

- [x] Decouple monitor detail pages from global sidebar heartbeat reloads.
  - Do not let detail refreshes trigger sidebar-wide heartbeat fetches.
  - Use selected-monitor-specific endpoints for detail charts, checks, and summary data.

- [x] Remove any live-event handlers that trigger full sidebar heartbeat reloads.
  - Patch only the affected row and selected detail state in memory.

## Backend Query Scaling

- [ ] Add repository queries scoped to requested monitor IDs.
  - Replace “load recent rows for everything, then regroup in memory” with ID-scoped queries.
  - Add a repo method like `HeartbeatRowsForMonitorIDs(ids, perMonitorLimit)`.

- [ ] Make per-monitor heartbeat retrieval efficient across supported dialects.
  - Use a query shape that caps rows per monitor.
  - Validate behavior on SQLite, MySQL, and Postgres.
  - Avoid scanning thousands of unrelated rows for a small visible subset.

- [x] Keep monitor identity ordering queries lightweight.
  - Avoid fetching unnecessary dashboard/detail fields for sidebar list rendering.

## Refresh And Transport Strategy

- [x] Split refresh behavior into separate lanes.
  - Sidebar list metadata: slow refresh or explicit invalidation.
  - Visible heartbeat strips: short refresh only for visible rows.
  - Monitor detail: selected-monitor-specific refresh cadence.

- [x] Move status-change propagation to a push-first model.
  - Use websocket or SSE events to patch known monitor rows.
  - Treat push as the primary freshness path for status and last-check state.
  - Keep polling only as a bounded fallback for visible heartbeat strips and reconnection recovery.

- [x] Add hidden-tab backpressure.
  - Pause or heavily reduce sidebar polling when the tab is hidden.
  - Resume with a single catch-up fetch on visibility restore.

## Rollout Plan

- [x] Phase 1: add ID-scoped heartbeat endpoint and sidebar list endpoint.
- [x] Phase 2: refactor sidebar to fetch visible monitor IDs only, without virtualization yet.
- [x] Phase 3: add sidebar virtualization.
- [x] Phase 4: remove global heartbeat reloads from detail/live event paths.
- [x] Phase 5: tune polling, caching, and hidden-tab behavior.
- [ ] Phase 6: delete obsolete global sidebar heartbeat logic and endpoints.

## Observability

- [ ] Instrument sidebar scaling metrics.
  - Visible monitor count.
  - Heartbeat batch size.
  - Sidebar heartbeat endpoint latency.
  - Rows scanned vs rows returned.
  - Sidebar refresh frequency.
  - Live event rate and reconnect behavior.

- [ ] Add inspect/debug visibility for sidebar data-flow behavior where useful.
  - Make it easy to tell when the UI is issuing a visible-ID-scoped fetch versus a broader fallback path.

## Validation

- [ ] Add frontend tests around virtualized visible-ID fetching behavior.
- [ ] Add backend tests for ID-scoped heartbeat payload generation.
- [ ] Add rendered-app validation for large monitor sets.
  - Seed thousands of monitors.
  - Validate sidebar responsiveness and bounded request volume.
  - Validate that only visible rows trigger heartbeat strip fetches.
