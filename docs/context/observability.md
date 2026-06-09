# Observability

This document describes the current logging and telemetry model across GoForj and `web`.

## Current Direction

GoForj observability work should currently be thought of in two layers:

- metric production inside generated apps
- standard observability-stack validation outside the app

That means:

- generated apps emit Prometheus-compatible metrics
- the metric contract is first proven with VictoriaMetrics and Grafana
- Lighthouse comes after the metric contract is already proven useful

This ordering is intentional.

Lighthouse should not be the first place where the framework discovers:

- awkward metric names
- missing labels
- noisy histograms
- dashboard-hostile aggregation choices

Those are easier to discover by trying to build standard dashboards against a Prometheus-compatible backend first.

## Logging Philosophy

Prefer:

- high-signal default logs
- low-noise startup/shutdown output
- explicit operational markers

Avoid:

- giant boot dumps
- per-hook trace spam by default
- leaking implementation details into user-facing logs

## Process Logs

These are generally good default-visible logs:

- HTTP server start/stop
- scheduler start/stop
- queue worker start/stop
- watcher start/stop
- dev ready markers
- clear degraded-runtime warnings when optional facilities are unavailable

## Primitive Chatter

Detailed managed primitive chatter should generally be:

- debug-level
- or explicitly enabled

Examples:

- per-hook lifecycle details
- DB/event bus start-stop chatter
- repeated backend driver connection failures when the higher-level degraded state is already known

## Route Visibility

Normal boot should not print the full route table.

Instead:

- boot logs a route count summary
- `route:list` is the explicit full route dump

Current intended message shape:

```text
Routes registered; use command route:list for full list
```

## Degraded Runtime Feedback

When optional subsystems degrade, prefer one clear structured warning over repeated low-level noise.

Examples:

- optional storage disk unavailable; skipping
- reconnecting to Lighthouse live runtime

Recent rules learned from storage work:

- log optional storage disk failures through the normal app logger
- do not print directly from generated managers to raw `stderr`
- avoid emitting the same warning once per bootstrap process if `forj run` starts multiple subprocesses
- if the UI hides an unavailable resource, show an explicit unavailable/degraded state instead of silent emptiness

## `APP_LOG_TIME`

Console timestamps are gated by:

- `APP_LOG_TIME`

If timestamps seem present but only show `.000`, the likely issue is emitted timestamp precision, not the renderer.

## `web` Telemetry Story

Current important pieces in `web`:

- `webmiddleware`
  - request logging
  - body dump
  - other request/response middleware
- `web.Response`
  - response status/size/committed visibility
- `webprometheus`
  - Prometheus metrics middleware and handler

## `webprometheus`

This package exists to give `web` a first-class Prometheus story.

Current capability includes:

- scrape handler
- request counter
- in-flight gauge
- request duration histogram
- request size histogram
- response size histogram
- route-aware labels

This is meant to make `web` feature-close to Echo’s Prometheus support without forcing apps to depend directly on Echo middleware.

## Metrics Posture

When adding new framework metrics, prefer:

- normalized route or resource identities
- bounded labels
- low-cardinality dimensions
- metrics that answer real operational questions

Avoid:

- raw path labels with IDs in them
- labels sourced from user input
- emitting framework-owned transport traffic as if it were app traffic
- adding metrics that look impressive in `/metrics` output but do not help operators

Good default framework-owned surfaces:

- HTTP
- queues
- scheduler

Potentially useful later surfaces:

- database health
- cache/store operations
- mail delivery
- event delivery

The current proving path is:

1. HTTP metrics
2. queue metrics
3. scheduler metrics
4. VictoriaMetrics + Grafana validation
5. Lighthouse adaptation

If in doubt, read [../designs/metrics-design.md](../designs/metrics-design.md) before expanding the metric set.

## Metrics Implementation State

The metrics work is no longer just conceptual. Generated apps now have a real Prometheus-compatible observability pipeline that should be treated as the current source of truth.

At a high level:

- generated apps emit metrics from framework-managed primitives
- VictoriaMetrics + vmagent + Grafana are the first-class proving environment
- Lighthouse should consume a metric contract that is already operationally useful

This means future metrics work should usually start with:

- metric semantics
- label quality
- Grafana usefulness
- render and smoke validation

