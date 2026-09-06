# Queue Temporal Workflow Driver Design

## Status

- Proposed architecture, not scheduled for a release.
- This design replaces the assumption that Temporal requires a separate GoForj
  component or sibling workflow library.
- Phase 0 must prove state projection, lifecycle, and compatibility before the
  framework exposes configuration or generators.

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
| `Chain(A, B, C)` | Execute activities sequentially | Clean |
| `Batch(A, B, C)` | Schedule activity futures in parallel and collect results | Clean |
| Registered job handler | Generic or named Temporal activity | Clean |
| Serialized job payload | Activity input | Clean |
| Chain or batch ID | Temporal workflow ID | Clean |
| `Retry(n)` | Activity maximum attempts set to `n + 1` | Requires explicit translation |
| `Backoff(d)` | Initial interval `d` with coefficient `1` | Requires explicit translation |
| `Timeout(d)` | Activity start-to-close timeout | Requires an explicit nonzero default |
| `Delay(d)` | Durable timer before activity scheduling | Clean |
| `OnQueue(name)` | Activity task queue | Mostly clean |
| `AllowFailures()` | Collect member failures without early workflow failure | Clean |
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

Client cancellation or a network timeout can leave workflow-start acceptance
ambiguous. The driver must allocate the workflow ID before the request, return
it with any ambiguous error according to the established queue dispatch
contract, and safely resolve or retry the same start identity. A later
already-started response is evidence to reconcile, not automatically a new
failure or permission to generate another workflow ID.

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
| `WithObserver` | Supported through replay-safe event translation |
| `WithMiddleware` | Supported around the shared activity job executor |
| `WithStore` | Rejected because Temporal is the workflow state authority |
| `WithClock` | Rejected for production execution; SDK workflow time and test-environment time remain authoritative |
| `WithWorkers` | Must not guess how one number divides workflow and activity poller concurrency |
| `Ready` | Supported with distinct client and worker states |
| queue admin operations | Unsupported unless each operation receives a precise Temporal contract |
| native stats | Unsupported unless values map to the established queue snapshot without fabrication |
| pause and resume | Unsupported until task acceptance and in-flight behavior are defined |

Temporal needs separate workflow-task and activity-task concurrency settings.
Until queue has a domain-neutral worker policy that can express both, those
settings belong in `temporalqueue.Config`. `WithWorkers` must either receive a
documented activity-only meaning or fail construction. It must not silently
control one poller while leaving the other at an SDK default.

Every unsupported constructor option must fail during construction. Every
unsupported job or workflow option must fail before the service accepts an
execution. Capability helpers must report the same result as the operation.

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

If queries cannot satisfy the established availability contract, a derived
read model may be added. It must be explicitly non-authoritative, rebuildable
from Temporal history, and never participate in workflow transition ownership.
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

The module owns:

- Temporal client configuration and dialing
- workflow and activity registration
- worker lifecycle
- chain and batch compatibility workflows
- policy translation
- Temporal-specific readiness and inspection
- tests against the Temporal SDK test environment and a real development
  server

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
runtime. `queue:work` remains the worker command. It must distinguish physical
queue selection from workflow polling explicitly, either through separate
flags or a small worker-mode option. An empty queue list must not ambiguously
mean both "all physical queues" and "Temporal only", and a physical queue
filter must not silently change Temporal polling.

## Runtime And Lifecycle

Each queue runtime owns its selected driver's workers. A Temporal runtime owns
its workflow and activity pollers plus its native workflow coordinator. Other
drivers own their physical workers and use queue's built-in coordinator. A
client-only process may construct and dispatch through either path without
starting a local worker.

Shutdown of the Temporal driver must stop new polling before waiting for
admitted work and close the client only after its worker has stopped. When the
Jobs runtime starts several queue instances, a failure to start any instance
must clean up those already started before returning.

Phase 0 must test this ordering against the actual GoForj runtime host. It must
not assume lifecycle hooks that the runtime host does not call.

The current runtime host cancels sibling runtime contexts and waits for every
`Run` method to return. Application lifecycle shutdown hooks execute only after
the command returns. Temporal worker shutdown must therefore occur inside the
jobs runtime after its run context is canceled. Deferring it to an application
shutdown hook would make the runtime host wait for a worker that is itself
waiting for a later hook.

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
Worker applications may independently poll ordinary queue names, Temporal
workflow task queues, Temporal activity task queues, or an intentional
combination.

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

