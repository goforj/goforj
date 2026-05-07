# Lighthouse Execution Inspection Design

## Purpose

This document describes an execution inspection system for GoForj, centered on
Lighthouse.

The goal is to let an operator inspect one concrete execution and answer:

- what entered the app
- what happened during it
- what framework primitives it touched
- what failed
- what was slow

This is not just a tracing feature. It is an execution inspection feature.

It also should not be a Lighthouse-only architecture.

The first consumer may be Lighthouse, but the underlying capture and correlation
model should be reusable for:

- trace tooling
- deeper execution inspection
- future exports or adapters

## Why This Matters

GoForj now has a stronger context and source propagation model across:

- `http`
- `jobs`
- `scheduler`
- `startup`
- `cli`

Sibling libraries also now preserve context more consistently across:

- cache
- storage
- events
- queue

That work was necessary because Lighthouse needs trustworthy execution context
before it can offer useful request or job inspection.

Without that foundation:

- trace lineage would be incomplete
- queue or event activity could be misattributed
- request and job views would be misleading
- primitive activity could not be correlated reliably

This design is the next step after the context propagation work.

## Primary Goal

Provide a first-party operator experience where a developer can inspect a single
execution and see correlated framework activity in one place.

Examples:

- inspect one HTTP request
- inspect one queue job execution
- inspect one scheduler-triggered run
- inspect one CLI command run

Underneath that product goal, the architectural goal is:

- build a reusable execution telemetry substrate
- use Lighthouse as the first consumer of that substrate

## Non-Goals

This design is not trying to build all of these at once:

- a full distributed tracing backend
- a full OpenTelemetry replacement
- long-term retention analytics
- a generic log aggregation platform
- a metrics dashboard replacement

It also should not require every downstream app to adopt custom business-level
instrumentation before the feature becomes useful.

## Product Shape

The core experience should be:

1. open Lighthouse
2. browse recent executions
3. filter by source, status, time, route, job, or command
4. open one execution
5. inspect a unified timeline and correlated primitive activity

The important shift is:

- not “show me every log line in the system”
- but “show me what happened during this execution”

## Terminology

The framework should use one attribution concept:

- `source`

Current expected values:

- `http`
- `jobs`
- `scheduler`
- `cli`
- `startup`
- `app` as a fallback only

Execution inspection should be built around `source`, not around the older
runtime-style terminology.

## Design Goals

- make one execution easy to inspect end to end
- preserve low-friction usefulness with only framework instrumentation
- correlate logs, traces, and primitive activity by execution
- work across `http`, `jobs`, `scheduler`, and `cli`
- remain useful in local development before large-scale tracing rollout
- keep the core framework model simple and explicit

## Architectural Layers

This design should be split into two layers.

### Layer 1: Execution Telemetry Substrate

This is the reusable backend model.

It should own:

- execution identity
- correlation
- execution record lifecycle
- event capture
- retention
- query shape

It should not assume any specific UI.

### Layer 2: Consumers

Consumers should sit on top of the substrate.

Examples:

- Lighthouse execution inspection
- trace and span viewers
- future deep-diagnostics tools
- exports or adapters

This split matters because the reusable execution model should not be coupled to
one screen or one storage presentation.

## High-Level Model

An execution inspection record should represent one owned execution boundary.

Examples:

- one HTTP request
- one queue job attempt
- one scheduler job run
- one CLI command run

Each execution record should have:

- an execution id
- source
- start time
- end time
- status
- summary fields specific to the source type
- links to correlated spans, logs, and framework events

### Core Record

Suggested conceptual shape:

```go
type ExecutionRecord struct {
    ID        string
    Source    string
    Name      string
    StartedAt time.Time
    EndedAt   time.Time
    Status    string
    Summary   map[string]any
}
```

This is not meant as a final code-level API. It is the architectural record the
rest of the experience should orbit around.

## Context’s Role

Context is the propagation backbone, but it should remain lightweight.

Context should carry:

- cancellation and deadlines
- `source`
- trace and span propagation
- execution identity
- a lightweight execution recorder handle

Context should not carry:

- the full execution record
- a growing slice of primitive events
- full query or payload capture
- arbitrary mutable inspection state

The right mental model is:

- context carries the execution envelope and recorder handle
- the recorder owns the mutable execution data
- persistence happens outside context

