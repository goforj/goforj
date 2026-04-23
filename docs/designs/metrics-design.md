# Metrics Design

## Goals

GoForj metrics should provide a simple, fast, framework-aligned observability primitive without dragging the framework into a full tracing or telemetry platform.

The metrics primitive should stand on its own and remain useful regardless of which downstream system consumes it.

Primary goals:

- low-overhead hot-path updates
- no locks for normal counter/gauge/histogram updates
- snapshot-based export
- Prometheus-compatible `/metrics`
- Kubernetes-friendly pull metrics by default
- explicit framework wiring instead of singleton-first design
- reusable primitive package with GoForj-owned integration
- compatibility with multiple downstream consumers such as Lighthouse, Grafana-backed stacks, and other Prometheus-compatible tools

Non-goals for v1:

- dynamic label support
- distributed metrics aggregation in core
- an in-framework TSDB
- OpenTelemetry/tracing integration
- every exporter under the sun

## Design Direction

Metrics should follow the same boundary pattern as other GoForj primitives:

- reusable core primitive lives in a sibling library such as `github.com/goforj/metrics`
- GoForj owns:
  - component selection
  - generated wiring
  - HTTP endpoint exposure
  - framework instrumentation for HTTP, jobs, scheduler, and later subsystems

Metrics should not be built as a package-global singleton with framework code reaching into it implicitly.

Instead:

- the core library should expose a `Registry`
- generated apps should wire a `metrics.Registry` or `metrics.Manager`
- an optional default registry may exist as a convenience API, but it should not be the architectural center

## Why Not Singleton-First

A package-global default registry is easy to demo, but it is the wrong primary model for GoForj.

Problems:

- harder to test and isolate
- harder to support multiple registries
- makes framework composition implicit
- does not match the explicit wiring style used across GoForj

Preferred model:

```go
type Registry struct {
    // registered metrics
}
```

Generated apps can still expose a convenience surface, but the core contract should be explicit registry ownership.

## Package Boundaries

### Core library

Suggested shape for `github.com/goforj/metrics`:

```text
registry.go
counter.go
gauge.go
histogram.go
snapshot.go
prometheus.go
```

Core types:

- `Registry`
- `Counter`
- `Gauge`
- `Histogram`
- `Snapshot`
- Prometheus encoder/handler helpers

### GoForj integration

Suggested generated app integration:

```text
internal/metrics/
  manager.go
  http_metrics.go
  jobs_metrics.go
  scheduler_metrics.go
```

GoForj integration owns:

- endpoint exposure
- middleware/hooks
- generated defaults
- component-level docs

This boundary is important:

- the primitive should not be Grafana-specific
- the primitive should not assume a single storage backend
- the primitive should export a standard-enough metrics surface that multiple consumers can ingest it

## Metric Types

v1 should support only:

- counters
- gauges
- histograms

This is enough to cover the majority of useful framework instrumentation.

### Counter

Counters are monotonic increasing values.

Requirements:

- lock-free increment/add
- snapshot uses atomic load only

Shape:

```go
type Counter struct {
    value atomic.Uint64
}
```

### Gauge

Gauges represent a point-in-time value.

Requirements:

- lock-free set/add
- signed values allowed

Shape:

```go
type Gauge struct {
    value atomic.Int64
}
```

### Histogram

Histograms should use fixed buckets only in v1.

Requirements:

- fixed bucket boundaries
- lock-free bucket/count/sum updates
- consistent exported representation

Important constraint:

Do not store histogram sums as accumulated float bit patterns. That is mathematically wrong.

Instead:

- observe integer values only
- store `sum` as `int64`
- store `count` and buckets as atomic unsigned integers

Suggested shape:

```go
type Histogram struct {
    bounds  []int64
    buckets []atomic.Uint64
    count   atomic.Uint64
    sum     atomic.Int64
}
```

This makes the hot path simple and avoids float accumulation issues.

## Units

Metrics should have explicit units.

Examples:

- duration
- bytes
- items
- requests

Duration metrics should be recorded internally as integer base units, not raw `float64`.

Recommended approach:

- observe durations as `time.Duration`
- convert to nanoseconds internally
- export to Prometheus seconds where appropriate

This keeps the internal representation fast and precise while still matching Prometheus conventions.

## Naming

Metrics benefit from visible hierarchy.

So dotted canonical names are acceptable and, in many cases, preferable for human readability.

Examples:

- `http.requests.total`
- `http.request.duration`
- `jobs.processed.total`
- `scheduler.runs.total`

The main rule is:

- one canonical internal name
- exporters are responsible for deterministic normalization when a target format requires it

For Prometheus specifically, the exporter should normalize dotted names into Prometheus-safe identifiers.

Examples:

- `http.requests.total` -> `http_requests_total`
- `jobs.processed.total` -> `jobs_processed_total`

This means the naming model should be explicit:

- hierarchy/readability is represented in the canonical metric name
- wire-format compatibility is handled by the exporter

What should not happen is ad hoc or ambiguous normalization.

Normalization rules should be:

- deterministic
- centralized in the exporter
- documented

