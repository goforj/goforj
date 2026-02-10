# Redis Ingest Plan

## Goal
Move monitor check result persistence to a durable Redis-backed ingest path that:
- Preserves reliability under retries/restarts.
- Supports high-throughput batch inserts.
- Pushes realtime UI updates with low latency.

## Principles
- Queue for truth, pub/sub for UX.
- Idempotent writes everywhere.
- Backpressure is observable and controlled.
- Keep current worker probe flow; change persistence path first.

## Target Architecture

### Producer (Probe Worker)
- Probe job executes check as it does now.
- Instead of writing directly to DB, enqueue `monitor.check.result` payload to Asynq.
- Payload includes:
  - `event_id` (ULID/UUID)
  - `monitor_id`
  - `checked_at`
  - `status`, `status_code`, `duration_ms`
  - `error_class`, `error_message`
  - `attempt`, `source`, optional `meta`

### Durable Ingest Consumer
- New dedicated Asynq handler consumes `monitor.check.result`.
- Handler appends to in-memory batch buffer.
- Batch flush rules:
  - `max_batch_size` (ex: 250)
  - `flush_interval` (ex: 200ms)
  - immediate flush on shutdown signal
- Flush writes with `CreateInBatches`.
- On DB failure:
  - retry handler task via Asynq retry policy
  - never publish realtime event for failed writes

### State/Incident Processor
- During flush transaction:
  - persist monitor checks
  - update monitor streak/circuit fields
  - open/resolve incidents deterministically
- Keep this logic centralized in ingest service (single source of truth).

### Realtime Fanout
- After successful DB commit, publish lightweight update events:
  - Redis pub/sub channel (ex: `monitoring.events`)
  - event types: `check.recorded`, `monitor.state.changed`, `incident.opened`, `incident.resolved`
- API WS hub subscribes and pushes updates to connected clients.
- UI consumes deltas without polling for hot-path updates.

## Data Integrity

### Idempotency
- Add DB uniqueness guard:
  - preferred: unique `event_id`
  - fallback: composite (`monitor_id`, `checked_at`, `status`, `duration_ms`)
- Ingest treats duplicates as success (no-op).

### Ordering
- Ordering guaranteed per monitor by `checked_at`, not receive time.
- UI/pill bucketing uses `checked_at` only.

### Delivery Semantics
- At-least-once from queue.
- Exactly-once effect at DB via idempotency.

## Config
- `MONITOR_INGEST_ENABLED=true`
- `MONITOR_INGEST_BATCH_SIZE=250`
- `MONITOR_INGEST_FLUSH_MS=200`
- `MONITOR_INGEST_QUEUE=monitoring_ingest`
- `MONITOR_EVENTS_CHANNEL=monitoring.events`
- `MONITOR_INGEST_MAX_INFLIGHT` (protect memory)

## Observability
- Metrics:
  - `ingest_queue_depth`
  - `ingest_batch_size`
  - `ingest_flush_ms`
  - `ingest_write_errors_total`
  - `ingest_duplicate_events_total`
  - `ingest_dropped_events_total` (target 0)
- Logs include:
  - `event_id`, `monitor_id`, queue retry count, flush size, flush duration
- Diagnostics page can add ingest health strip:
  - queue lag, batch throughput, retry pressure

## Rollout Plan

### Phase 1: Internal Refactor (No Behavior Change)
- Extract current `writeCheckWithPolicy` into `CheckIngestService`.
- Keep direct call path for now.

### Phase 2: Queue-backed Ingest
- Worker enqueues `monitor.check.result`.
- Add ingest consumer + batch flush.
- Gate behind feature flag.

### Phase 3: Realtime Events
- Publish post-commit events to Redis pub/sub.
- API WS layer subscribes and forwards.
- UI subscribes and applies targeted updates.

### Phase 4: Polling Reduction
- Keep periodic polling as fallback.
- Reduce polling frequency where WS coverage is sufficient.

## Failure Modes and Handling
- Redis unavailable:
  - probe job returns retryable error; no data loss by design.
- DB unavailable:
  - ingest task retries via Asynq; no fanout until success.
- API node restart:
  - WS reconnects; polling fallback prevents stale UI.
- Duplicate delivery:
  - swallowed by unique index/idempotent insert.

## Test Plan
- Unit:
  - idempotent ingest
  - batch flush thresholds
  - incident transitions
- Integration:
  - enqueue -> ingest -> DB rows -> WS event
  - DB failure + retry + eventual success
  - duplicate payload replay
- Load:
  - seed 10k+ monitors
  - verify sustained ingest throughput and queue lag bounds

## Open Decisions
- Keep Asynq for ingest vs Redis Streams consumer groups.
  - Recommendation: start with Asynq (already present), revisit Streams only if needed.
- Event payload granularity.
  - Recommendation: send minimal deltas + monitor id; let UI fetch expanded state only when needed.

## Implementation Checklist (File Targets)

### 1) Add ingest task contract
- [ ] Define task type and payload structs.
  - Template: `templates/demo/internal/monitoring/ingest_task.go.tmpl`
  - Rendered: `internal/monitoring/ingest_task.go`
- [ ] Include validation and lightweight normalization helpers.