## Recorder Model

The clean architecture is to attach an execution recorder to context at ingress.

Conceptually:

```go
ctx = inspection.WithRecorder(ctx, recorder)
```

Then downstream framework and library code can emit structured events into that
recorder during execution.

Conceptual shape:

```go
type ExecutionRecorder interface {
    AddEvent(Event)
    SetSummaryField(key string, value any)
    Finish(status string, err error)
}
```

That recorder can:

- buffer in memory
- stream incrementally
- sample
- redact
- truncate
- finalize persistence at the end of execution

This is much cleaner than trying to shove a growing execution payload into
context values directly.

## Suggested Code Shape

The code-level shape should reflect the separation between propagation and
capture.

### Context carrier helpers

Suggested conceptual helpers:

```go
package inspection

func WithExecutionID(ctx context.Context, id string) context.Context
func ExecutionIDFromContext(ctx context.Context) string

func WithRecorder(ctx context.Context, r ExecutionRecorder) context.Context
func RecorderFromContext(ctx context.Context) ExecutionRecorder
```

GoForj-specific source helpers can continue to live alongside app context
helpers:

```go
ctx = app.WithSource(ctx, app.SourceHTTP)
```

### Recorder creation at ingress

Ingress boundaries should create and attach the recorder:

- HTTP request boundary
- queue job handler boundary
- scheduler task boundary
- CLI command boundary
- startup/bootstrap boundary when desired

Conceptually:

```go
func beginExecution(ctx context.Context, source app.Source, name string) (context.Context, ExecutionRecorder) {
    recorder := telemetry.NewExecutionRecorder(...)
    ctx = app.WithSource(ctx, source)
    ctx = inspection.WithExecutionID(ctx, recorder.ID())
    ctx = inspection.WithRecorder(ctx, recorder)
    return ctx, recorder
}
```

### Event emission

Framework surfaces and primitive observers should retrieve the recorder from
context when present and emit normalized events.

Conceptually:

```go
if recorder := inspection.RecorderFromContext(ctx); recorder != nil {
    recorder.AddEvent(Event{
        Kind:   "cache.operation",
        Name:   "settings",
        Status: "hit",
    })
}
```

The important point is:

- context carries the recorder reference
- the recorder owns the event accumulation

## Lifecycle Model

Each execution should have a lifecycle:

1. begin execution at ingress
2. attach source, execution id, and recorder to context
3. emit structured events during execution
4. finish the execution with final status
5. persist or finalize summary state

Conceptually:

```go
ctx, recorder := beginExecution(ctx, app.SourceHTTP, "GET /api/v1/hello")
defer recorder.Finish("ok", nil)
```

For failures:

```go
defer func() {
    recorder.Finish(statusFromErr(err), err)
}()
```

The persistence surface should not require every primitive call to know how
storage works. It should just emit structured events against the recorder.

## Source-Specific Identity

The record should carry source-specific summary fields.

### HTTP

- method
- route
- status code
- duration
- user or session identity when available

Suggested name:

- `GET /api/v1/hello`

### Jobs

- queue
- job name
- attempt
- success or failure
- duration

Suggested name:

- `monitoring:check`

### Scheduler

- scheduler job name
- target kind
- success, failure, or skipped
- duration

Suggested name:

- `monitor:poll`

### CLI

- command name
- args summary
- exit status
- duration

Suggested name:

- `monitor:retention`

### Startup

- boot step or initialization phase
- success or failure
- duration

This is likely lower priority in the UI, but should still fit the same model.

## What Should Be Inspectable

For one execution, Lighthouse should eventually surface:

- summary metadata
- source and timing
- trace or span tree
- logs correlated to the execution
- framework primitive activity
- errors and failures
- slow operations

### Primitive Activity

Framework-level primitive panels are one of the most valuable parts of this
feature because GoForj already knows how to observe them.

Initial primitive panels should include:

- logs
- HTTP
- cache
- storage
- events
- queue
- scheduler
- database
- mail

For each panel, the UI should prefer high-signal summaries over raw exhaust.

Examples:

- logs with level, message, timestamp, and correlated fields
- cache operations with hit/miss/error and latency
- storage operations by disk and operation
- event publishes and deliveries
- queue enqueue/process/retry/archive activity
- database query summaries and slow-query highlights