Units and semantic suffixes should still be explicit. They should not be guessed purely from punctuation.

## Labels

v1 should not support dynamic labels.

This is deliberate.

## Framework Instrumentation Coverage

The metrics primitive is only useful if GoForj uses it consistently across the framework-owned runtime surface.

The intended framework instrumentation model is:

- HTTP metrics
- queue metrics
- scheduler metrics
- later runtime and primitive metrics where they are stable and operationally useful

### HTTP

HTTP is the first instrumentation surface because every generated app with `WebAPI` exposes it and it is the easiest place to validate:

- naming
- label shape
- bucket choices
- scrape compatibility
- dashboard usefulness

The current intended HTTP set is:

- total requests
- in-flight requests
- 4xx total
- 5xx total
- request latency
- request count by normalized route
- request latency by normalized route

Important constraints:

- use normalized route patterns, not raw request paths
- avoid labels that include user-controlled or unbounded values
- exclude framework-owned transport routes from app request metrics when they are not part of the user app surface

Examples of framework-owned HTTP endpoints that should generally be excluded:

- `/metrics`
- Lighthouse transport/API endpoints used to power the local runtime UI
- websocket upgrade transport endpoints

### Queues

Queue metrics are the next highest-value surface after HTTP because production queue behavior is otherwise easy to lose visibility into.

The first queue set should cover:

- jobs processed total
- jobs failed total
- jobs retried total
- jobs scheduled total
- jobs active / in-flight gauge
- job execution duration histogram

Possible bounded labels:

- queue name
- job kind if the set is framework-registered and bounded
- outcome when it is a fixed finite set

Avoid:

- raw payload content
- unbounded job names
- dynamic tenant/user identifiers

### Scheduler

Scheduler metrics should answer whether scheduled work is running and whether it is healthy.

The first scheduler set should cover:

- scheduler runs total
- scheduler failures total
- scheduler skips total
- scheduled job duration histogram

Optional bounded labels may later include:

- schedule name
- schedule kind such as interval vs cron

Again, the point is bounded operational dimensions, not free-form tagging.

### Later Surfaces

Once the core model is proven, later framework-owned instrumentation can expand into areas such as:

- database pool health
- cache/store operation counters and latency
- outbound mail delivery counters and latency
- event bus delivery counters
- runtime process/lifecycle visibility

These should come later, not before the queue/scheduler baseline is proven.

## Observability Stack Direction

Metrics emission and observability storage/UI are separate concerns.

GoForj should provide a metrics primitive and generated integration that can feed multiple downstream systems:

- Prometheus
- VictoriaMetrics
- Grafana
- Lighthouse
- other Prometheus-compatible consumers

That separation matters because Lighthouse should not be the first place where the framework discovers whether a metric model is awkward, under-labeled, or too noisy.

The metric contract should first be proven against a standard Prometheus-compatible stack.

## Standard Validation Target

The standard proving ground for GoForj metrics should be:

- VictoriaMetrics for storage/query
- Grafana for dashboards and operator UX

Why this comes before Lighthouse:

- it validates the metric model against widely understood tooling
- dashboards expose naming and label problems quickly
- it forces pragmatic questions like "can an operator actually answer anything from this metric set?"
- it keeps Lighthouse from becoming the first coupled consumer of an immature contract

Lighthouse can then consume a contract that is already proven useful.

## Observability Component Model

GoForj should grow an explicit `Observability` top-level component rather than treating storage/UI setup as an ad hoc pile of docker templates.

The component model should look like this:

- `Observability` as the top-level platform capability
- optional child components underneath it, starting with `Grafana`

The intended relationship is:

- `Observability` owns the metrics stack wiring and local runtime integration
- `Grafana` adds the optional dashboard UI layer

This follows the same parent/child pattern already established elsewhere in GoForj:

- the user config records selected components
- dependency enforcement stays in the centralized component catalog / normalization layer
- child components are modeled in metadata, not by scattered string checks

Important boundary:

- user config should express selected features
- user config should not directly contain internal dependency bookkeeping

## Default Observability Stack

The default storage/query backend should be VictoriaMetrics.

Reasons:

- Prometheus-compatible ingest model
- simple single-binary local setup
- good fit for touchless local Docker rendering
- easier local footprint than a more complex multi-service metrics stack

The initial stack should be:

- generated app exposing `/metrics`
- VictoriaMetrics scraping the app
- optional Grafana configured against VictoriaMetrics

This should work out of the box in a rendered local environment without users having to write scrape configuration by hand.

## What the Observability Component Should Render

The `Observability` component should eventually render:

- Docker Compose services for VictoriaMetrics
- scrape configuration that points at generated app metrics endpoints
- persistent local storage defaults suitable for development
- environment wiring and docs

If the child `Grafana` component is selected, it should also render:

- Grafana service configuration
- a preconfigured datasource targeting VictoriaMetrics
- provisioned dashboards
- sensible local auth defaults for a touchless dev experience

This should be treated as "just works" infrastructure, not as a vague starter example users need to finish manually.

## Dashboard Proving Scope