Generated examples must not place credentials, tokens, payment details, or
unbounded application objects into workflow history. Search attributes and
memo fields must be treated as metadata surfaces, not secret storage.

## Testing Strategy

### Queue repository

- Run existing chain and batch contracts against the built-in engine.
- Define a compatibility subset and run it against Temporal.
- Assert unsupported options fail before execution starts.
- Prove retry, timeout, backoff, delay, task-queue, and error translation.
- Prove no application activity is repeated by workflow replay.
- Prove worker restart resumes an execution.
- Prove state queries for running and retained terminal executions.
- Prove shutdown ordering and bounded activity drain.
- Run workflow determinism checks and history replay tests.
- Prove declared payload, chain-length, and batch-fan-out limits against a real
  server and fail larger inputs before workflow acceptance.
- Replay histories produced by the previous supported application version
  against the proposed worker version.
- Prove unknown activity task queues fail before workflow acceptance.
- Exercise the real Temporal server for lifecycle and recovery boundaries that
  the SDK test environment cannot prove.

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
- Verify secrets are blanked from examples and diagnostics.

## Phase 0 Decision Spikes

No public configuration or generator should land before these spikes complete:

1. Split shared handler execution from workflow coordination without changing
   public queue behavior. Prove `Register`, ordinary `Dispatch`, worker start,
   worker shutdown, and readiness no longer depend on coordinator ownership.
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
8. Prove `FindChain` and `FindBatch` behavior during execution, after
   completion, and after worker restart.
9. Prove worker start, partial-start rollback, poll, drain, stop, and
   client-close ordering under the real GoForj runtime host.
10. Record every unsupported existing queue and workflow option.
11. Measure dependency and generated-binary impact when the driver is compiled
   and omitted.
12. Prove default Redis plus named Temporal composition through one generated
    Jobs runtime.
13. Establish real-server payload and history limits for portable chains and
    batches.
14. Prove one forward deployment and rollback using captured workflow
    histories and the selected worker-versioning policy.

## Release Gates

The Temporal option remains hidden and unsupported until every major boundary
has one outcome recorded in this design:

| Boundary | Required outcome |
| --- | --- |
| Existing driver isolation | Every existing driver contract passes unchanged when the Temporal module is available but not selected |
| Native driver ownership | Root and named Temporal queues construct one unambiguous client, registry, worker set, and native coordinator without root-module Temporal types |
| Temporal ordinary dispatch | `Queue.Dispatch` starts one framework-owned workflow and reports acceptance consistently |
| Portable handler execution | The same registered handler and middleware execute through an existing driver and a Temporal activity |
| Callback durability | Named continuations land, or callback-bearing portable workflows fail before acceptance |
| State reads | `FindChain` and `FindBatch` meet their contract, or portable Temporal coordination does not ship |
| Uniqueness | Exact `UniqueFor` parity is proven, or the option fails before acceptance |
| Retry and timeout policy | Every zero value and explicit value has a tested translation |
| Task routing | Every accepted task queue has a known local or declared external poller |
| History limits | Payload and fan-out limits are measured and enforced before acceptance |
| Evolution | Previous-version histories replay on forward deploy and rollback |
| Lifecycle | Partial startup, cancellation, timeout, and repeated shutdown converge safely |
| Security | Authentication, encryption boundaries, redaction, retention, and audit ownership are documented and tested |
| Disabled output | Renders that omit the Temporal driver contain no Temporal dependency or configuration |
| Toolchain compatibility | The selected Temporal SDK supports the established minimum Go version, or an unavoidable increase has an explicit migration |

An unresolved row blocks the affected portable feature. It does not justify a
silent semantic downgrade, a second source of truth, or a new top-level
component.

## Implementation Sequence

1. Complete Phase 0 without changing generated applications.
2. Add the internal queue responsibility split, native-workflow driver
   capability, and compatibility contracts.
3. Add the optional `driver/temporalqueue` module in the queue repository.
4. Release and consume those queue modules using their established multi-module
   tag convention.
5. Add GoForj environment and compile-time dependency projection under
   Background Jobs.
6. Connect the existing Temporal developer service to that consumer.
7. Add generated worker registration and runtime hosting.
8. Add native workflow and activity generators with determinism tests.
9. Add inspection, metrics, security guidance, and transition guards.
10. Enable the option in the interactive project flow only after the largest
    generated composition passes.

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