## Relationship to Tracing

Tracing is necessary, but the product should not stop at raw span trees.

Raw traces answer:

- what spans ran
- parent-child relationships
- durations

Execution inspection should additionally answer:

- what framework activity happened
- what logs relate to this execution
- what primitive operations were touched
- what failed first
- what likely caused the user-visible issue

So the relationship should be:

- tracing provides backbone correlation
- execution inspection provides the operator product

The reusable substrate should support both.

That means the recorder and execution record model should not assume:

- Lighthouse-only fields
- one UI layout
- one tracing backend

## Correlation Model

At minimum, the system needs a stable execution correlation primitive.

That can be built from:

- context propagation
- source stamping
- trace/span context when enabled
- generated middleware and handler boundaries

The framework should establish correlation at ingress boundaries:

- HTTP server request boundary
- queue handler execution boundary
- scheduler task boundary
- CLI app runner boundary
- startup/bootstrap boundary

Everything else should flow from that.

The correlation key set should stay small:

- execution id
- source
- trace/span context
- optional execution name metadata

That is enough to correlate richer records elsewhere without bloating context.

## Where Ownership Belongs

### Sibling libraries

Sibling libraries should remain responsible for:

- accepting and preserving context
- exposing observer hooks
- not losing execution context in drivers

They should not own the execution inspection product.

### GoForj framework

GoForj should own:

- ingress correlation setup
- generated middleware and handler wiring
- source tagging
- framework-level instrumentation
- execution record construction
- recorder attachment to context
- execution lifecycle finalization
- correlation-aware logging hooks

### Lighthouse

Lighthouse should own:

- browsing recent executions
- filtering and searching
- detail view rendering
- timeline presentation
- correlated log/span/primitive panels

This is important:

- the libraries provide signals
- GoForj wires execution semantics
- Lighthouse is the first operator UI consumer

## Suggested UI Shape

### Execution List

Primary columns:

- time
- source
- name
- status
- duration
- primary error summary when present

Filters:

- source
- status
- time range
- route
- queue
- job name
- command

### Execution Detail

High-level sections:

1. Summary
2. Timeline
3. Logs
4. Trace / spans
5. Primitive activity
6. Errors

The detail page should bias toward fast diagnosis, not raw exhaust.

## Timeline Direction

The most valuable UI is likely a merged execution timeline that shows:

- request or task start
- logs
- queue or event activity
- cache and storage operations
- database hotspots
- exceptions
- completion

This does not need to be pixel-perfect distributed tracing on day one.
It just needs to let the operator reconstruct what happened.

## Sensible v1 Scope

The right v1 is narrower than “build a full execution inspection platform.”

Recommended v1:

- recent execution list for `http`, `jobs`, `scheduler`, `cli`
- execution detail summary
- correlated logs as a first-class panel
- primitive summaries for:
  - logs
  - cache
  - storage
  - events
  - queue
  - scheduler
  - database
- span tree when tracing is enabled

That is already very strong.

Under the hood, v1 should still establish the reusable pieces:

- execution id on context
- recorder attachment at ingress
- normalized execution event model
- structured log correlation against execution id
- bounded retention/query surface

## Suggested v1 Non-Goals

Do not require these for the initial version:

- replaying executions
- full payload capture for every primitive
- infinite retention
- tenant-aware access control
- custom business-event plugins
- deep per-query explain plans

Those can come later.

## Data Capture Strategy

The framework should be selective about what gets retained.

Good defaults:

- summary-first records
- bounded recent history
- sampled or truncated large payloads
- primitive summaries instead of full payload dumps by default

This is especially important for:

- database queries
- HTTP bodies
- event payloads
- queue payloads
- log field redaction

The inspection product should be useful without turning into a PII leak or a
memory sink.

That is another reason not to store full inspection payloads directly in
context. The recorder layer needs the freedom to redact, truncate, sample, and
summarize.

## Local Development vs Production

The product should work in both environments, but not identically.

### Local development

Best fit for:

- richer payload visibility
- shorter retention
- frequent inspection
- request-by-request debugging

### Production

Best fit for:

- stricter limits
- redaction
- bounded retention
- summary-first inspection

This should be an explicit configuration choice, not an accidental behavior
difference.