Before Lighthouse grows richer metric views, GoForj should prove the metric contract with a small number of first-party Grafana dashboards.

The first dashboard set should be:

- HTTP overview
- queue overview
- scheduler overview

Each dashboard should answer practical operator questions quickly.

Examples:

- which routes are hottest?
- which routes are slowest?
- are 4xx/5xx rates rising?
- are jobs failing or retrying?
- are queues backing up?
- are scheduled jobs running and how long do they take?

If a dashboard cannot answer those kinds of questions cleanly, the metric contract probably still needs work.

## Lighthouse Relationship

Lighthouse should not try to become a full Grafana replacement.

The cleaner product boundary is:

- Lighthouse provides local runtime control-plane visibility
- VictoriaMetrics + Grafana provide full observability exploration

Lighthouse may later:

- show lightweight summaries derived from the metric contract
- link into richer observability views
- query a Prometheus-compatible backend when available

But it should not be the first or only proving ground for the metrics primitive.

## Delivery Order

The most pragmatic implementation order from here is:

1. finish the core metrics primitive and framework HTTP integration
2. add queue metrics
3. add scheduler metrics
4. stand up the `Observability` component with VictoriaMetrics
5. add optional `Grafana` child component
6. ship first-party dashboards
7. iterate on names, labels, and buckets from real dashboard usage
8. adapt Lighthouse to the proven contract

This keeps the contract-first work ahead of product UI coupling.

## v1 Scope Adjustment

The original v1 scope should now be interpreted as:

- metrics primitive
- generated `/metrics`
- HTTP instrumentation
- queue instrumentation
- scheduler instrumentation
- Prometheus-compatible export
- observability stack validation through VictoriaMetrics and optional Grafana

Still out of scope for this first wave:

- distributed tracing
- OpenTelemetry bridge work
- bespoke TSDB behavior in core
- a fully custom Lighthouse observability UI replacing Grafana

Reasons:

- cardinality explosions are one of the easiest ways to break Prometheus-backed metrics
- labels complicate both the hot path and the registration model
- most useful framework signals can be shipped without them initially

So for v1:

- no dynamic labels
- no ad hoc label maps
- no unbounded label values

If labels are added later, they should come through a strictly predeclared vector-style API, not arbitrary runtime maps.

## Registry

The registry owns metric registration and snapshot collection.

Registration is not a hot path, so a mutex is acceptable there.

Suggested shape:

```go
type Registry struct {
    mu         sync.RWMutex
    counters   map[string]*Counter
    gauges     map[string]*Gauge
    histograms map[string]*Histogram
}
```

The hot path is metric update, not registration.

So the main performance rule is:

- no locks in `Inc`, `Add`, `Set`, or `Observe`

## Descriptor and Registration Model

The primitive should use an explicit descriptor model rather than loosely passing names/options around without structure.

Suggested descriptor shape:

```go
type Descriptor struct {
    Name string
    Help string
    Unit string
    Kind MetricKind
}
```

Where `MetricKind` is one of:

- counter
- gauge
- histogram

This gives registration one clear source of truth for:

- canonical metric identity
- human-facing help text
- unit
- metric type

### Registration semantics

Registration should be explicit and deterministic.

The design should support idempotent registration of the same metric definition.

That means:

- registering the same metric name with the same descriptor returns the existing metric
- registering the same metric name with a conflicting descriptor should fail loudly

Recommended behavior:

- registry-level methods return an error on conflicting registration
- convenience helpers may panic in clearly programmer-error situations if desired, but the core registry should support error-returning registration paths

Examples of conflicts:

- same name, different metric kind
- same name, different help text
- same name, different unit
- same name, different histogram bucket set

This is especially important if metrics lives in a reusable sibling library and multiple subsystems register metrics independently.

### Registration timing

Framework-owned metrics should ideally be registered during app startup or subsystem initialization, not ad hoc on arbitrary hot paths.

That keeps:

- metric identity stable
- exporter output predictable
- startup-time failures visible instead of surfacing during runtime traffic

## Snapshot Model

Exporters should operate on snapshots, not live metric structures.

This keeps exporters:

- simpler
- safer
- isolated from concurrent mutation logic

Suggested shape:

```go
type Snapshot struct {
    Counters   []CounterSnapshot
    Gauges     []GaugeSnapshot
    Histograms []HistogramSnapshot
}
```

Snapshot generation can allocate. That is acceptable because export is not the hot path.

Snapshots should be usable beyond the HTTP `/metrics` path.

That means:

- HTTP pull is the default export path
- but snapshots should remain reusable for other downstream integrations, including future Lighthouse ingestion paths if needed

## Export Surface

Prometheus should be the first and primary export target.

The exporter abstraction should stay minimal.

Rather than starting with a broad `Exporter` interface, prefer an encoder-style contract:

```go
type Encoder interface {
    Encode(io.Writer, *Snapshot) error
}
```

This maps naturally to HTTP pull metrics.

For example:

```go
func Handler(reg *Registry) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        snap := reg.Snapshot()
        _ = EncodePrometheus(w, snap)
    }
}
```

This is enough for v1.