### 2) Implement `CheckIngestService`
- [ ] Create service with:
  - append-to-buffer
  - flush by size/interval
  - transactional batch write + state/incident updates
- [ ] Reuse existing policy logic from current check flow.
  - Template: `templates/demo/internal/monitoring/check_ingest_service.go.tmpl`
  - Rendered: `internal/monitoring/check_ingest_service.go`

### 3) Add repository methods for batch ingest
- [ ] Batch create checks (`CreateInBatches`).
- [ ] Idempotent insert path (duplicate-safe).
  - Template: `templates/demo/internal/models/monitor_check.go.tmpl`
  - Rendered: `internal/models/monitor_check.go`
- [ ] Add helper queries needed by ingest policy transitions.
  - Template: `templates/demo/internal/models/incident.go.tmpl`
  - Rendered: `internal/models/incident.go`
- [ ] Add monitor state update helpers for streak/circuit updates.
  - Template: `templates/demo/internal/models/monitor.go.tmpl`
  - Rendered: `internal/models/monitor.go`

### 4) Database constraints for idempotency
- [ ] Add `event_id` column (preferred) or composite uniqueness key.
- [ ] Add indexes for ingest and diagnostics queries.
  - Templates:
    - `templates/demo/internal/migrations/*_demo_monitor_checks_table.mysql.up.sql.tmpl`
    - `templates/demo/internal/migrations/*_demo_monitor_checks_table.sqlite.up.sql.tmpl`
    - matching `.down.sql.tmpl` files
  - Rendered: `internal/migrations/*demo_monitor_checks_table*.sql`

### 5) Convert probe job to producer-only
- [ ] Update monitor check job to enqueue ingest payload instead of direct DB write.
- [ ] Keep retry/circuit classification logic in probe stage.
  - Template: `templates/demo/internal/monitoring/monitor_check_job.go.tmpl`
  - Rendered: `internal/monitoring/monitor_check_job.go`

### 6) Add ingest worker/handler registration
- [ ] Add Asynq handler for `monitor.check.result`.
- [ ] Ensure queue name/config is centralized.
  - Template: `templates/internal/jobs/worker.go.tmpl`
  - Rendered: `internal/jobs/worker.go`

### 7) Wire providers
- [ ] Register `CheckIngestService` + ingest handler dependencies in wire sets.
  - Templates:
    - `templates/wire/inject_jobs_app.go.tmpl`
    - `templates/wire/inject_jobs_cmd.go.tmpl` (if needed by current graph)
  - Rendered:
    - `wire/inject_jobs_app.go`
    - `wire/inject_jobs_cmd.go`

### 8) Realtime event fanout
- [ ] Add publisher in ingest service after successful commit.
- [ ] Add subscriber in API WS hub and fanout to clients.
  - Templates:
    - `templates/demo/internal/monitoring/events.go.tmpl`
    - `templates/internal/devconsole/*` (only if reusing devconsole bus)
    - `templates/demo/internal/monitoring/controller.go.tmpl` (route wiring, if required)
  - Rendered:
    - `internal/monitoring/events.go`

### 9) Config/env support
- [ ] Add ingest tuning env vars with defaults:
  - `MONITOR_INGEST_ENABLED`
  - `MONITOR_INGEST_BATCH_SIZE`
  - `MONITOR_INGEST_FLUSH_MS`
  - `MONITOR_INGEST_QUEUE`
  - `MONITOR_EVENTS_CHANNEL`
- [ ] Add comments/docs in env templates.
  - Templates:
    - `templates/.env.tmpl`
    - `templates/.env.host.tmpl`
  - Rendered:
    - `.env`
    - `.env.host`

### 10) Frontend realtime hookup (monitoring UI)
- [ ] Subscribe to monitor events and apply targeted UI patches:
  - monitor status pills
  - detail metrics
  - incident list deltas
- [ ] Keep existing polling as fallback (reduced frequency).
  - Templates:
    - `templates/demo/frontend/src/views/MonitoringView.vue`
    - `templates/demo/frontend/src/lib/monitoring-requests.ts`
    - `templates/demo/frontend/src/lib/monitoring-state.ts` (if split)

### 11) Diagnostics additions
- [ ] Add ingest health endpoint and UI card:
  - queue lag
  - batch size
  - write error count
  - duplicate count
- [ ] Extend diagnostics page table/cards.
  - Templates:
    - `templates/demo/internal/monitoring/controller.go.tmpl`
    - `templates/demo/frontend/src/views/DiagnosticsView.vue`

### 12) Tests
- [ ] Unit tests: ingest batching, idempotency, incident transitions.
  - Templates:
    - `templates/demo/internal/monitoring/check_ingest_service_test.go.tmpl`
    - `templates/demo/internal/models/monitor_check_repo_test.go.tmpl`
- [ ] Integration tests: producer->queue->ingest->db->fanout.
  - Existing suite target:
    - `internal/forj/*integration_test.go`
  - Add demo-specific assertions in:
    - `internal/forj/demo_app_integration_test.go`

### 13) Rollout safety flags
- [ ] Keep direct-write path behind temporary fallback flag.
- [ ] Default to new ingest path for demo once validated.
- [ ] Remove fallback after 1-2 release cycles.