## Why This Should Live In Lighthouse

Lighthouse is already the natural operator surface for:

- runtime state
- process inspection
- source-aware views
- dev and ops diagnostics

Execution inspection fits that direction better than Grafana or raw logs do.

Grafana should stay focused on:

- metrics dashboards
- trend analysis
- aggregated operational views

Lighthouse should own:

- concrete execution inspection
- request and job diagnosis
- near-real-time operator debugging

## Risks

### Overbuilding too early

The biggest risk is trying to build a tracing platform instead of an inspection
product.

Avoid:

- inventing too much storage architecture up front
- building an abstract telemetry pipeline before the UI needs are clear
- trying to support every source or primitive in v1

### Capturing too much data

A second major risk is recording too much raw data too early.

This increases:

- cost
- noise
- privacy risk
- implementation complexity

### Confusing runtime and source again

The framework has now flattened attribution to `source`.

This design should not reintroduce an overlapping `runtime` concept inside the
inspection product. Use `source` consistently.

## Dependencies This Design Assumes

This design assumes:

- source propagation across framework ingress points
- sibling-library context preservation across drivers
- execution-boundary context ownership
- framework metrics and observer hooks continuing to read from context

That work has already started, and some of it is now explicitly tested.

## Recommended Next Steps

1. Define the execution record and recorder model outside the UI layer.
2. Add context helpers for execution id and recorder attachment.
3. Attach recorders at framework ingress boundaries.
4. Choose the minimal retention strategy for recent executions.
5. Add a thin Lighthouse execution list for recent `http` requests first.
6. Add execution detail with correlated logs and primitive summaries.
7. Expand to `jobs`, `scheduler`, and `cli`.
8. Layer span-tree inspection in when tracing is enabled.

## Phased Delivery

This should be built in phases.

### Phase 1: Substrate

Build the reusable execution telemetry substrate first.

Scope:

- execution id
- recorder attachment to context
- execution lifecycle begin/finish
- normalized event model
- bounded retention for recent executions

No rich UI is required yet beyond whatever minimal hooks help validate the
capture model.

### Phase 2: HTTP-First Inspection

Make the first useful operator surface around HTTP.

Scope:

- recent execution list for HTTP requests
- execution detail summary
- correlated logs
- primitive summaries

This gives the fastest feedback loop because HTTP is usually the easiest source
to exercise repeatedly during development.

### Phase 3: Jobs, Scheduler, and CLI

Expand the same model to non-HTTP execution sources.

Scope:

- queue job inspection
- scheduler run inspection
- CLI command inspection

This is where the source model pays off heavily because the same substrate can
cover very different ingress surfaces without inventing new concepts.

### Phase 4: Trace-Backed Deep Inspection

Layer richer tracing and span-tree views on top of the execution substrate.

Scope:

- trace tree rendering
- span-linked primitive summaries
- richer timing and dependency views

At this point Lighthouse becomes a stronger execution diagnosis tool rather than
just a recent-activity browser.

## Logs As A First-Class Signal

Logs should not be treated as an afterthought or merely as one more tab.

In practice, logs are often the fastest way to answer:

- what failed
- what branch was taken
- what identifiers were involved
- what happened immediately before an error

So the system should treat logs as a first-class execution signal alongside
other primitive activity.

That means:

- logs should be correlated to execution id
- logs should be queryable by source and execution
- logs should appear in the execution timeline
- structured log fields should be preserved where possible

Conceptually, logs should be captured through the same recorder substrate:

```go
recorder.AddEvent(Event{
    Kind:  "log",
    Level: "error",
    Name:  "user lookup failed",
})
```

This does not mean every logger call in the app must be rewritten manually.
It means the framework should make it easy for logging to correlate with the
active execution.

Useful log fields for the execution view include:

- timestamp
- level
- message
- source
- execution id
- trace id when present
- selected structured fields

The UI should support both:

- a summary of notable logs
- a complete execution-scoped log stream when needed

## Implementation Heuristics

If this work starts, keep these rules:

1. Start from execution inspection, not trace infrastructure.
2. Prefer one execution detail page over many unrelated panels.
3. Reuse existing source/context propagation instead of inventing new identity.
4. Keep context lightweight; carry recorder handles, not giant payloads.
5. Keep primitive instrumentation summary-first.
6. Keep Lighthouse as the first UI owner and GoForj as the wiring owner.