If future push/export jobs need a broader interface, it can be added later.

## Prometheus Export

Prometheus is the right first target because:

- it is the most common pull-based metrics consumer in infrastructure environments
- it fits Kubernetes naturally
- it is easy to expose via `/metrics`

Prometheus compatibility here should be understood as a transport/export format choice, not a commitment to Prometheus as the only storage or visualization backend.

That same exported metrics surface should be consumable by:

- Prometheus
- VictoriaMetrics
- Lighthouse, if it chooses to scrape Prometheus-compatible metrics
- other tools that understand Prometheus text format

Requirements:

- counters encode as Prometheus counters
- gauges encode as Prometheus gauges
- histograms encode as Prometheus cumulative bucket series plus `_sum` and `_count`
- dotted canonical metric names are normalized into Prometheus-safe identifiers

The encoder should be:

- in-memory only
- text-based
- allocation-aware, but not over-engineered

Normalization should be deterministic and exporter-owned.

For v1, that means:

- replace `.` with `_`
- append Prometheus-required suffixes where appropriate
- keep unit naming explicit rather than inferred from arbitrary string fragments

### Normalization rules

These rules should be written down explicitly so the canonical internal metric name and exported Prometheus name do not drift.

Recommended Prometheus normalization rules:

1. canonical internal names may use `.` as a hierarchy separator
2. exporter replaces `.` with `_`
3. exporter replaces unsupported characters with `_`
4. repeated separators collapse to a single `_`
5. names should not depend on exporter heuristics for semantic meaning

Examples:

- `http.requests.total` -> `http_requests_total`
- `jobs.processed.total` -> `jobs_processed_total`

The exporter should not guess units from arbitrary name fragments.

Instead:

- units should come from metric descriptors
- Prometheus-required suffixes should be added in a deterministic, documented way

That means the design should decide one of these two approaches and document it clearly during implementation:

- canonical names already include semantic suffixes like `.total` and `.duration`
- or the exporter derives final Prometheus suffixes from descriptor kind/unit metadata

Either can work, but the implementation must choose one and keep it consistent.

## Instrumentation Placement

Framework instrumentation should not live in the core metrics library.

Keep these concerns separate:

- core library owns metric types and snapshot/export mechanics
- GoForj owns how HTTP, jobs, scheduler, auth, and mail emit metrics

That means no `instrument/` subtree in the core library for v1.

Instead, GoForj integration should provide:

- HTTP middleware
- jobs hooks
- scheduler wrappers
- optional auth/mail instrumentation later

## Metrics Component

GoForj should treat metrics as a first-class component:

- `Metrics` component flag
- generated wiring when enabled
- HTTP endpoint exposure when `Metrics && WebAPI`

This keeps the subsystem optional and consistent with other primitives.

## Component and Subsystem Model

Near term, the concrete implementation surface is still a metrics primitive and a metrics integration layer.

That means:

- package/integration names should stay concrete, e.g. `metrics`
- generated app integration can live under `internal/metrics`

Longer term, metrics should be understood as one part of a broader observability subsystem.

So the product/component model may eventually grow into:

- `Observability` as a broader subsystem area
- `Metrics` as one concrete primitive
- tracing as a future sibling primitive
- optional child integrations such as `Grafana`

In other words:

- code/package boundary stays concrete
- user-facing subsystem can broaden later without renaming the core metrics primitive prematurely

### Endpoint policy

The design should also make the metrics endpoint policy explicit.

Near-term recommended behavior:

- `/metrics` is exposed only when metrics is enabled and a Web API surface exists
- it is intended for scraping by infrastructure tools, not end-user interaction

Implementation should later define:

- whether it is public by default
- whether it should be gated by diagnostics settings
- whether some environments should hide or disable it

## Observability Relationship

Metrics is one part of a broader observability story.

Longer term, GoForj should likely expose:

- `Observability` as a higher-level subsystem/component area
- `Metrics` as one core primitive
- tracing later as a separate but related primitive
- optional visualization and local-stack tooling layered on top

In that model:

- metrics/tracing primitives are producers
- external systems are consumers
- Grafana is only one optional consumer/UI layer

This distinction matters.

Grafana can help prove and demonstrate the model, but the metrics primitive must remain useful without Grafana.

That allows GoForj metrics to feed:

- Lighthouse
- Grafana-compatible stacks
- Prometheus/VictoriaMetrics
- other metrics tools in the future

## Optional Grafana Experience

If GoForj adds a local observability stack later, it should be treated as optional integration, not as the definition of the metrics system.

That means a future component model may look like:

- `Observability`
  top-level component or subsystem area
- `Grafana`
  optional child component for local dashboards, datasource provisioning, and Docker setup

In that shape:

- `Metrics` remains the core producer primitive
- Grafana remains an optional convenience UI
- backend choices such as VictoriaMetrics or Prometheus remain downstream concerns

This preserves portability and keeps the primitive layer clean.

## v1 Framework Instrumentation Order

The rollout should be incremental.

### First

HTTP instrumentation:

- request count
- request duration
- request status classes or status codes

This provides immediate value and a stable baseline.

### Second

