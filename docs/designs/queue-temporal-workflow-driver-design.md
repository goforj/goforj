# Queue Temporal Workflow Driver Design

## Status

- Proposed architecture, not scheduled for a release.
- This design replaces the assumption that Temporal requires a separate GoForj
  component or sibling workflow library.
- Phase 0 must prove state projection, lifecycle, and compatibility before the
  framework exposes configuration or generators.

## Review Record

Five serial reviews were applied to this design. Each pass reviewed the result
of the prior pass rather than the original proposal.

| Pass | Focus | Finding resolved |
| --- | --- | --- |
| 1 | Queue API and driver construction | The existing bridge erases optional capabilities and the current workflow engine owns ordinary queue operations; an internal responsibility split and construction descriptor are prerequisites |
| 2 | Identity, state, and error semantics | Queue IDs, Temporal workflow and run IDs, ambiguous acceptance, complete state reconstruction, and not-found translation now have explicit contracts |
| 3 | GoForj generation and runtime lifecycle | Existing queue-resource selection remains authoritative, workflow-specific worker modes were removed, and partial startup now requires rollback |
| 4 | Enterprise operation and upgrades | Framework workflow definitions are versioned durable protocol, task queues are not security boundaries, and credential, codec, retention, and replay ownership are explicit |
| 5 | Adversarial compatibility and simplification | Typed option provenance, retry-layer ownership, batch cancellation interleavings, observer replay safety, and independently releasable milestones are explicit gates |

The reviews found no reason to create a new top-level component or sibling
library. They did find prerequisites that block an immediate implementation.
Those prerequisites are represented below as Phase 0 decisions and release
gates. An unproven gate disables the affected capability; it does not permit a
silent behavior change.

## Summary

GoForj should integrate Temporal as an optional driver in
`github.com/goforj/queue`.

The queue library already owns two related but distinct responsibilities:

1. Physical delivery of ordinary jobs through sync, worker pool, SQL, Redis,
   NATS, SQS, and RabbitMQ runtimes.
2. Workflow orchestration through durable chains, batches, state stores,
   callbacks, recovery, testing helpers, and lifecycle events.

Temporal can own both responsibilities when it is the selected driver.
Ordinary queue jobs run as framework-owned, single-activity workflows. Chains
and batches use a driver-native workflow capability so Temporal owns their
history, replay, and state transitions. Temporal must not implement
`queue.WorkflowStore`.

The integration has two levels:

- Existing `queue.Chain` and `queue.Batch` operations may run through the
  Temporal driver's native coordinator when their semantics can be preserved.
- Applications may register native Temporal workflows for capabilities that
  cannot be represented by static chains and batches, including signals,
  updates, durable timers, child workflows, queries, and continue-as-new.

Existing drivers continue to use queue's built-in workflow coordinator.
Temporal advertises a native workflow capability that queue detects during
construction. This keeps one driver selection and one orchestration authority
per queue instance without hiding Temporal's programming model behind an
inaccurate portable abstraction.

## Existing Boundary

The high-level queue already composes a driver runtime and an internal engine:

```text
queue.Queue
├── queueRuntime
│   ├── ordinary job dispatch
│   ├── handler delivery
│   └── physical worker lifecycle
└── workflow.Engine
    ├── ordinary dispatch and handler registration
    ├── physical worker lifecycle delegation
    ├── chain and batch orchestration
    ├── workflow state
    └── workflow lifecycle events
```

This is close to the integration seam, but it is not yet a safe extension
point. The current `workflow.Engine` also implements ordinary `Dispatch`,
`DispatchDirect`, `Register`, `StartWorkers`, and `Shutdown`. Replacing that
whole interface would change ordinary job delivery and violate this design's
central compatibility promise.

Queue must first separate three internal responsibilities without changing its
public API:

```text
queue.Queue
├── selected driver runtime
├── shared job registry and executor
└── workflow coordinator
    ├── built-in coordinator for ordinary drivers
    └── driver-native coordinator when the driver provides one
```

`Queue.Dispatch` always uses the selected driver. Both coordinator paths use
the shared registry and executor for application jobs. `Chain`, `Batch`,
`FindChain`, `FindBatch`, and workflow-specific lifecycle behavior delegate to
the driver's native capability when present and otherwise use the built-in
coordinator.

The exact private interfaces are a queue-repository decision. GoForj must not
design them through generated adapters or expose them solely for framework
convenience.

The Temporal driver must not feed chain and batch envelopes back through the
built-in coordinator. That would create two orchestration authorities: queue
would persist transitions while Temporal persisted a second history for
delivery wrappers. Driver-native delegation is what makes the single-driver
model valid.

### Construction Seam Required Before The Driver

The current optional-driver bridge adapts a backend to queue's private
`driverQueueBackend` and `driverRuntimeQueueBackend` interfaces before the
high-level queue is constructed. That adaptation intentionally erases unknown
optional interfaces. A Temporal module therefore cannot advertise native
workflow coordination through the existing bridge unchanged.

The queue repository must add one internal construction descriptor that carries
the ordinary driver runtime and optional domain-neutral capabilities together.
The bridge may expose that descriptor to nested driver modules, but the root
queue module must own and validate it. Construction must reject a native
coordinator whose lifecycle owner, job executor, or driver identity differs
from the ordinary runtime in the same descriptor.

This change must also remove ordinary registration, dispatch, and worker
lifecycle ownership from the current `workflow.Engine`. Today `Queue.Register`,
`Queue.Dispatch`, `Queue.StartWorkers`, and `Queue.Shutdown` all call that
engine. The refactor is complete only when those methods use the shared job
runtime directly and only chain, batch, state, pruning, and workflow events use
the selected coordinator.

Current high-level options also collapse store, clock, and middleware settings
into opaque `workflow.Option` functions. A native driver cannot inspect those
functions to validate its capabilities. Queue must retain typed internal option
provenance, including whether a value was explicitly supplied, while preserving
the public `queue.Option` API. The built-in coordinator and native coordinator
then consume the same parsed configuration, and construction can truthfully
reject unsupported combinations before any client or worker starts.

The construction refactor and its existing-driver contract tests must land
before the Temporal module. Shipping both in one indivisible change would make
regressions difficult to attribute and rollback.

## Goals

1. Keep all queue and workflow concepts in the existing queue ecosystem.
2. Preserve every existing driver's behavior when Temporal is compiled in but
   not selected.
3. Preserve established chain and batch behavior where Temporal can implement
   it truthfully.
4. Let advanced workflows use the Temporal Go SDK directly.
5. Keep Temporal dependencies out of projects that do not compile the driver.
6. Reuse GoForj runtime hosting, dependency injection, generation, development
   services, observability, and shutdown policies.
7. Make determinism, payload persistence, retry translation, and unsupported
   capabilities explicit.

## Non-Goals