Only after that should the same data model be surfaced inside Lighthouse.

## Current Metrics Controls

Framework metrics are now controlled in two layers:

- component-level render enablement through the `Metrics` component
- per-surface runtime toggles through env

Current per-surface toggles:

- `METRICS_HTTP_ENABLED`
- `METRICS_CACHE_ENABLED`
- `METRICS_STORAGE_ENABLED`
- `METRICS_EVENTS_ENABLED`
- `METRICS_MAIL_ENABLED`
- `METRICS_QUEUE_ENABLED`
- `METRICS_DATABASE_ENABLED`
- `METRICS_AUTH_ENABLED`
- `METRICS_SCHEDULER_ENABLED`

Important behavior:

- observer-backed primitives should not attach observers when disabled
- direct instrumentation paths should no-op cleanly when disabled
- "disabled" should mean the instrumentation path is absent or inert enough to make overhead comparisons honest

This matters for both production control and the maintainer-only overhead workflow.

## Current Instrumented Surfaces

The current framework metrics work covers these major areas:

- HTTP
- queue
- scheduler
- cache
- storage
- mail
- events
- database

The quality of those surfaces is not identical yet.

Current reality:

- HTTP is in a good place as the reference model
- queue has meaningful per-queue, per-job, and per-worker views and is materially beyond a toy counter set
- scheduler has moved to count-oriented operator views instead of raw ops/sec where run counts are the more intuitive question
- database is now table-oriented and includes pool-pressure views; future work should keep panel language operator-facing
- cache/storage/mail/auth have crossed from "instrumented" into "operator-usable" and now include named-accessor filters where useful
- events now cover both publish and delivery views; keep validating that in-process and cross-process drivers behave consistently from an operator perspective
- Grafana dashboards are seeded as starred dashboards so the first-party views are visible from the left navigation after the stack comes up

Grafana seed is a one-shot local helper, not a durable service. It is safe to interrupt because seeding only stars dashboards and sets Grafana preferences. The generated `grafana-seed` Compose service should keep a short `stop_grace_period` so `forj dev` shutdown does not feel blocked by Docker waiting on a helper container.

Database specifically now includes:

- general operation counters and duration histograms
- pool pressure metrics
- slow-query metrics with query fingerprints
- query-volume metrics grouped by fingerprint
- a bounded fingerprint lookup/info metric for Grafana

The fingerprint model is intentionally split:

- Prometheus-friendly identity uses a short `fingerprint`
- human lookup uses a bounded `query_shape`
- unsafe or suspicious query shapes must collapse to `other`

Do not expand database metric labels with raw SQL.

If the fingerprint normalizer is touched in future work, preserve the fail-closed posture:

- suspicious opaque shapes should not become metric labels
- normalization should prefer bounded tokenization over brittle regex-order tricks
- the fallback path should group uncertain shapes under `other`

The standard for future work is not "it emits metrics."

The standard is:

- can an operator explain what is happening?
- can they find hotspots quickly?
- can they tell which named primitive is misbehaving?
- can they reason about driver differences without guesswork?

## Process Model

Metrics are intentionally process-local.

Current process split:

- API process exposes its own metrics endpoint
- jobs process exposes its own metrics endpoint
- scheduler process exposes its own metrics endpoint

The render currently uses dedicated metrics env vars:

- `METRICS_API_PORT`
- `METRICS_JOBS_PORT`
- `METRICS_SCHEDULER_PORT`

Current defaults have been using:

- `9100`
- `9101`
- `9102`

The point of this design is:

- each process keeps simple local metrics ownership
- vmagent scrapes all three processes
- VictoriaMetrics aggregates across them
- Grafana dashboards can show both rolled-up and per-process views

Do not try to fake a single in-process global metrics runtime across multiple binaries.

## Metrics Discovery Model

The current direction is:

- `forj generate` owns vmagent metrics target generation
- vmagent should consume generated target files through `file_sd_configs`
- topology awareness belongs in generated target entries, not hand-maintained CSV env vars

That means target generation should be thought of as a deployment-shape problem, not as "guess a port range."

Current practical rules:

- same port across isolated services is fine
- same port across multiple local host processes is not
- host and port must both be part of target generation logic
- the generated target file is the scrape contract for local/dev observability

If observability target generation is disabled intentionally, that should be treated as an escape hatch for custom operator-managed config, not the default posture.