Jobs instrumentation:

- jobs started
- jobs completed
- jobs failed
- job duration

### Third

Scheduler instrumentation:

- runs
- duration
- failures
- skipped/overlap events

### Later

Auth and mail:

- login attempts
- auth failures
- password reset requests
- mail sends/failures

Only after the primitive and exporter model is proven by HTTP/jobs/scheduler.

## Production-Ready Framework Instrumentation

If GoForj metrics is intended to be team-ready in production, the framework should eventually instrument most of the operationally relevant surfaces it owns.

That means the metrics story should not stop at an HTTP middleware and a few counters.

The framework should aim for broad first-party instrumentation across:

- HTTP
- database
- jobs
- scheduler
- auth
- mail
- cache
- storage
- events
- CLI/runtime lifecycle

This does not mean every surface must land in the first implementation slice. It does mean the design should anticipate comprehensive coverage instead of treating metrics as a narrow feature.

### Baseline principle

Framework-owned execution paths should emit useful metrics by default when metrics is enabled.

Examples:

- HTTP middleware should not require manual app code to emit request metrics
- job execution should not require manual job wrappers to emit queue metrics
- scheduler executions should not require per-task instrumentation to emit run/failure metrics

Application teams can always add their own business metrics on top, but the framework should cover its own platform behavior automatically.

## Instrumentation Coverage Matrix

### HTTP

Expected baseline metrics:

- request count
- request duration
- in-flight requests
- response status distribution
- panic/error count

Recommended labels:

- route pattern
- method
- status class or status code

Avoid:

- raw request paths
- query strings
- user IDs

### Database

Expected baseline metrics:

- connection open count or pool stats where available
- query count
- query duration
- failed queries
- ping/readiness failures
- migration runs and migration failures

Recommended labels:

- connection name
- driver
- operation category if bounded

Avoid:

- raw SQL text as a label
- full DSNs

### Jobs

Expected baseline metrics:

- jobs enqueued
- jobs started
- jobs completed
- jobs failed
- jobs retried
- job duration
- queue depth or lease/backlog metrics where the backend exposes them safely

Recommended labels:

- queue name
- job type
- terminal status

Avoid:

- job IDs
- payload contents

### Scheduler

Expected baseline metrics:

- scheduler runs
- scheduler run duration
- scheduler failures
- skipped runs
- overlap/lock contention skips

Recommended labels:

- task name
- status

Avoid:

- dynamic payload-derived labels

### Auth

Expected baseline metrics:

- login attempts
- login success
- login failure
- lockouts triggered
- sessions created
- sessions revoked
- password reset requests
- email verification requests

Recommended labels:

- flow name
- outcome

Avoid:

- usernames
- emails
- session IDs

### Mail

Expected baseline metrics:

- mail send attempts
- successful sends
- failed sends
- send duration
- provider/backend failures

Recommended labels:

- mailer name
- driver
- template or message kind
- outcome

Avoid:

- recipient addresses
- subjects

### Cache

Expected baseline metrics:

- cache reads
- cache writes
- cache deletes
- cache hits
- cache misses
- operation duration
- backend failures

Recommended labels:

- cache name
- driver
- operation

Avoid:

- cache keys

### Storage

Expected baseline metrics:

- reads
- writes
- deletes
- bytes transferred
- operation duration
- backend failures

Recommended labels:

- disk/storage name
- driver
- operation

Avoid:

- object keys or file paths as labels

### Events

Expected baseline metrics:

- events published
- events handled
- handler failures
- handler duration

Recommended labels:

- bus name
- event type
- handler name where bounded

Avoid:

- event payload data

### CLI and Lifecycle

Expected baseline metrics:

- command runs
- command failures
- command duration
- app startup duration
- app shutdown duration
- readiness failures
- dependency initialization failures

Recommended labels:

- command name
- subsystem
- outcome

Avoid:

- full argv

## Label Policy

The label policy matters as much as the metric types themselves.

Production-safe defaults should prioritize:

- bounded label sets
- stable values
- framework-owned dimensions only

General rule:

- use labels only where the value space is known and limited

Good examples:

- route pattern
- HTTP method
- queue name
- scheduler task name
- cache/store/driver name
- status

Bad examples:

- raw URLs
- user identifiers
- emails
- SQL strings
- object keys
- request IDs
- session IDs

If labels are added later in the core primitive, GoForj integration should still keep framework-emitted labels conservative.

## Automatic vs App-Defined Metrics

GoForj should distinguish two categories clearly:

### Automatic framework metrics

These come from framework-owned execution paths and should be emitted automatically when metrics is enabled.

Examples:

- HTTP request metrics
- scheduler run metrics
- job execution metrics
- cache/store/mail/auth counters for framework-managed operations

### App-defined metrics

These are custom business metrics emitted by application code.

Examples:

- orders placed
- invoices issued
- tenant quota usage

The framework should make app-defined metrics easy, but the production-ready baseline should not depend on app authors instrumenting core platform behavior themselves.

## Future Labels Path

Even if labels are out of v1, the design should leave a clear growth path.

Recommended future stance:

- no dynamic labels in v1
- if labels are added later, they should be predeclared/vector-based
- arbitrary runtime label maps should be avoided

That keeps the API compatible with future expansion without baking in a cardinality footgun from the start.

### HTTP label model

The first labeled framework metric set should almost certainly be HTTP.

This is the most immediately useful place to add labels because teams routinely want to answer questions like:

- which routes are getting traffic
- which methods are hottest
- which endpoints are failing
- which routes are slow

But the HTTP label model has to stay bounded.

Recommended first HTTP label set:

- `method`
- `route`
- `status`

Recommended semantics:

- `method`
  - uppercase HTTP method
  - examples: `GET`, `POST`, `PUT`
- `route`
  - normalized route pattern from the router, not the raw request URI
  - examples:
    - `/api/v1/hello`
    - `/api/v1/users/:id`
    - `/-/health`
- `status`
  - exact response status code as a string
  - examples: `200`, `404`, `500`

This allows useful Prometheus queries without introducing obvious cardinality traps.

Examples:

```text
http_requests_total{method="GET",route="/api/v1/hello",status="200"} 42
http_request_duration_seconds_bucket{method="GET",route="/api/v1/users/:id",le="0.1"} 128
```

Deliberately avoid:

- raw request URI
  - bad: `/api/v1/users/123`
  - good: `/api/v1/users/:id`
- query strings
- host
- user agent
- request ID
- user ID
- tenant ID
- auth subject

Those are either unbounded, noisy, privacy-sensitive, or all three.

### Aggregate plus labeled HTTP metrics

When labeled HTTP metrics are added, GoForj should still keep the current aggregate HTTP metrics.

That means the framework can emit both:

- aggregate metrics
  - `http.requests`
  - `http.request.duration`
- labeled HTTP metrics
  - `http.requests.by_route`
  - `http.request.duration.by_route`

Why keep both:

- aggregate counters stay cheap and easy to reason about
- dashboards often want a top-line total without label selectors
- labeled series add dimension, but should not replace the simple baseline

### Internal traffic exclusion policy

The framework should also define which HTTP requests are excluded from app-facing HTTP metrics.

Recommended exclusions:

- `/metrics`
  - scrape traffic should not inflate application traffic metrics
- upgraded WebSocket connections
  - long-lived sockets poison in-flight gauges and latency histograms if treated like normal HTTP requests
- internal framework-only transport paths when they are not meaningful app traffic
  - examples may include certain Lighthouse transport endpoints

This should apply consistently to:

- aggregate HTTP metrics
- labeled HTTP metrics

If teams want visibility into scrape traffic or other internal transport activity, that should come through separate metrics rather than being mixed into app request metrics.

### Suggested implementation shape for labels

If the core primitive grows label support later, the API should stay explicit and predeclared.

Recommended direction:

- registration defines the allowed label keys up front
- updates use ordered label values, not dynamic maps
- framework integrations own the normalization of label values before emission

Conceptually:

```go
type CounterVec struct { ... }

httpRequests := registry.MustCounterVec(
    Descriptor{Name: "http.requests.by_route", Help: "...", Kind: CounterKind},
    []string{"method", "route", "status"},
)

httpRequests.WithLabelValues("GET", "/api/v1/users/:id", "200").Inc()
```

The important part is not the exact method names.

The important part is:

- the label keys are fixed
- the value order is fixed
- framework code is responsible for using normalized route patterns
- ad hoc runtime label maps are still avoided

## Histogram API Shape

The histogram API still needs to stay explicit about how observations are recorded.

Recommended direction:

- integer observation primitives internally
- typed helpers for common domains

For example:

- `Observe(int64)` for base-unit values
- `ObserveDuration(time.Duration)` for duration histograms

This keeps the hot path simple while giving application and framework code ergonomic helpers.

Bucket configuration should also be explicit:

- fixed buckets only
- validated at registration time
- consistent defaults per metric category where GoForj provides framework instrumentation

Examples:

- HTTP latency buckets
- job duration buckets
- payload size buckets

should not necessarily all share the same defaults.

## Semantic Consistency

To make the metrics surface usable by teams, the framework should keep conventions consistent across all instrumented surfaces.

That means:

- one naming scheme
- one unit policy
- one label policy
- one snapshot/export model

Examples:

- durations should use the same internal unit strategy across HTTP, jobs, scheduler, and mail
- status-style labels should be named consistently
- names for counts, failures, and durations should follow a repeatable convention

Without this, coverage alone becomes noisy instead of useful.

## Phased Implementation Plan

The design should not try to deliver every instrumented surface at once.

Recommended sequence:

### Phase 1

- core primitive library
- registry/descriptor model
- counter/gauge/histogram
- snapshot model
- Prometheus encoder
- GoForj `Metrics` component
- `/metrics` endpoint for Web API apps
- HTTP instrumentation

### Phase 2

- scheduler instrumentation
- jobs instrumentation
- lifecycle/startup/readiness metrics

### Phase 3

- database instrumentation
- auth instrumentation
- mail instrumentation

### Phase 4