1. Implementing `queue.WorkflowStore` over Temporal.
2. Running the built-in queue coordinator over Temporal delivery wrappers.
3. Reproducing Temporal's full API as portable queue interfaces.
4. Reimplementing Temporal Web in Lighthouse.
5. Managing production Temporal clusters or Temporal Cloud accounts.
6. Claiming transparent parity before the compatibility contracts pass.

## Single Driver Model

Temporal is selected through the existing queue driver contract:

```text
QUEUE_DRIVER=temporal
QUEUE_SUPPORTED_DRIVERS=workerpool,redis,temporal
QUEUE_ADDRESS=127.0.0.1:7233
QUEUE_NAMESPACE=default
QUEUE_NAME=my-app-jobs
QUEUE_TLS_ENABLED=false
QUEUE_API_KEY=
QUEUE_TLS_CERT_PATH=
QUEUE_TLS_KEY_PATH=
QUEUE_SHUTDOWN_TIMEOUT=60s
```

The exact driver placeholders are a Phase 0 output and must follow the queue
resource's existing root, named, and App-prefixed environment rules. GoForj
adds `temporal` to `ResourceQueue`, backed by `ServiceQueueTemporal`. The
existing Temporal developer-service entry lists that service in `Providers`,
so local service planning follows from a real queue consumer.

No `ResourceWorkflows`, `ResourceQueueWorkflow`, `WORKFLOWS_*`, or
`QUEUE_WORKFLOW_*` configuration is added.

Applications that want Redis for ordinary jobs and Temporal for workflows use
the existing named queue mechanism instead of a second configuration axis:

```text
QUEUE_DRIVER=redis
QUEUE_WORKFLOWS_DRIVER=temporal
QUEUE_WORKFLOWS_ADDRESS=127.0.0.1:7233
QUEUE_WORKFLOWS_NAMESPACE=default
QUEUE_WORKFLOWS_NAME=my-app-workflows
```

`WORKFLOWS` in those keys is the application-chosen name of a queue resource.
It is not a hidden framework workflow component.

Code dispatches durable workflows through that generated queue instance:

```go
workflowID, err := app.Queues().Workflows().Chain(first, second).Dispatch(ctx)
```

This composition is already how GoForj represents queue backends with distinct
delivery, worker, scaling, and failure policies. It avoids teaching the
project model that every queue contains two independently selected engines.

## Local Development Service

Add `ServiceQueueTemporal` to the existing Temporal developer service's
`Providers`. Selecting a local Temporal queue then activates the profile
through normal service planning. Merely compiling the driver does not start the
service.

The existing container image pin and health command must be validated against
the selected SDK before implementation. The design must not assume that an
orphaned development-service template is current or compatible simply because
it already exists.

Local defaults must state whether history survives `forj dev down` and restart.
An ephemeral default is acceptable when it is clearly reported and prevents
stale development histories from failing replay after code edits. Persistence,
when enabled, needs a named volume and an explicit cleanup path. The generated
development service has no production authentication posture and must remain
bound and documented as local tooling.

## Public Surface

Portable workflow consumers continue to use only the established queue API,
whether the selected instance uses Redis, SQL, or Temporal:

```go
workflowID, err := app.Queue().Chain(first, second).Dispatch(ctx)
```

Native Temporal consumers use Temporal SDK types directly. Generated Wire
providers supply `client.Client` to code that starts, signals, updates, or
queries native workflows, and supply a worker registration interface to
application-owned registration code. The root queue package does not add
portable wrappers for those operations.

The optional queue Temporal module may expose Temporal-specific construction
and registration because that is an honest vendor integration. It must not add
those types to the root queue module or require them when the driver is not
compiled. No application should need to import GoForj template internals to use
the client.

When more than one Temporal queue instance is configured, generated Wire names
the driver-specific clients and registries by queue resource. It must not
inject an ambiguous unqualified `client.Client`.

## Compatibility Map

| Queue workflow behavior | Temporal implementation | Assessment |
| --- | --- | --- |
| `Chain(A, B, C)` | Execute activities sequentially | Structurally clean; failure and event contracts must pass |
| `Batch(A, B, C)` | Schedule activity futures in parallel and collect results | Structure is clean; cancellation interleavings must pass |
| Registered job handler | Generic or named Temporal activity | Signature is clean; execution contract must pass |
| Serialized job payload | Activity input | Clean within measured payload and codec limits |
| Chain or batch ID | Temporal workflow ID | Clean if ID constraints pass |
| `Retry(n)` | Activity maximum attempts set to `n + 1` | Requires explicit translation |
| `Backoff(d)` | Initial interval `d` with coefficient `1` | Requires explicit translation |
| `Timeout(d)` | Activity start-to-close timeout | Requires an explicit nonzero default |
| `Delay(d)` | Durable timer before activity scheduling | Clean |
| `OnQueue(name)` | Activity task queue | Mostly clean |
| `AllowFailures()` | Collect member failures without early workflow failure | Must pass aggregate and interleaving contracts |
| Worker restart recovery | Temporal history replay | Clean |
| `FindChain` and `FindBatch` | Workflow query or derived state projection | Must be proven |
| `Progress`, `Then`, `Catch`, and `Finally` closures | Process-local functions cannot be replayed | Not durable |
| `UniqueFor` on individual jobs | No equivalent activity-level uniqueness contract | Not clean |
| `WorkflowStore` mutation methods | Temporal owns history and transitions | Incompatible |
| `Prune(before)` | Temporal retention and administrative deletion | Incompatible |
| Queue middleware | Activity wrapper or Temporal interceptor | Must be proven |
| Fake queue workflow assertions | Temporal test environment adapter | Separate test path |

The Temporal driver must reject an unsupported option instead of silently
changing its meaning.

## Ordinary Jobs On The Temporal Driver

Temporal does not enqueue a standalone activity without workflow ownership in
the same model used here. `Queue.Dispatch` therefore starts a framework-owned
job workflow that executes one registered job activity.

```text
Queue.Dispatch(job)
        │
        ▼
start QueueJobWorkflow
        │
        ▼
execute registered job activity
```

This has more persistence and scheduling overhead than Redis or a database
queue. It is nevertheless a truthful driver choice when an application values
Temporal's history, recovery, and operational tooling more than low-cost job
throughput. GoForj documentation must keep Redis, SQL, and broker drivers as
the recommendation for high-volume, independent jobs.

Dispatch acceptance means the Temporal service accepted the workflow start. It
does not mean the activity completed. Workflow IDs, reuse policy, retry policy,
and cancellation behavior must be stable parts of the Temporal driver
contract.

Queue identity remains authoritative at the public boundary:

| Queue value | Temporal value | Contract |
| --- | --- | --- |
| ordinary `DispatchResult.DispatchID` | framework workflow ID or a reversible component of it | Stable before the start request and stable across reconciliation |
| chain ID returned by `Dispatch` | workflow ID | The same value accepted by `FindChain` |
| batch ID returned by `Dispatch` | workflow ID | The same value accepted by `FindBatch` |
| chain node ID or batch job ID | activity ID | Deterministic and stable across workflow replay |
| Temporal run ID | driver diagnostic metadata | Never substituted for a queue ID |

Queue IDs must satisfy Temporal's workflow and activity ID constraints without
truncation or lossy rewriting. If the existing ID format cannot be used
directly, the driver needs a collision-resistant reversible encoding and tests
at the maximum supported length.

Client cancellation or a network timeout can leave workflow-start acceptance
ambiguous. The driver must allocate the workflow ID before the request, return
the established queue identity with any ambiguous error where the current API
permits it, and safely resolve or retry the same start identity. For ordinary
jobs this is `DispatchResult.DispatchID`; for a chain or batch it is the ID
returned beside the error. A later already-started response is evidence to
reconcile, not automatically a new failure or permission to generate another
workflow ID.

## Portable Chain And Batch Workflows

The Temporal integration may register two framework-owned workflow types:

- a queue chain workflow that accepts the persisted chain definition
- a queue batch workflow that accepts the persisted batch definition

Each job becomes an activity invocation. A generic activity can resolve the
job type through the same handler registry used by ordinary queue workers, so
existing handlers retain `context.Context`, payload binding, application
dependencies, and middleware behavior.

```text
queue.Chain(...).Dispatch(ctx)
          │
          ▼
start QueueChainWorkflow with chain ID
          │
          ├── execute registered job activity A
          ├── execute registered job activity B
          └── execute registered job activity C
```

The adapter must translate policy deliberately:

- Queue retry counts retries after the initial attempt. Temporal maximum
  attempts includes the initial attempt. The adapter must always set the
  maximum because Temporal otherwise permits unlimited activity retries.
- Queue backoff is a fixed duration where supported. Temporal defaults to an
  exponential coefficient, so parity requires a coefficient of `1`.
- Queue timeout is an execution-attempt boundary. The closest Temporal
  activity boundary is start-to-close timeout. Temporal requires at least one
  activity execution timeout, while queue permits an unset timeout. The
  Temporal coordinator therefore needs a documented engine default and must
  not inherit an arbitrary SDK or server default.
- Queue delay should use workflow time, never `time.Sleep` inside replayed
  workflow code.
- A job queue name can select an activity task queue. The workflow task queue
  remains separate configuration because workflow and activity pollers have
  different operational roles.

Framework-owned workflows must not add a second automatic retry layer around
activity retries. Ordinary job, chain, and batch workflow executions use no
workflow-level retry policy unless queue later defines one independently. The
activity retry policy alone implements `Retry` and `Backoff`; otherwise one
exhausted activity could cause the entire workflow to run again and exceed the
queue retry budget.

