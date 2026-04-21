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