- cache instrumentation
- storage instrumentation
- events instrumentation
- broader docs and dashboards

This keeps the first slice useful without pretending the entire production matrix lands in one pass.

## Testing Strategy

The design should explicitly support testing at multiple layers.

### Core library tests

- metric type unit tests
- registration collision tests
- histogram bucket behavior tests
- snapshot correctness tests
- Prometheus encoder golden tests

### GoForj integration tests

- rendered app `/metrics` exposure tests
- middleware/hook instrumentation tests
- generated app smoke tests

### CI posture

- no dependence on Grafana in core CI
- no dependence on a metrics backend to validate primitive correctness
- dashboards and optional observability stack should be tested separately from primitive correctness

## Future Tracing Compatibility

Tracing is not part of the initial metrics implementation, but the design should remain compatible with it.

That means:

- metrics should not assume tracing exists
- metrics naming and instrumentation boundaries should not block later trace integration
- traces should be able to coexist as a sibling primitive, not as an awkward extension stuffed into metrics

Later additions may include:

- tracing integration
- exemplar support
- correlation between metrics and traces

But those should remain follow-on work, not hidden assumptions baked into the first metrics implementation.

## Performance Contract

v1 should make realistic guarantees, not absolute ones.

Good contract:

- no locks for normal metric updates
- no allocations for unlabeled metric updates after registration
- snapshot/export may allocate

Avoid overly absolute claims like “zero allocations everywhere” because they become misleading once middleware and exporter formatting are involved.

## Kubernetes Behavior

The default behavior should be:

- each pod exposes `/metrics`
- Prometheus scrapes each pod
- aggregation happens outside the app

This keeps the metrics subsystem:

- stateless
- horizontally scalable
- compatible with standard infrastructure practice

## Explicitly Out of Scope

These should not be part of v1:

- dynamic labels
- distributed aggregation in core
- TSDB storage
- OpenTelemetry
- tracing
- exemplars
- push gateways as a first-class requirement
- exporter plugin systems beyond what the Prometheus path actually needs

## Recommended v1 Shape

Core:

- `Registry`
- `Counter`
- `Gauge`
- `Histogram` with integer observations
- `Snapshot`
- Prometheus encoder/handler helpers

GoForj integration:

- `Metrics` component
- generated registry/manager
- `/metrics` endpoint for Web API apps
- HTTP instrumentation first

## Implementation Checklist

Use this as the working checklist for the current observability rollout.

### Core Metrics Primitive

- [x] Create sibling `github.com/goforj/metrics` library
- [x] Implement `Registry`
- [x] Implement `Counter`
- [x] Implement `Gauge`
- [x] Implement `Histogram`
- [x] Implement snapshot export model
- [x] Implement Prometheus text encoder
- [x] Add bounded vector support for predeclared labels
- [ ] Review and potentially reduce default histogram bucket count for HTTP latency
- [ ] Write/refresh primitive README with final API examples and cardinality guidance

### GoForj HTTP Integration

- [x] Add `Metrics` component to the centralized component catalog
- [x] Enforce `Metrics -> WebAPI` dependency in normalization/catalog logic
- [x] Generate `internal/metrics` manager wiring
- [x] Expose `/metrics` in rendered Web API apps
- [x] Instrument framework-owned HTTP request totals, inflight, error counts, and latency
- [x] Instrument normalized per-route request totals and latency
- [x] Exclude `/metrics` from app HTTP metrics
- [x] Exclude Lighthouse transport/API endpoints from app HTTP metrics
- [x] Add rendered integration coverage for `/metrics`
- [ ] Decide whether to keep both aggregate HTTP latency histogram and route-labeled HTTP latency histogram
- [ ] Document the final emitted HTTP metric contract in generated docs

### Queue Metrics

- [x] Instrument jobs processed total
- [x] Instrument jobs failed total
- [x] Instrument jobs retried total
- [x] Instrument jobs scheduled total
- [x] Instrument active/inflight job gauge
- [x] Instrument job execution duration histogram
- [x] Decide bounded label model for queue metrics
- [x] Add generated tests for queue metric emission
- [x] Validate queue metrics through a rendered app
- [x] Build production-grade queue dashboards around throughput, latency, inflight, and job/worker breakdowns
- [x] Normalize emitted logical job names across drivers so dashboard legends remain stable

### Scheduler Metrics

- [x] Instrument scheduler runs total
- [x] Instrument scheduler failures total
- [x] Instrument scheduler skips total
- [x] Instrument scheduled job duration histogram
- [x] Decide bounded label model for scheduler metrics
- [x] Add generated tests for scheduler metric emission
- [x] Validate scheduler metrics through a rendered app
- [x] Build operator-focused scheduler dashboards around run counts, outcomes, latency, inflight work, and skip reasons
- [ ] Continue refining scheduler charts so count-oriented views remain intuitive at low run volumes

### Primitive Metrics Expansion

