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

## Implementation Status

This section tracks the current state of the design against the codebase so the
remaining work is explicit.

### Done

- [x] Reusable substrate exists
- [x] Recorder attachment to context
- [x] Execution lifecycle begin/finish
- [x] Normalized inspect event model
- [x] Bounded capture configuration
- [x] Finished-inspect batch publish to Lighthouse
- [x] HTTP-first operator surface exists
- [x] Recent request list
- [x] Inspect detail summary
- [x] Merged event timeline
- [x] Request and response tabs
- [x] Copy-as-curl
- [x] Execution-scoped log events are captured and shown in the timeline
- [x] Publish/drop metrics exist for Grafana and alerting
- [x] Buffer depth metric
- [x] Dropped metric
- [x] Flushes metric
- [x] Flush errors metric
- [x] Flush batch size metric
- [x] Flush duration metric
- [x] Grafana inspect dashboard exists and is seeded

### Partial

- [ ] Phase 3 non-HTTP inspection is fully productized
- [x] `jobs`, `scheduler`, and `cli` capture exists
- [ ] UI is no longer biased toward HTTP in non-HTTP sources
- [ ] Source-specific list/detail surfaces are complete
- [ ] Filtering and search are fully source-aware
- [x] Source, status, time, and free-text filters exist
- [ ] Route-specific filtering is first-class
- [ ] Job-specific filtering is first-class
- [ ] Command-specific filtering is first-class
- [ ] Logs are a fully first-class signal
- [x] Logs appear in the merged timeline
- [x] Structured log handling exists
- [ ] Stronger execution-scoped log experience exists
- [ ] Redaction and truncation defaults are clearly locked down

### Not Started / Open Product Work

- [ ] Phase 4 trace-backed deep inspection
- [ ] Trace tree rendering
- [ ] Span-linked primitive summaries
- [ ] Richer timing/dependency views
- [ ] Live incremental inspect streaming
- [ ] Secondary persistence/export beyond Lighthouse's live browse window
- [ ] Source-specific retention policy
- [ ] Source-specific sampling policy

### Near-Term Backlog

- [ ] Finish Phase 3 UI for `jobs`, `scheduler`, and `cli`
- [ ] Add first-class route / job / command filters
- [ ] Improve execution-scoped log presentation
- [ ] Lock down redaction and truncation defaults
- [ ] Decide whether Phase 4 trace views should move forward now

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

The live execution inspection path now has a concrete v1 direction:

- process-local capture in the source runtime
- Lighthouse as the primary sink for finished inspects
- Lighthouse as the retained recent-view owner

Recommended shape:

- process-local in-flight execution state only while the execution is active
- no source-runtime retained finished inspect history as the default model
- finished inspect batches streamed from app processes into Lighthouse
- Lighthouse-owned recent aggregation and browseability

Optional secondary shape later:

- no durable local fallback when Lighthouse is unavailable; dropped inspects are acceptable in v1
- optional secondary export/persistence later if product needs history beyond Lighthouse's live window

Recommended backend posture:

- local development: process-local memory + Lighthouse live sink
- production default: process-local memory + Lighthouse live sink
- relational database: not the default live inspection backend
- shared cache: optional later only if Lighthouse itself needs a secondary persistence/export layer

This fits the workload because execution inspection data is:

- high write volume
- short lived
- latency-sensitive on the write path
- primarily for recent inspection, not long-term analytics

The explicit tradeoff is:

- source runtimes keep write cost low by capturing locally and publishing only once
- Lighthouse fan-in centralizes product logic where it belongs
- cross-process browsing becomes Lighthouse's job instead of every app process writing through a shared store

## Bounded Retention

Inspect data must always be finite and explicitly configured.

We should not allow unbounded growth of:

- in-flight recorder state
- event payloads per inspect
- outbound publish buffers
- Lighthouse-retained recent inspect history

The v1 bounded model is split by responsibility:

- source runtimes
  - bounded in-flight execution recorders
  - bounded events per inspect
  - bounded non-blocking outbound buffer to Lighthouse
- Lighthouse
  - bounded retained finished inspect store

The intended model is:

- source runtimes hold active execution state only while an inspect is running
- source runtimes do not retain finished inspect history as the default source of truth
- Lighthouse retains a finite recent inspect window
- the publish buffer is finite and drops under pressure rather than blocking traffic

## Configuration Shape

The bounded nature of the system should be visible in configuration.

Current conceptual shape:

```text
LIGHTHOUSE_INSPECT_ENABLED=true
LIGHTHOUSE_INSPECT_MAX_TOTAL=250
LIGHTHOUSE_INSPECT_MAX_INFLIGHT=100
LIGHTHOUSE_INSPECT_MAX_EVENTS=200
LIGHTHOUSE_INSPECT_SAMPLE_RATE=1.0
LIGHTHOUSE_INSPECT_BUFFER_SIZE=4096
LIGHTHOUSE_INSPECT_FLUSH_INTERVAL=1s
LIGHTHOUSE_INSPECT_FLUSH_BATCH_SIZE=100
```

If production sampling is enabled:

```text
LIGHTHOUSE_INSPECT_MAX_TOTAL=1000
LIGHTHOUSE_INSPECT_MAX_INFLIGHT=250
LIGHTHOUSE_INSPECT_MAX_EVENTS=200
LIGHTHOUSE_INSPECT_SAMPLE_RATE=0.1
LIGHTHOUSE_INSPECT_BUFFER_SIZE=4096
LIGHTHOUSE_INSPECT_FLUSH_INTERVAL=1s
LIGHTHOUSE_INSPECT_FLUSH_BATCH_SIZE=100
```

The design should preserve these concepts:

- Lighthouse-owned bounded retained inspect history
- bounded in-flight recorder pressure in source runtimes
- a finite per-record payload
- optional production sampling
- Lighthouse as the primary sink
- bounded non-blocking outbound buffering

## V1 Decisions

The following decisions are now explicit for the first implementation:

- app processes push finished inspects only
- live incremental streaming is out of scope for v1
- if Lighthouse is unavailable, new outbound inspects are dropped
- the app never blocks request traffic waiting on Lighthouse
- outbound transport is batched
- default batch size target is `100`
- default flush interval target is `1s`
- sampling happens at `Begin()`
- sampled-out inspects are skipped entirely
- no error-promotion bypass for sampled-out requests in v1
- app owns capture + enqueue only
- Lighthouse owns ingest, browse retention, and multi-process aggregation
- the retained browse view is count-bounded in Lighthouse

## Encrypted Batch Transport Contract

The inspect transport contract should be simple and explicit.

Connection model:

- source runtimes connect to Lighthouse over the existing agent websocket
- authentication uses one generated `LIGHTHOUSE_SECRET`
- after connection authentication, Lighthouse websocket frames should use one shared encrypted envelope
- encryption belongs at the Lighthouse connection/message layer, not per inspect endpoint

Message model:

- finished inspects are published in batches only
- transport should use a versioned batch payload contract
- all inspect batches should flow through the same encrypted websocket message path as other Lighthouse control-plane traffic

Recommended conceptual shape:

```go
type InspectBatchEnvelope struct {
    Version int      `json:"version"`
    Source  string   `json:"source"`
    SentAt  time.Time `json:"sent_at"`
    Records []Record `json:"records"`
}
```

The outer control-plane envelope should continue to carry:

- message type
- message id
- source
- encrypted payload

The important behavioral contract is:

- source runtimes publish finished inspect batches
- Lighthouse ingests them idempotently enough for recent browsing
- no request path work waits on network flush
- all post-auth Lighthouse traffic uses the same encrypted message framing

## Outbound Buffer Design

The app-to-Lighthouse publish path should use a bounded single-consumer outbound ring buffer.

Goals:

- request traffic never blocks on Lighthouse I/O
- memory use stays bounded
- enqueue work stays as small as possible
- drop behavior is explicit under pressure

Recommended shape:

- inspect capture stays local while the execution is running
- on `Finish`, the app serializes one immutable outbound payload
- the request path enqueues only that payload pointer
- one background flusher drains the queue and publishes batches to Lighthouse

Suggested internal types:

```go
type outboundInspect struct {
	traceID string
	source  string
	buf     []byte
	size    int
	errLike bool
}

type inspectRing struct {
	mask  uint64
	slots []unsafe.Pointer // *outboundInspect
	head  atomic.Uint64    // consumer
	tail  atomic.Uint64    // producers
	drops atomic.Uint64
}
```

Design rules:

- ring capacity should be a power of two
- queue entries should be immutable after enqueue
- request path should enqueue pointers, not large struct copies
- one background goroutine should own drain + batch flush
- request path should never take a global flush lock
- request path should never wait on Lighthouse network writes

Hot-path target:

1. finish inspect locally
2. serialize once into a compact outbound payload
3. enqueue pointer into the bounded ring
4. return

Background flush target:

1. drain up to `batch_size`
2. publish one batch to Lighthouse
3. recycle payloads later if pooling proves necessary

Initial drop policy:

- if the buffer is full, drop the new inspect immediately
- increment dropped counters atomically
- emit a rate-limited warning log

That keeps production traffic predictable even when Lighthouse is unavailable or slow.

Recommended initial knobs:

```text
LIGHTHOUSE_INSPECT_BUFFER_SIZE=4096
LIGHTHOUSE_INSPECT_FLUSH_INTERVAL=1s
LIGHTHOUSE_INSPECT_FLUSH_BATCH_SIZE=100
```

## Metrics And Drop Contract

The Lighthouse inspect publish path must expose a stable operational contract.

Required metrics:

- `lighthouse.inspects.buffer.depth`
  - gauge
  - labels: `source`
- `lighthouse.inspects.dropped`
  - counter
  - labels: `source`, `reason`
- `lighthouse.inspects.flushes`
  - counter
  - labels: `source`, `status`
- `lighthouse.inspects.flush.errors`
  - counter
  - labels: `source`, `reason`
- `lighthouse.inspects.flush.batch_size`
  - histogram
  - labels: `source`
- `lighthouse.inspects.flush.duration`
  - histogram
  - labels: `source`

Required drop reasons in v1:

- `buffer_full`
- `marshal`
- `not_connected`

Required operational behavior:

- drops are counted, not silently ignored
- drop logs are rate limited
- request traffic still returns immediately when the buffer is full
- Grafana dashboards should treat drop metrics as first-class health signals

Required dashboard views:

- current buffer depth by source
- dropped inspect rate by source and reason
- flush batch size distribution
- flush latency distribution
- flush error rate by source and reason

Recommended initial behavior under pressure:

- drop newest on full buffer
- keep the request path non-blocking
- consider source-aware or error-aware priority later only if needed

## Transport Security

Transport security must be automatic and touchless.

Users should not need to create or manage certificates manually.

The intended v1 posture is:

- GoForj generates a unique `LIGHTHOUSE_SECRET` automatically
- app processes and Lighthouse both receive that secret through generated config/wiring
- websocket authentication uses that secret
- after authentication, Lighthouse traffic uses a shared encrypted message envelope
- encryption is handled once at the Lighthouse connection/message layer
- feature-specific payload code should not implement its own crypto

The design goal is:

- secure-by-default Lighthouse traffic
- unique generated secret per project/environment
- no manual certificate authoring burden on users
- one shared encryption model for Lighthouse websocket traffic

The key rule remains:

- request traffic should pay only local capture + enqueue cost
- Lighthouse transport and flush cost must stay off the request path

## Pruning Strategy

Pruning should be deterministic and cheap inside Lighthouse.

Recommended approach:

- keep in-flight execution state in memory during the execution
- finalize the inspect in-process
- stream the finalized inspect to Lighthouse
- let Lighthouse own multi-process aggregation and recent browse ordering
- keep only a bounded non-blocking outbound buffer between app and Lighthouse
- drop new outbound inspects when that buffer is full
- keep Lighthouse retention count-bounded for the recent inspect surface

The system should avoid making a shared backend mandatory for every inspect write.

This should happen at the execution store layer, not in UI code or recorder code.

## Relationship To Tracing

Execution inspection storage is not meant to replace durable tracing backends.

The expected split is:

- execution inspection store
  - process-local capture and bounded outbound buffering
  - Lighthouse-owned live aggregation and browseability
- tracing backend
  - longer-lived span storage and distributed trace analysis

This lets Lighthouse inspect recent executions without forcing every app process through a shared backend on the hot path.

## Open Questions

These questions do not need to block initial implementation, but they should be
resolved deliberately.

### Storage Model

- Should Lighthouse ingest only finalized inspect payloads forever, or do we eventually want optional live incremental streaming?
- Does Lighthouse need a secondary persistence/export layer later, or is its live window enough?

### Retention

- What is the right default retained execution count?
- Do we eventually need a time-based backup expiry in Lighthouse beyond count-bounded recent history?
- Should Lighthouse retention stay global, or become source-specific later?

### Sampling

- Should sampling stay global only, or become source-specific later?
- Should successful high-volume HTTP traffic sample differently from jobs/scheduler traffic?

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