Batch failure behavior needs an interleaving contract. The built-in coordinator
dispatches members independently and marks the aggregate cancelled after the
first terminal failure when `AllowFailures` is absent. Work already accepted by
a backend can still race with that state transition. A Temporal workflow must
define whether already-started activities are requested to cancel, allowed to
finish, or ignored after aggregate cancellation, then prove the resulting
handler calls, counters, callbacks, and events match the established queue
contract. It must not rely on Temporal's default scope-cancellation behavior.
Temporal delivers activity cancellation through heartbeats, so a cancellation
claim must also define whether the generic queue activity emits framework
heartbeats and at what cost. The driver cannot imply prompt cancellation for a
handler that does not participate in that mechanism. See the
[Temporal activity timeout and heartbeat contract](https://docs.temporal.io/develop/go/activities/timeouts).

Every selected activity task queue must have a known local poller or be marked
as externally owned. An arbitrary `OnQueue` value must not schedule a Temporal
activity onto an unpolled task queue and report successful workflow acceptance.
Generated validation should fail unknown task queues before starting the
workflow.

Queue does not promise identical retry timing across every physical driver.
The Temporal coordinator must publish its exact retry schedule as an engine
contract rather than claim behavior that queue does not currently normalize.

## Handler And Error Semantics

The shared job executor must reconstruct the same `queue.Message` correlation
fields for a Temporal activity that a physical queue worker supplies. Temporal
activity attempts are one-based; queue attempts are zero-based. The adapter
must translate the number before middleware, handlers, or observers see it.

Error translation must be bidirectional and tested:

| Queue outcome | Temporal representation |
| --- | --- |
| handler success | successful activity completion |
| ordinary handler error with retry budget | retryable application failure |
| `queue.Permanent(err)` | non-retryable application failure |
| exhausted queue retry budget | final activity failure |
| handler panic | failure with an explicitly documented retry policy |
| canceled handler context | cancellation only when workflow cancellation owns it |
| infrastructure uncertainty before acceptance | start failure, not workflow failure |

The adapter must preserve the application's error category without promising
the exact concrete Go error after Temporal serialization. `errors.Is` and
`errors.As` behavior that depends on an in-process error value cannot cross a
Temporal history boundary unless the integration defines and registers a
stable encoded application-error type.

Temporal activities may execute more than once around timeout, worker loss,
and retry boundaries. Existing queue guidance requiring idempotent handlers
still applies. The design must not describe workflow replay as proof that an
activity itself executes only once.

`UniqueFor` is a separate contract. Queue uniqueness suppresses independently
dispatched logical jobs by effective queue, job type, and canonical payload
for a TTL. Temporal workflow IDs can suppress duplicate workflow starts, but
they do not provide the same per-activity, payload-based TTL behavior across
independent workflow executions. The Temporal driver must reject
`UniqueFor` until an implementation proves the established identity, scope,
TTL, concurrency, and release behavior.

## Queue Options And Capabilities

Selecting a new driver does not make every optional queue capability portable.
The Temporal module must publish the same kind of explicit capability matrix as
the existing backends.

| Existing surface | Temporal driver decision |
| --- | --- |
| `WithObserver` | Must prove replay-safe, non-duplicating event translation |
| `WithMiddleware` | Supported around the shared activity job executor |
| `WithHandlerContextDecorator` | Supported around activity handler execution |
| `WithStore` | Rejected because Temporal is the workflow state authority |
| `WithClock` | Rejected for production execution; SDK workflow time and test-environment time remain authoritative |
| constructor `WithWorkers` and `Queue.WithWorkers` | Activity execution concurrency; workflow-task concurrency remains driver configuration |
| `WithLegacyDirectEnvelope` | Rejected because it selects an obsolete physical-delivery envelope rather than a Temporal execution contract |
| `WithContext` | Supported for client calls; cancellation after acceptance does not cancel the workflow |
| `Driver` | Returns `temporal` |
| `Register` | Registers the shared handler as a Temporal activity target |
| `StartWorkers` and `Run` | Start the instance's workflow and activity pollers as one lifecycle unit |
| `Shutdown` | Drains and stops pollers, then closes the client within the supplied context budget |
| `Ready` | Supported with distinct client and worker states |
| `ListJobs` | Unsupported; visibility results do not satisfy the existing job listing contract without a proven adapter |
| `RetryJob` | Unsupported; workflow reset and activity retry are not the existing job retry operation |
| `CancelJob` | Unsupported until the ordinary dispatch identity and cancellation settlement contract are proven |
| `DeleteJob` | Unsupported; history deletion is an administrative retention operation |
| `ClearQueue` | Unsupported; Temporal has no equivalent queue-clearing contract |
| `History` | Unsupported; Temporal execution history is not the queue throughput timeline |
| `Stats` and native stats helpers | Unsupported unless values map to the established queue snapshot without fabrication |
| `Pause` and `Resume` | Unsupported until task acceptance and in-flight behavior are defined |
| `FindChain` and `FindBatch` | Supported only if the state-read release gate passes |
| `Prune` | Unsupported unless a permission-aware retention mapping is proven |
| `NewFake` and fake assertions | Remain the portable domain test surface; they do not emulate Temporal replay |

Temporal needs separate workflow-task and activity-task concurrency settings.
The established worker option controls concurrent application job execution,
so the Temporal driver maps it to maximum concurrent activity execution.
Workflow-task poller count and concurrency belong in `temporalqueue.Config`
because they do not represent additional application job handlers. Both values
must have documented defaults and bounds.

Every unsupported constructor option must fail during construction. Every
unsupported job or workflow option must fail before the service accepts an
execution. A post-construction operation with an error return must return its
established unsupported-capability error. Capability helpers must report the
same result as the operation.

## Callback Migration

The existing fluent callbacks hold Go closures in process memory. The queue
library documents them as ephemeral, and its roadmap already calls for named
durable continuations.

Temporal cannot persist or replay those closures. The Temporal driver must not
present them as durable simply because the enclosing workflow is durable.

Before portable chains and batches claim Temporal parity, queue should provide
named continuations for the durable cases:

```text
chain failure     -> named job or activity
chain completion  -> named job or activity
batch progress    -> named job or activity
batch success     -> named job or activity
batch failure     -> named job or activity
batch completion  -> named job or activity
```

Until that API exists, dispatching a Temporal-backed chain or batch with a
closure callback must fail before starting the workflow. Silently dropping the
callback or retaining it only in the starting process would create a false
durability promise.

## Workflow State

Temporal history is authoritative for a Temporal-backed execution. Queue must
not mirror imperative transitions into a `WorkflowStore` and then attempt to
keep two sources of truth synchronized.

The portable workflows should maintain deterministic `ChainState` and
`BatchState` values and expose them through registered Temporal query handlers.
Phase 0 must prove that the existing `FindChain` and `FindBatch` expectations
can be met for:

- running executions
- completed executions retained by the namespace
- worker restart and history replay
- unknown workflow IDs
- unavailable workers
- continued-as-new executions where applicable

The returned values must preserve every existing field, including dispatch ID,
queue, node and job IDs, stored job policy, batch name, counters, failure text,
and created and updated timestamps. Workflow time supplies timestamps while
workflow code is executing. Wall-clock reads inside workflow code are not
allowed. Unknown IDs must map to `queue.ErrWorkflowNotFound`; service,
authorization, history-decoding, and worker-availability failures must remain
distinguishable from not found.

If queries cannot satisfy the established availability contract, a derived
read model may be added. It must be explicitly non-authoritative, rebuildable
from Temporal history, and never participate in workflow transition ownership.
The driver must not claim portable state-read support if an active compatible
worker is required but the existing queue store could answer the same read
without one. A server-side history decoder or derived projection is required
for that case, and its upgrade compatibility must be tested with retained old
histories.
If neither queries nor a derived read model can preserve the documented read
contract, Temporal cannot ship as a drop-in coordinator for `Chain` and
`Batch`. Native Temporal workflow support may still proceed independently.

Temporal retention replaces queue workflow pruning. `Queue.Prune` should
return an explicit unsupported-capability error for Temporal-backed executions
unless a safe and permission-aware translation is proven. Namespace retention
must remain an operator decision.

## Native Temporal Workflows

Static chains and batches are not a complete Temporal programming model.
Native workflows should use `go.temporal.io/sdk/workflow` directly for:

- signals and updates
- durable timers and waiting
- queries over workflow-owned state
- child workflows
- compensation and saga control flow
- continue-as-new
- workflow versioning

GoForj should generate registration and safe file placement, not a second SDK.
Workflow functions receive `workflow.Context` and no injected I/O dependencies.
Activities receive ordinary `context.Context` and dependencies through Wire.

```go
// PaymentWorkflow has no injected dependencies because replayed code must not perform I/O directly.
type PaymentWorkflow struct{}

// NewPaymentWorkflow creates deterministic payment orchestration without application dependencies.
func NewPaymentWorkflow() *PaymentWorkflow {
	return &PaymentWorkflow{}
}

// Execute coordinates payment through a replay-safe activity call.
func (w *PaymentWorkflow) Execute(ctx workflow.Context, input PaymentInput) (PaymentResult, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
	})

	var result PaymentResult
	if err := workflow.ExecuteActivity(ctx, ChargePaymentActivityName, input).Get(ctx, &result); err != nil {
		return PaymentResult{}, err
	}
	return result, nil
}
```

An empty workflow struct is only a useful default shape. It does not enforce
determinism. Generated tests and CI must run Temporal's replay and static
workflow checks against representative histories.

## History And Scale Limits

Temporal persists commands and results in workflow history. Passing an entire
queue chain or batch definition means every job payload becomes durable
workflow input, and every scheduled activity adds history events.

The compatibility adapter must define and validate before workflow start:

- maximum encoded chain and batch input size
- maximum portable chain length
- maximum portable batch fan-out
- maximum individual job payload accepted by the configured Temporal service
- behavior when activity results or failure details approach service limits

Large batches cannot be declared universally compatible until a real-server
test proves the selected limit. If the required limit is too restrictive, the
adapter may partition work into child workflows, but that changes cancellation,
progress, and visibility behavior and therefore requires its own contract.

Long-running native workflows must use continue-as-new before history growth
approaches the service limit. GoForj may provide guidance and inspection, but
must not inject continue-as-new automatically because only application code
knows which state must cross the run boundary.

## Deployment And Workflow Evolution

Changing deterministic workflow code while executions remain open can make
history replay fail. Unit tests against only newly created histories do not
prove upgrade safety.

Before production support, the integration must define:

- how representative production histories are captured and replayed in CI
- when generated examples use SDK version markers
- how worker deployments or build identities route open executions
- rollback behavior when a new worker cannot replay old history
- how long old workflow code remains deployable relative to namespace
  retention and maximum execution duration

Activity code may change without replaying its prior implementation, but its
serialized input, output, and application-error contracts still require
compatibility. Long-running activities must heartbeat when cancellation and
retry recovery depend on progress details.

The queue Temporal module itself owns durable framework protocol. Its ordinary
job, chain, and batch workflow type names, input envelopes, activity type
names, search attributes, and command ordering cannot be treated as private
implementation details after release. Each persisted envelope needs an
explicit schema version. Decoders must retain support for every history that
can still exist under the documented retention and maximum execution window.

Every module release must replay committed history fixtures created by each
supported prior driver version. A change that cannot replay an open history
requires a new workflow type or an SDK-supported version marker plus a worker
rollout that keeps the old implementation available. Dependency updates to the
Temporal SDK are subject to the same replay gate.

## Queue Repository Ownership

Reusable Temporal integration belongs in the existing queue repository. It
does not justify a new top-level `goforj/workflow` sibling.

The preferred packaging is an optional `driver/temporalqueue` nested Go module
in the queue repository, matching the existing optional backend modules. Its
driver backend also advertises the private native-workflow capability consumed
by the root queue runtime.

The optional module isolates the Temporal dependency graph from the root queue
module. Phase 0 must select an SDK release compatible with the minimum Go
version supported by queue and generated applications. The current newest SDK
must not be adopted by silently increasing either module's Go version. If a
required SDK feature forces an increase, the design must identify that exact
constraint, its consumer impact, and the migration before implementation.

At this review, queue's root module declares Go 1.24.4. Temporal Go SDK v1.48.0
declares Go 1.25.4, while v1.45.0 declares Go 1.24.0. Version 1.45.0 is therefore
the initial compatibility candidate, not an unconditional pin. Phase 0 must
prove that it contains every required stable API and is compatible with the
selected server and local development image. Experimental SDK surfaces are not
allowed in the portable driver contract.

The module owns:

- Temporal client configuration and dialing
- workflow and activity registration
- worker lifecycle
- chain and batch compatibility workflows
- policy translation
- Temporal-specific readiness and inspection
- tests against the Temporal SDK test environment and a real development
  server

The repository's release tooling discovers nested modules and gives this module
the tag `driver/temporalqueue/vX.Y.Z`. Its `go.mod` keeps the intentional local
replace to the root queue module for repository tests, while its release-time
requirement must be pinned to the same published queue version. The module must
be added to the integration and example module inventories and to every release
dependency-closure test. A root `vX.Y.Z` tag alone does not publish the driver.

The root queue module owns the smallest additive driver-capability seam needed
to select native workflow coordination. A bridge under queue's `internal`
namespace can follow the existing optional-driver construction pattern without
exporting Temporal-specific types from the root package.

The GoForj repository owns:

- component and environment projection under Background Jobs
- generated manager and Wire integration
- workflow and activity generators
- runtime host participation
- developer-service activation
- Lighthouse presentation
- enabled and disabled render contracts
- the Temporal driver pin in the core dependency catalog, dependency catalog
  tests, render-warm module, generated fixture replacement inventory, and the
  largest generated composition

## GoForj Component Model

Temporal workflows remain part of Background Jobs. GoForj must not add a
parallel top-level Workflows component that competes with the workflow surface
already exposed by queue.

When Background Jobs is disabled, a rendered application contains no queue
workflow manager, Temporal client, worker, generated workflow package,
environment keys, commands, module dependency, or Temporal service consumer.

When the Temporal driver is not compiled, output remains unchanged from
today's render.

When a queue resource selects the Temporal driver, GoForj additionally:

- includes the optional queue Temporal module
- activates the existing Temporal development-service profile
- renders Temporal client and worker construction
- registers portable compatibility workflows and application-owned native
  workflows
- hosts that driver's Temporal workflow and activity pollers inside the Jobs
  runtime
- exposes Temporal-specific readiness and inspection without duplicating the
  Temporal Web UI

The generated `jobs.Runtime` remains the only host runtime for both ordinary
queue workers and Temporal pollers. Do not add a fourth top-level workflow
runtime. `queue:work` remains the worker command and its existing positional
filters continue to select generated queue resources. Selecting a Redis queue
starts Redis workers. Selecting a Temporal queue starts that instance's
workflow and activity pollers. With no filters, all configured queue instances
start. No separate workflow-worker mode or second set of selection flags is
added.

The Temporal driver configuration must enumerate every activity task queue it
will poll in addition to its workflow task queue. `OnQueue` selects only from
that declared set. A queue resource filter controls ownership of the whole
Temporal driver instance; it must not start only half of that instance's
required pollers.

### Task Queue Topology

One Temporal SDK worker is bound to one task queue. A queue resource that uses
different workflow and activity task queues therefore owns a set of SDK
workers, not one worker:

```text
Temporal queue resource
├── client and namespace
├── workflow-task worker
│   ├── framework workflows
│   └── application native workflows
└── activity-task workers
    ├── default queue from the existing queue `NAME`
    └── each additional declared `OnQueue` target
```

The workflow-task worker disables activity polling unless its task queue is
also an explicitly declared activity target. Activity-only workers disable
workflow polling. All activity workers register the same shared queue handler
executor. Configuration must reject duplicate, empty, or conflicting task
queue roles and must validate every effective default queue before startup.

The exact environment keys remain a Phase 0 decision, but the model has four
distinct values: queue resource identity, Temporal namespace, workflow task
queue, and the set of activity task queues. `QUEUE_NAME` retains its established
meaning as the default application job queue; it is not silently repurposed as
the workflow task queue.

### Registration And Poller Homogeneity

Every worker deployment polling the same workflow task queue must register a
compatible implementation for every workflow type that can be routed there.
Every deployment polling an activity task queue must expose a compatible
generic queue activity and the same application job-handler set expected on
that queue. Sharing a namespace and task queue does not merge or negotiate
registries.

Framework workflow and activity type names require a reserved, versioned prefix
owned by the queue Temporal module. Application registration must fail before
worker startup when a native workflow or activity collides with a reserved name
or when one name is registered with incompatible signatures. The exact names
are durable protocol and follow the driver-versioning policy.

Generated defaults must namespace workflow and activity task queues by project,
App, and queue resource consistently with existing physical queue naming. If
operators deliberately configure two Apps or queue resources to share a task
queue, deployment guidance must require identical registrations and compatible
worker versions. GoForj cannot validate a remote fleet from one process, so it
must display the resolved namespace, task queues, registration fingerprint, and
worker deployment identity for operational comparison.

The registration fingerprint is diagnostic evidence, not an authorization or
routing mechanism. It is computed from stable type names, protocol versions,
and handler identities without including source paths, payloads, or secrets.
Deployment automation should reject different fingerprints on the same task
queue unless the selected Temporal worker-versioning rollout isolates them.
Local process readiness can reject local collisions, but must not claim it has
verified registrations across a remote fleet.

## Runtime And Lifecycle

Each queue runtime owns its selected driver's workers. A Temporal runtime owns
its workflow and activity pollers plus its native workflow coordinator. Other
drivers own their physical workers and use queue's built-in coordinator. A
client-only process may construct and dispatch through either path without
starting a local worker.

GoForj's current multi-queue startup loop returns immediately when one queue
fails to start. It does not roll back queue instances that started earlier in
the loop. Temporal support must not build on that behavior. The jobs runtime
must track successful starts and shut them down in reverse start order within
the queue shutdown budget before it returns the startup error. This correction
protects every driver, not only Temporal.

Shutdown of the Temporal driver must stop new polling before waiting for
admitted work and close the client only after its worker has stopped. When the
Jobs runtime starts several queue instances, a failure to start any instance
must clean up those already started before returning.

Temporal worker startup and ongoing failure are separate channels. A successful
SDK worker start does not report a fatal poller error that occurs later. The
driver needs a terminal-result signal owned by the queue runtime. `Queue.Run`
must wait for either caller cancellation or that signal, finish the driver's
shutdown protocol, and return the fatal error. GoForj's Jobs runtime must run
selected queues through that failure-aware path so a fatal Temporal worker
causes `RuntimeHost` to cancel sibling runtimes. Readiness and logging alone are
not sufficient failure propagation.

The SDK worker stop operation is not an idempotent contract and can panic when
called repeatedly. In the candidate SDK, the fatal-error callback is also
followed by an SDK-owned stop. A driver that reacts to that callback by calling
stop again can race the SDK. The Temporal driver therefore needs a proven
lifecycle adapter that coordinates SDK-owned and driver-owned termination,
observes when each worker has actually stopped, rolls back an already-started
subset, and closes the shared client only after the set is terminal. The design
does not prescribe `Start`, `Run`, or the fatal callback until Phase 0 proves
one composition without a startup or shutdown race. This follows the
[Go SDK worker lifecycle contract](https://pkg.go.dev/go.temporal.io/sdk/worker#Worker).

Concurrent shutdown, startup rollback, fatal worker failure, and repeated
`Queue.Shutdown` calls must leave each underlying worker stopped and the shared
client closed exactly once from the queue consumer's perspective. No path may
depend on recovering an SDK panic or sleeping to infer that shutdown completed.

Phase 0 must test this ordering against the actual GoForj runtime host. It must
not assume lifecycle hooks that the runtime host does not call.

The current runtime host cancels sibling runtime contexts and waits for every
`Run` method to return. Application lifecycle shutdown hooks execute only after
the command returns. Temporal worker shutdown must therefore occur inside the
jobs runtime after its run context is canceled. Deferring it to an application
shutdown hook would make the runtime host wait for a worker that is itself
waiting for a later hook.

The failure-aware `Queue.Run` path must still honor the queue shutdown budget.
It cannot call an unbounded background shutdown after cancellation. Phase 0
must establish where that budget lives for direct queue consumers and generated
GoForj runtimes without changing the established `Run(ctx)` signature.

Temporal client creation has two modes:

- A dispatch-only process uses the SDK's lazy client so application startup
  does not require a reachable Temporal service.
- A process configured to poll uses an eager, deadline-bounded connection and
  fails startup if it cannot reach the service.

Readiness must distinguish an unconnected lazy client from an unhealthy worker.
The former is not a startup failure until an operation needs the service. The
latter is a failed runtime.

Client-only use must remain possible. Starting or signaling a workflow from an
HTTP handler should not require an ordinary queue worker in that process.
Worker applications select generated queue resources. A selected Temporal
resource polls the workflow and activity task queues declared by its driver
configuration as one lifecycle-owned unit.

## Observability

The integration should translate only events whose semantics are stable across
engines:

- workflow accepted
- workflow completed or failed
- activity started, succeeded, retried, or failed
- worker started, stopped, or unable to poll

Temporal SDK metrics remain the source for poller, workflow-task, activity-task,
and replay behavior. Queue observer events remain the source for the portable
job and workflow vocabulary. The bridge must prevent one Temporal event from
appearing as duplicate queue facts during replay.

Application observers must never execute directly from replayed workflow code.
Portable events require stable event IDs and an emission path with a documented
delivery guarantee, such as activity-side emission with durable deduplication
or an external history consumer. If that path cannot preserve an existing
event kind, the driver reports that event as unsupported rather than emitting a
plausible duplicate.

Lighthouse should show:

- selected queue driver and native workflow capability
- namespace and task queues, with credentials redacted
- registered portable and native workflow types
- registered activity types
- client and poller readiness
- a link to Temporal Web in local development

Execution history remains in Temporal Web.

## Security And Enterprise Controls

Temporal persists workflow inputs, activity inputs, results, signal payloads,
update payloads, and failure details in workflow history. Generated guidance
must treat those values as durable records, not transient function arguments.

The integration must support:

- TLS and API-key authentication without logging credentials
- namespace isolation
- explicit data-converter and payload-codec configuration
- encryption for sensitive payloads when required by deployment policy
- redaction in GoForj inspection and diagnostics
- dependency and vulnerability scanning for the optional module
- documented retention and deletion ownership

Payload encryption does not automatically protect every metadata surface.
Workflow and activity type names, task queue names, workflow IDs, search
attributes, memo metadata, logs, metrics, and some failure text can remain
visible outside encoded payloads. Generated identifiers and examples must not
embed personal data or secrets in those fields.

Task queues provide routing, not tenant authorization. A worker with namespace
access may be able to poll task queues whose names it knows, subject to the
deployment's authorization model. Enterprise isolation must use appropriately
configured namespaces, credentials, network policy, and service-side
authorization rather than relying on `QUEUE_NAME` or `OnQueue` values.

Temporal service acceptance does not make an execution payload trusted. Any
principal with sufficient namespace write access may be able to start a
framework-owned workflow type directly instead of entering through
`Queue.Dispatch`. Before a framework workflow schedules an activity, it must
validate envelope version, encoded size, queue-owned identity shape, registered
job type, option bounds, and allowed task queue. The activity repeats the
security-relevant checks before invoking application code. Unknown versions,
job types, or task queues fail permanently and must not enter an unlimited
retry loop.

The generic queue activity is a confused-deputy boundary because it can invoke
any handler registered in that worker. Deployments with different caller trust
levels or materially different job privileges require separate Temporal
namespaces and credentials, or a deployment-owned authorization layer that is
verified before handler execution. Task queue naming alone is insufficient.
GoForj does not invent a payload signature scheme because durable key rotation,
replay, and recovery would require an independently reviewed protocol.

Temporal Cloud API keys and self-hosted authentication are different
deployment contracts. The driver must document which credential forms each
mode supports, how server identity is verified, and whether credential or
certificate rotation requires rebuilding the client or restarting workers.
Production configuration must not silently fall back to plaintext when TLS
material is missing or invalid.

Authentication must fail closed. Configuring an API key, authorization token
supplier, client certificate, or server name without the transport and trust
material required by that mode is a construction error. Dispatch-only and
worker deployments must document their minimum service permissions separately
so operators can issue different credentials even when both deployments use
the same generated binary. Static secret values are copied into client state
and require a documented rotation action; dynamic token suppliers must bound
refresh latency and failures without logging tokens.

Self-hosted Temporal does not become authorized merely because mTLS encrypts
and authenticates a connection. The deployment must configure and test its
claim mapper and authorizer, or explicitly record network-level trust as an
accepted risk. The default permissive authorizer is not an enterprise control.
This distinction follows Temporal's
[self-hosted security guidance](https://docs.temporal.io/self-hosted-guide/security).

Error adapters must prevent secrets from being copied into Temporal failure
messages. Inspection must bound and redact payload previews. Metrics must not
use workflow IDs, payload values, or other unbounded identifiers as labels.

Enterprise deployment guidance must identify:

- the trust boundary between application workers and the Temporal service
- namespace authentication and authorization ownership
- certificate and API-key rotation behavior
- network paths required by clients, workers, and any payload codec service
- retention, archival, backup, legal hold, and deletion responsibility
- audit evidence for workflow starts, signals, updates, cancellation, and
  termination
- dependency inventory and supported Temporal server and SDK versions

A payload codec service that lets Temporal Web display encrypted values is a
separate privileged decryption surface. GoForj must not enable or publish one
without explicit authentication, authorization, network, and audit controls.

Payload codecs and custom data converters execute on application-controlled
workers and clients. Their version, key access, failure behavior, and backward
decoding compatibility are part of the durable protocol. Encryption keys must
remain available for retained histories and backups for as long as those
histories must be readable, with revocation and destruction governed by the
organization's retention policy.

Generated examples must not place credentials, tokens, payment details, or
unbounded application objects into workflow history. Search attributes and
memo fields must be treated as metadata surfaces, not secret storage.

Queue observers and application logs are operational telemetry, not an
authoritative audit trail. Enterprise evidence for workflow starts, signals,
updates, cancellation, termination, namespace administration, and credential
use must come from the managed service or secured self-hosted service boundary.
The deployment owner must document availability, retention, integrity, access,
and export of those records. GoForj documentation must not claim an audit
control merely because it emits queue events.

## Testing Strategy

### Queue repository

- Run existing chain and batch contracts against the built-in engine.
- Define a compatibility subset and run it against Temporal.
- Assert unsupported options fail before execution starts.
- Prove retry, timeout, backoff, delay, task-queue, and error translation.
- Prove the configured activity execution timeout is always nonzero, activity
  retry defaults are fully overridden, and heartbeat behavior matches every
  cancellation claim.
- Prove no application activity is repeated by workflow replay.
- Prove worker restart resumes an execution.
- Prove state queries for running and retained terminal executions.
- Prove shutdown ordering and bounded activity drain.
- Inject a fatal poller error after successful startup and prove `Queue.Run`
  returns it, GoForj cancels sibling runtimes, and shutdown remains bounded.
- Race concurrent startup failure, caller cancellation, fatal worker failure,
  and repeated shutdown; prove the SDK worker is stopped at most once.
- Run workflow determinism checks and history replay tests.
- Prove declared payload, chain-length, and batch-fan-out limits against a real
  server and fail larger inputs before workflow acceptance.
- Start heterogeneous worker registries against one task queue and prove the
  deployment check detects incompatible fingerprints. Prove reserved-name and
  signature collisions fail before polling starts.
- Replay histories produced by the previous supported application version
  against the proposed worker version.
- Replay committed framework workflow fixtures from every supported prior
  Temporal driver version, including fixtures encoded by supported data
  converter and codec versions.
- Prove unknown activity task queues fail before workflow acceptance.
- Submit malformed, oversized, unknown-version, unknown-job, and unauthorized
  task-queue inputs directly to framework workflow and activity types; prove no
  application handler runs and the failure is terminal.
- Prove credentials, certificate data, payloads, and failure details are absent
  from logs, metrics labels, inspection output, and panic recovery output.
- Exercise the real Temporal server for lifecycle and recovery boundaries that
  the SDK test environment cannot prove.
- Run the repository's module inventory and release-script contracts with the
  Temporal module included. Verify its planned tag independently.
- Validate the root, Temporal driver, integration, and examples modules
  independently, then validate a published-version consumer with `GOWORK=off`
  and no local replacement.

### GoForj repository

- Preserve byte-for-byte default renders when Temporal is not selected.
- Verify enabled and disabled template ownership.
- Compile the largest generated composition with queue jobs, portable
  workflows, native workflows, schedules, metrics, and multiple Apps.
- Verify rerender preserves application-owned workflow registration.
- Verify disabling Temporal cannot silently strand user-authored workflow code.
- Verify local service planning activates Temporal only for a consumer.
- Verify the Temporal entry in `ResourceQueue` round-trips through project
  configuration, named and App-prefixed environment reconciliation, service
  planning, project description, and compile-time driver manifests.
- Verify the jobs runtime performs Temporal shutdown before returning and does
  not depend on later application lifecycle hooks.
- Verify a later queue startup failure shuts down every earlier queue that
  started successfully, in reverse start order and within the shared budget.
- Verify existing `queue:work` queue-resource filters select Temporal instances
  without introducing a workflow-specific command mode.
- Verify generated task queue defaults remain distinct across project, App, and
  named queue scopes, and inspection exposes a redacted registration
  fingerprint without claiming fleet-wide verification.
- Verify secrets are blanked from examples and diagnostics.

## Phase 0 Decision Spikes

No public configuration or generator should land before these spikes complete:

1. Split shared handler execution from workflow coordination without changing
   public queue behavior. Prove `Register`, ordinary `Dispatch`, worker start,
   worker shutdown, and readiness no longer depend on coordinator ownership.
   Preserve typed provenance for store, clock, middleware, observer, and worker
   options without changing the public `queue.Option` API.
2. Add a private native-workflow driver capability and prove every existing
   driver still selects the built-in coordinator. Prove the optional-driver
   bridge preserves that capability instead of erasing it during adaptation.
3. Prove the optional module can construct a normal `*queue.Queue` and expose
   its native coordinator without exporting Temporal types from the root
   module or introducing an import cycle.
4. Prove root and named Temporal queue resources receive unambiguous clients,
   registrations, and worker ownership.
5. Run one ordinary job, one chain, and one batch through a Temporal driver in
   the SDK test environment.
6. Adapt one registered queue handler into a Temporal activity without changing
   the handler's public signature.
7. Prove exact retry, backoff, timeout, delay, and error classification.
   Prove workflow-level retries cannot multiply the activity attempt budget.
8. Prove `FindChain` and `FindBatch` behavior during execution, after
   completion, after worker restart, and while no compatible worker is polling.
   Verify every returned field and distinguish not-found from infrastructure
   and authorization failures.
9. Prove worker start, partial-start rollback, poll, drain, stop, and
   client-close ordering under the real GoForj runtime host. Inject a fatal
   error after successful startup and prove it reaches `RuntimeHost`.
10. Record every unsupported existing queue and workflow option.
11. Measure dependency and generated-binary impact when the driver is compiled
   and omitted.
12. Prove default Redis plus named Temporal composition through one generated
    Jobs runtime.
13. Establish real-server payload and history limits for portable chains and
    batches.
14. Prove one forward deployment and rollback using captured workflow
    histories and the selected worker-versioning policy.
15. Prove batch failure interleavings for queued, started, completed, failed,
    cancellation-resistant, and cancellation-aware activities with and without
    `AllowFailures`.
16. Prove every portable observer event has a stable identity and cannot be
    duplicated solely by workflow replay.
17. Exercise every exported `Queue` method, constructor option, fluent builder
    option, capability helper, and fake assertion against the documented
    Temporal support decision.
18. Build the worker-set lifecycle adapter against the selected SDK. Prove
    synchronous startup reporting, asynchronous fatal-error reporting,
    observable terminal completion, partial-start rollback, and repeated queue
    shutdown without racing an SDK-owned stop.
19. Run module inventory, release planning, and a no-replacement downstream
    resolution for the proposed queue and Temporal driver versions.
20. Threat-model direct invocation of every framework-owned workflow and
    activity type by a namespace writer. Prove validation happens before
    application code and document the namespace isolation requirement for
    different privilege domains.
21. Prove registration homogeneity for every shared task queue, reserved-name
    collision handling, generated task-queue names, and worker-versioning
    isolation during a mixed-version rollout.

## Release Gates

The Temporal option remains hidden and unsupported until every major boundary
has one outcome recorded in this design:

| Boundary | Required outcome |
| --- | --- |
| Existing driver isolation | Every existing driver contract passes unchanged when the Temporal module is available but not selected |
| Public surface | Every existing method and option is supported with tested semantics or returns an established unsupported-capability error at the earliest possible boundary |
| Native driver ownership | Root and named Temporal queues construct one unambiguous client, registry, worker set, and native coordinator without root-module Temporal types |
| Temporal ordinary dispatch | `Queue.Dispatch` starts one framework-owned workflow and reports acceptance consistently |
| Portable handler execution | The same registered handler and middleware execute through an existing driver and a Temporal activity |
| Callback durability | Named continuations land, or callback-bearing portable workflows fail before acceptance |
| State reads | `FindChain` and `FindBatch` meet their contract, or portable Temporal coordination does not ship |
| Uniqueness | Exact `UniqueFor` parity is proven, or the option fails before acceptance |
| Retry and timeout policy | Every zero value and explicit value has a tested translation |
| Retry ownership | Activity policy is the only automatic retry layer for framework workflows and never exceeds queue's attempt budget |
| Batch cancellation | Failure interleavings preserve established handler, counter, callback, and event behavior |
| Task routing | Every accepted task queue has a known local or declared external poller |
| Registry homogeneity | Shared task queues have compatible type and handler registrations, with reserved-name collisions rejected before polling |
| History limits | Payload and fan-out limits are measured and enforced before acceptance |
| Evolution | Previous-version histories replay on forward deploy and rollback |
| Driver protocol | Persisted framework envelopes are versioned and every supported prior driver history replays under the proposed release |
| Lifecycle | Partial startup, cancellation, timeout, and repeated shutdown converge safely |
| Fatal worker failure | A post-start poller failure reaches `Queue.Run` and the GoForj runtime host without polling or log inspection |
| Stop idempotency | Concurrent lifecycle paths call the underlying worker stop and client close at most once and return one stable result |
| Worker selection | Existing `queue:work` filters select whole queue resources, and a Temporal resource owns all pollers required by its declared task queues |
| Observer delivery | Portable events have stable identities and workflow replay cannot create duplicates |
| Security | Authentication modes, server identity, authorization boundaries, encryption, key rotation, redaction, retention, and audit ownership are documented and tested |
| Untrusted history input | Direct malformed or unauthorized invocation of framework workflow and activity types cannot reach application handlers or retry indefinitely |
| Audit evidence | Service-side audit evidence ownership and retention are documented without treating application observer events as an audit control |
| Disabled output | Renders that omit the Temporal driver contain no Temporal dependency or configuration |
| Toolchain compatibility | The selected Temporal SDK supports the established minimum Go version, or an unavoidable increase has an explicit migration |
| Module publication | The driver has its own planned tag, release-time sibling pin, proxy-resolvable module, and `GOWORK=off` downstream proof |

An unresolved row blocks the affected portable feature. It does not justify a
silent semantic downgrade, a second source of truth, or a new top-level
component.

## Implementation Sequence

1. Complete Phase 0 without changing generated applications.
2. Add the internal queue responsibility split, native-workflow driver
   capability, and compatibility contracts.
3. Add the optional `driver/temporalqueue` module with ordinary job dispatch,
   handler execution, worker lifecycle, and readiness only.
4. Release ordinary Temporal queue support only after its own gates pass. Do
   not wait for portable chain and batch support if ordinary jobs are sound.
5. Add portable chain and batch coordination behind the private native
   capability only after state, callback, cancellation, event, and replay gates
   pass.
6. Release and consume those queue modules using their established multi-module
   tag convention.
7. Add GoForj environment and compile-time dependency projection under
   Background Jobs.
8. Connect the existing Temporal developer service to that consumer.
9. Add generated worker registration and runtime hosting.
10. Add native workflow and activity generators with determinism tests as a
    separate optional milestone after the driver lifecycle is proven.
11. Add inspection, metrics, security guidance, and transition guards.
12. Enable each capability in the interactive project flow only after its
    specific release gates and the largest generated composition pass.

## Decision

Temporal belongs in the queue driver catalog and must not implement
`WorkflowStore`.

The existing chain and batch primitives provide a useful portable subset. A
Temporal driver can implement that subset through an optional native-workflow
capability after callback durability, state queries, policy translation, and
lifecycle behavior are proven. Native Temporal workflows remain available for
capabilities beyond that subset.

This design preserves one owner for workflow concepts, one authoritative state
system per execution, and one honest boundary between portable queue workflows
and Temporal-specific orchestration.