## Storage And Retention Direction

The live execution inspection store should default to a lightweight cache-backed model rather than a database-backed model.

Recommended shape:

- execution header record
- execution event stream or chunk list
- recent execution indexes
- source-scoped indexes

This maps naturally to named cache buckets and keeps the model cheap for local development and production.

Recommended backend posture:

- local development: in-memory cache
- shared or production mode: Redis
- relational database: not the default live inspection backend

This fits the workload because execution inspection data is:

- high write volume
- short lived
- naturally bounded
- primarily for recent inspection, not long-term analytics

## Bounded Retention

Trace inspection data should always be finite and explicitly configured.

We should not allow unbounded growth of:

- execution headers
- event payloads
- correlated logs
- span summaries

At minimum the system should define:

- maximum retained execution count
- maximum concurrent in-flight executions
- maximum retained events per execution

The intended model is:

- retain only a finite number of recent executions
- overwrite older executions once the configured cap is exceeded
- keep storage footprint fixed and predictable

## Configuration Shape

The bounded nature of the system should be visible in configuration.

Conceptual environment shape:

```text
LIGHTHOUSE_TRACE_ENABLED=true
LIGHTHOUSE_TRACE_DRIVER=memory
LIGHTHOUSE_TRACE_MAX_TOTAL=1000
LIGHTHOUSE_TRACE_MAX_INFLIGHT=100
LIGHTHOUSE_TRACE_MAX_EVENTS=500
```

If Redis is used:

```text
LIGHTHOUSE_TRACE_DRIVER=redis
LIGHTHOUSE_TRACE_CACHE=execution_inspection
LIGHTHOUSE_TRACE_MAX_TOTAL=5000
LIGHTHOUSE_TRACE_MAX_INFLIGHT=250
LIGHTHOUSE_TRACE_MAX_EVENTS=1000
```

The exact names can change, but the design should preserve these concepts:

- a fixed-size retained inspect store
- bounded in-flight recorder pressure
- a finite per-record payload

## Pruning Strategy

Pruning should be deterministic and cheap.

Recommended approach:

- keep in-flight execution state in memory during the execution
- persist summary and bounded event payloads at execution finish
- assign each finished execution to a fixed slot in a bounded retained store
- advance a monotonic write cursor on each finished execution
- overwrite the prior slot occupant in place when capacity is reached
- keep a lookup index from execution id to slot for direct detail reads

This should happen at the execution store layer, not in UI code or recorder code.

## Relationship To Tracing

Execution inspection storage is not meant to replace durable tracing backends.

The expected split is:

- execution inspection store
  - recent, bounded, highly queryable operational records
- tracing backend
  - longer-lived span storage and distributed trace analysis

This lets Lighthouse inspect recent executions cheaply without requiring a heavyweight database or a full tracing backend in every environment.

## Open Questions

These questions do not need to block initial implementation, but they should be
resolved deliberately.

### Storage Model

- Should recent executions live in memory first, with optional persistence
  later?
- Should the substrate support both in-memory and persisted backends from the
  start, or should that come later?

### Retention

- What is the right default retained execution count?
- Should the fixed-slot store be the only model, or do we still want an alternate time-based mode later?

### Log Capture Shape

- Should logs be recorded directly through the recorder path?
- Or should logs be correlated later by execution id after they are emitted?
- If both are supported, which is the default model?

### Redaction

- What fields should be redacted by default?
- Where should redaction happen:
  - at emit time
  - at record finalization time
  - at query/render time

### Trace Dependency

- Is tracing optional in v1?
- If tracing is absent, what is the minimum useful execution detail experience?

### Payload Visibility

- Which payload types are safe to expose by default?
- How much body or payload detail should be retained for:
  - HTTP
  - queue jobs
  - events
  - database queries

## Bottom Line

The right direction is not just “add tracing.”

The right direction is:

- use tracing and context propagation as the correlation backbone
- attach a reusable execution recorder at ingress
- build a reusable execution telemetry substrate first
- build a Lighthouse execution inspection product on top of that
- make one request, one job, or one scheduler run easy to understand

That is the execution inspection experience GoForj should aim for.