## Polling HTTP Warm-Up

Monitoring poll cycles previously used a `HEAD` warm-up probe for HTTP targets.

That caused noisy real-world errors against misbehaving sites and CDNs such as:

- `protocol error: received DATA on a HEAD request`

Current direction:

- do not use `HEAD` for HTTP warm-up
- prefer a small HTTP/1 `GET` if warm-up is still needed

If this noise reappears, inspect the monitoring poll warm-up path before assuming the issue is in seed validation.

## Scrape / Validation Expectations

When validating metrics, remember:

- vmagent is typically scraping on a fixed interval, currently configured in rendered apps under `containers/observability/vmagent/prometheus.yml`
- a common default has been `scrape_interval: 15s`

Practical consequence:

- not every point appears instantly
- low-frequency events can look sparse
- if a panel seems empty, first confirm the process endpoint is emitting the metric and then wait at least one scrape cycle

Do not assume a missing line means the instrumentation is broken before checking scrape cadence and the active time range.

## Render Ownership

Most of the metrics implementation currently lives in generated templates, not just sibling repos.

Important ownership points:

- metric manager wiring lives in generated app templates
- primitive-specific recording hooks are often added in rendered managers, repositories, or middleware
- Grafana dashboards are rendered from templates under `templates/containers/observability/grafana/dashboards`
- observability stack container wiring is part of GoForj rendering

That means changes often span:

- metric emission
- dashboard queries
- render config / component wiring
- smoke validation in a rendered app

Do not stop after changing only the metric producer or only the dashboard.

## Labeling Rules Learned So Far

The practical lessons matter more than abstract purity.

Prefer:

- labels based on normalized framework-owned identities
- named primitive labels where operators actually think in terms of named connectors
- stable labels like process, driver, queue, job, route, connection, store, disk, mailer

Avoid:

- raw user-driven labels
- labels that change shape across drivers
- driver-specific naming that makes cross-driver dashboards inconsistent
- overloading one label to mean multiple concepts

Two important learned rules:

- if different drivers can describe the same logical concept, normalize that concept now
- dashboard legends must read like operator concepts, not implementation accidents

Example:

- queue/job naming should be emitted uniformly across drivers so dashboards do not silently lose meaning

## Dashboard Filters

Grafana dashboards should include focused filters when the primitive has a meaningful named accessor or bounded operational identity.

Current expected filters:

- HTTP: route, method, status
- queue: queue, job name
- scheduler: job name
- database: connection, table
- cache: cache name
- storage: disk
- mail: mailer
- auth: provider
- events: bus, topic, handler

Why:

- operators often troubleshoot one named primitive at a time
- all-series dashboards become noisy as generated apps grow
- filter variables prove whether labels are actually useful and stable

Avoid adding filters for high-cardinality runtime values.

Good filters come from framework-owned, bounded identities. Bad filters come from user input, request IDs, raw paths, payload values, email addresses, or other unbounded values.

## Low-Latency Primitive Semantics

Some primitives are fast enough that naive histogram choices produce misleading dashboards.

This is especially true for:

- in-memory cache
- in-memory queue
- in-process events
- in-memory storage

Recent lesson:

- if the lowest histogram bucket is too high, dashboards can make extremely fast operations look much slower than reality

Future work on ultra-low-latency primitives should always examine:

- bucket floors
- displayed units
- whether the dashboard should show microseconds instead of milliseconds

Do not accept a dashboard that implies memory reads are taking multiple milliseconds without first checking histogram resolution and unit scaling.

## Database Metrics State

Database metrics have moved beyond just query counters.

Current direction includes:

- statement-level metrics
- operation grouping
- target/table-oriented views
- connection-oriented views
- pool pressure metrics sourced from `sql.DBStats`

Recent important implementation note:

- pool wait duration is currently recorded as a raw cumulative nanosecond counter and converted in Grafana queries

Why this matters:

- future dashboard math must keep unit conversion explicit
- do not assume the metric library is automatically scaling counter units for display

Current dashboard language should prefer "table" over "target" where the operator is really reasoning about tables.

## Queue Metrics State

Queue is the current strongest non-HTTP proving surface.

The direction there is:

- throughput by queue
- throughput by job name
- jobs by worker
- latency by queue
- latency by job
- inflight by queue and job
- enqueue pressure
- failure and retry hotspots

Important lesson from queue work:

- all drivers need to emit a uniform logical job name or type name
- otherwise dashboards become driver-specific and lose value

If queue metrics look wrong, check:

- whether the process-specific metrics endpoint is being scraped
- whether the driver emits the expected logical labels
- whether the selected Grafana time range actually covers a scrape window with activity

## Dashboard Standard

The dashboard bar is intentionally high.

A dashboard should not be considered good just because it renders.

It should answer questions like:

- what is slow?
- what is failing?
- which named primitive is hot?
- is pressure local to one process or global?
- is this a queue-specific issue, a driver issue, or an app-wide issue?

Avoid dashboards that are mostly:

- giant single stats
- redundant counters
- ambiguous legends
- raw metric dumps in chart form

Prefer:

- concise stat rows for orientation
- dense but readable breakdowns
- per-name and per-driver views where meaningful
- legends that map directly to operator language
- failure/error panels lower in the layout unless failures are the primary workflow
- top-row panels that answer "is this healthy and where is pressure?" quickly
- dashboard variables that let teams narrow the view without editing queries

Grafana navigation:

- first-party dashboards should be provisioned automatically
- dashboards should be seeded/starred when possible so they are visible in Grafana's left navigation
- do not rely on users manually importing JSON or manually favoriting dashboards before the local stack feels usable

## Validation Workflow For Metrics Work

When changing metrics, the most reliable workflow is:

1. update the template or instrumentation point
2. update the dashboard query or labels if needed
3. render a temp app or the host smoke app
4. confirm the process endpoints expose the expected series
5. confirm VictoriaMetrics is scraping them
6. confirm Grafana answers the operational question
7. run build/test validation for the template repo work

For Go commands during validation, prefer:

- `GOCACHE=/tmp/gocache`
- `GOMODCACHE=/tmp/gomodcache`

Typical validation includes:

- `go test ./...` in the template repo when metric manager code changed
- `forj render` in a temp or host test app
- `go build ./...` in the rendered app

Do not trust only a template compile if the rendered observability stack is part of the change.

## What Is Next

The current recommended order for future metrics work is:

1. finish the remaining primitive refinement where semantics are still soft
2. keep tightening low-latency metric semantics and label consistency
3. validate the entire dashboard set repeatedly through rendered apps
4. only then pull the polished model into Lighthouse

Concrete remaining focus areas:

- events
  - make delivery views as strong as publish views
  - keep driver semantics understandable for in-process vs cross-process delivery behavior
  - refine handler-focused operator panels so they answer "who is slow or hot?" immediately
- HTTP
  - finish contract cleanup around which aggregate and route-level series are truly worth keeping
  - keep the dashboard high-signal instead of drifting toward redundant stats
- database
  - keep improving pool pressure, query-shape, and table-oriented views
  - trim any panels that still feel diagnostic instead of operator-focused
- scheduler
  - continue refining count-over-time views so low-frequency jobs stay intuitive
  - keep skip/overlap/failure panels useful without dominating the happy-path layout
- auth
  - add user-facing auth surface documentation under the auth component
- render validation
  - keep validating dashboard/template changes through temp or host rendered apps
  - catch template escaping regressions early when editing Grafana JSON templates

Keep the priority order straight:

- make the metric contract right
- make the dashboards useful
- make the out-of-box observability story excellent
- then adapt that proven story into Lighthouse

## Lighthouse / Benchmark Output

Benchmark UI output should avoid:

- stale queued states after completion
- ambiguous runtime states

When UI state looks wrong, check whether the status precedence is correct before assuming runtime behavior is wrong.

Additional Lighthouse UX guidance:

- when the live websocket connection is lost after boot, dim the app and show an explicit reconnecting state
- while reconnecting, freeze navigation so users do not walk the UI into inconsistent states
- use toast feedback for transient action failures
- reserve inline alerts for page-state failures that need to stay visible
- when a selected resource disappears during refresh, show an explicit unavailable message instead of a silent empty state

Recent benchmark-specific lesson:

- benchmark letter grades should be driver-class aware
- do not grade external caches like Redis against in-process memory thresholds
- absolute throughput alone is a misleading UX signal when the backend class is fundamentally different