- [x] Instrument database query counts, durations, failures, and bounded labels
- [x] Instrument database pool pressure metrics from `sql.DBStats`
- [x] Instrument cache operations, outcomes, and latency
- [x] Instrument storage operations, bytes moved, and latency
- [x] Instrument event publish/deliver/fail metrics and handler latency
- [x] Instrument mail send outcomes and latency
- [x] Build production-grade database dashboards around table, connection, latency, failure, and pool pressure views
- [x] Build production-grade cache dashboards around hit rate, misses, named cache pressure, and low-latency views
- [x] Build production-grade storage dashboards around operation mix, bytes moved, and named disk views
- [x] Build production-grade mail dashboards around mailer throughput, outcomes, latency, and failure classes
- [x] Build production-grade auth dashboards around login, refresh, revoke, recovery, verification, and latency flows
- [x] Build production-grade platform overview dashboard for cross-primitive operator triage
- [ ] Continue tightening metric semantics for ultra-low-latency primitives
- [ ] Continue normalizing labels across drivers so dashboards stay consistent
- [ ] Finish event delivery/operator semantics so publish vs delivery views remain equally useful across drivers
- [ ] Finish remaining HTTP/dashboard contract cleanup and generated docs wording

### Observability Component

- [x] Add `Observability` top-level component to the centralized component catalog
- [x] Add `Grafana` as an optional child component under `Observability`
- [x] Enforce component dependencies through catalog metadata, not user config bookkeeping
- [x] Render VictoriaMetrics service wiring
- [x] Render scrape configuration for generated apps
- [x] Render per-process metrics endpoints and vmagent scrape wiring
- [ ] Render persistent local storage defaults suitable for development
- [x] Render observability-focused docs and next steps
- [x] Render a first-class Grafana dashboard set for core framework primitives
- [ ] Validate the full dashboard set through repeated rendered-app smoke passes, not just template-level compile checks
- [ ] Decide whether to expose favorites/bookmarks/navigation polish through provisioning or leave it user-managed
- [ ] Decide whether the base `Observability` component should imply `Metrics`

### Grafana Integration

- [x] Render optional Grafana service when child component is selected
- [x] Provision VictoriaMetrics datasource automatically
- [x] Ship a first-party HTTP overview dashboard
- [x] Ship a first-party queue overview dashboard
- [x] Ship a first-party scheduler overview dashboard
- [x] Ship first-party cache, storage, events, mail, and database dashboards
- [ ] Verify dashboards answer practical operator questions cleanly

### Validation And UX

- [x] Stand up a rendered local stack with app + VictoriaMetrics
- [x] Stand up a rendered local stack with app + VictoriaMetrics + Grafana
- [x] Validate metric names, labels, and buckets through real dashboards
- [ ] Trim noisy or low-value metrics before expanding surface area further
- [x] Add implementation notes to `docs/context/observability.md` as lessons are learned

### Lighthouse Follow-On

- [ ] Decide whether Lighthouse should scrape app metrics directly or query a Prometheus-compatible backend
- [ ] Add lightweight metrics summaries to Lighthouse only after the contract is proven in Grafana
- [ ] Keep Lighthouse positioned as a local control plane, not a full Grafana replacement

## Out-Of-Box Acceptance Criteria

The observability stack should be judged against a strict "works immediately after render" bar.

### `Observability` Component

If a user selects `Observability`, the rendered app should provide:

- a working VictoriaMetrics service without manual compose edits
- scrape configuration already pointed at the generated app
- sane local persistence defaults for development
- generated docs that explain the local URLs and expected behavior
- a stack that starts without the user writing additional config files by hand

What should not be required:

- manually editing scrape targets
- manually creating datasources
- manually importing dashboard JSON
- reading an external guide just to finish local setup

### `Grafana` Child Component

If a user selects `Grafana` under `Observability`, the rendered app should additionally provide:

- a working Grafana service in the rendered stack
- a preprovisioned VictoriaMetrics datasource
- preprovisioned first-party dashboards
- dashboard folders/titles that are understandable without internal project knowledge
- a first-visit experience where dashboards already load real data from the generated app

The expected user experience is:

1. render the project
2. start the local stack
3. open Grafana
4. immediately see live dashboards backed by the app's metrics

That is the product bar.

### Dashboard Behavior

Provisioned dashboards should:

- be authored against the GoForj metric contract, not copied blindly from generic community templates
- answer practical operator questions quickly
- degrade gracefully when some metric surfaces are absent
- avoid panels that appear broken merely because an optional component was not selected

Examples:

- if queue metrics are unavailable because queue components were not selected, queue dashboards should show an explicit empty or unavailable state rather than broken panels
- if scheduler metrics are unavailable, scheduler dashboards should behave similarly

### Review Standard

An observability implementation should not be considered complete if it merely:

- starts containers
- exposes raw metrics
- includes dashboard JSON files somewhere in the repo

It should be considered complete only when:

- the rendered stack boots cleanly
- Grafana is already wired
- dashboards already load
- the dashboards are useful without hand-tuning
- the generated docs match the actual rendered experience

## Final Principle

Metrics core should be:

- explicit
- fast
- simple
- Prometheus-friendly

GoForj integration should be:

- incremental
- framework-aware
- optional by component

Storage, dashboards, and long-term aggregation should remain outside the core metrics primitive.
