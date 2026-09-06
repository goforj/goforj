# Queue Temporal Workflow Engine Design

## Status

- Proposed architecture, not scheduled for a release.
- This design replaces the assumption that Temporal requires a separate GoForj
  component or sibling workflow library.
- Phase 0 must prove state projection, lifecycle, and compatibility before the
  framework exposes configuration or generators.

## Summary

GoForj should integrate Temporal through the workflow side of
`github.com/goforj/queue`.

The queue library already owns two related but distinct responsibilities:

1. Physical delivery of ordinary jobs through sync, worker pool, SQL, Redis,
   NATS, SQS, and RabbitMQ runtimes.
2. Workflow orchestration through durable chains, batches, state stores,
   callbacks, recovery, testing helpers, and lifecycle events.

Temporal belongs in the second responsibility. It is not another physical
queue driver, and it must not implement `queue.WorkflowStore`. Temporal owns
workflow history, replay, and state transitions itself.

The integration has two levels:

- Existing `queue.Chain` and `queue.Batch` operations may run on a Temporal
  workflow engine when their semantics can be preserved.
- Applications may register native Temporal workflows for capabilities that
  cannot be represented by static chains and batches, including signals,
  updates, durable timers, child workflows, queries, and continue-as-new.

This keeps workflow ownership in `goforj/queue` without hiding Temporal's
programming model behind an inaccurate portable abstraction.

## Existing Boundary

The high-level queue already composes a physical runtime and a workflow engine:

```text
queue.Queue
├── queueRuntime
│   ├── ordinary job dispatch
│   ├── handler delivery
│   └── physical worker lifecycle
└── workflow.Engine
    ├── chain orchestration
    ├── batch orchestration
    ├── workflow state
    └── workflow lifecycle events
```

This is the integration seam. Temporal replaces or supplements the workflow
engine. It does not sit beneath the current engine as a transport or store.

Layering the existing queue workflow engine over a Temporal queue driver would
create two orchestration authorities. Queue would persist chain and batch
transitions while Temporal persisted a second history for delivery wrappers.
That would retain the operational cost of Temporal without gaining its native
workflow model.

## Goals

1. Keep all queue and workflow concepts in the existing queue ecosystem.
2. Preserve ordinary queue driver behavior when Temporal workflows are enabled.
3. Preserve established chain and batch behavior where Temporal can implement
   it truthfully.
4. Let advanced workflows use the Temporal Go SDK directly.
5. Keep Temporal dependencies out of projects that do not select the engine.
6. Reuse GoForj runtime hosting, dependency injection, generation, development
   services, observability, and shutdown policies.
7. Make determinism, payload persistence, retry translation, and unsupported
   capabilities explicit.

## Non-Goals

1. Making Temporal a `queue.Driver`.
2. Implementing `queue.WorkflowStore` over Temporal.
3. Replacing ordinary background jobs with one-activity Temporal workflows.
4. Reproducing Temporal's full API as portable queue interfaces.
5. Reimplementing Temporal Web in Lighthouse.
6. Managing production Temporal clusters or Temporal Cloud accounts.
7. Claiming transparent parity before the compatibility contracts pass.

## Two Independent Configuration Axes

An application can use Redis for ordinary background jobs and Temporal for
durable workflows at the same time. Queue transport and workflow execution
must therefore be configured independently.

```text
QUEUE_DRIVER=redis
QUEUE_WORKFLOW_ENGINE=temporal
```

The default remains the current built-in workflow engine:

```text
QUEUE_WORKFLOW_ENGINE=builtin
```

Selecting Temporal adds only workflow-specific configuration:

```text
QUEUE_WORKFLOW_SUPPORTED_ENGINES=builtin,temporal
QUEUE_WORKFLOW_TEMPORAL_ADDRESS=127.0.0.1:7233
QUEUE_WORKFLOW_TEMPORAL_NAMESPACE=default
QUEUE_WORKFLOW_TEMPORAL_TASK_QUEUE=my-app-workflows
QUEUE_WORKFLOW_TEMPORAL_TLS_ENABLED=false
QUEUE_WORKFLOW_TEMPORAL_API_KEY=
QUEUE_WORKFLOW_TEMPORAL_CERT_PATH=
QUEUE_WORKFLOW_TEMPORAL_KEY_PATH=
QUEUE_WORKFLOW_TEMPORAL_SHUTDOWN_TIMEOUT=60s
```

These names are provisional until Phase 0 confirms the queue library's public
configuration seam. The important contract is that Temporal is selected as a
workflow engine, not as the application's ordinary queue driver.

## Compatibility Map

| Queue workflow behavior | Temporal implementation | Assessment |
| --- | --- | --- |
| `Chain(A, B, C)` | Execute activities sequentially | Clean |
| `Batch(A, B, C)` | Schedule activity futures in parallel and collect results | Clean |
| Registered job handler | Generic or named Temporal activity | Clean |
| Serialized job payload | Activity input | Clean |
| Chain or batch ID | Temporal workflow ID | Clean |
| `Retry(n)` | Activity maximum attempts set to `n + 1` | Clean with translation |
| `Backoff(d)` | Initial interval `d` with coefficient `1` | Clean with translation |
| `Timeout(d)` | Activity start-to-close timeout | Mostly clean |
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

The Temporal engine must reject an unsupported option instead of silently
changing its meaning.

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
  attempts includes the initial attempt.
- Queue backoff is a fixed duration where supported. Temporal defaults to an
  exponential coefficient, so parity requires a coefficient of `1`.
- Queue timeout is an execution-attempt boundary. The closest Temporal
  activity boundary is start-to-close timeout.
- Queue delay should use workflow time, never `time.Sleep` inside replayed
  workflow code.
- A job queue name can select an activity task queue. The workflow task queue
  remains separate configuration because workflow and activity pollers have
  different operational roles.

## Callback Migration

The existing fluent callbacks hold Go closures in process memory. The queue
library documents them as ephemeral, and its roadmap already calls for named
durable continuations.

Temporal cannot persist or replay those closures. The Temporal engine must not
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
type PaymentWorkflow struct{}

func NewPaymentWorkflow() *PaymentWorkflow {
	return &PaymentWorkflow{}
}

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

## Queue Repository Ownership

Reusable Temporal integration belongs in the existing queue repository. It
does not justify a new top-level `goforj/workflow` sibling.

The preferred packaging is an optional nested Go module in the queue
repository, similar to its optional backend modules. Its exact path is a Phase
0 decision because the integration is a workflow engine, not a queue driver.

The module owns:

- Temporal client configuration and dialing
- workflow and activity registration
- worker lifecycle
- chain and batch compatibility workflows
- policy translation
- Temporal-specific readiness and inspection
- tests against the Temporal SDK test environment and a real development
  server

The root queue module owns the smallest additive engine seam needed to compose
the integration. It should not export Temporal-specific types.

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

When Background Jobs is enabled with the built-in engine, output remains
unchanged from today's render.

When Background Jobs is enabled with the Temporal engine, GoForj additionally:

- includes the optional queue Temporal module
- activates the existing Temporal development-service profile
- renders Temporal client and worker construction
- registers portable compatibility workflows and application-owned native
  workflows
- hosts the Temporal worker alongside the ordinary queue worker
- exposes Temporal-specific readiness and inspection without duplicating the
  Temporal Web UI

## Runtime And Lifecycle

The queue runtime owns both its physical workers and selected workflow engine.
Starting queue workers starts both when Temporal is enabled. Shutdown stops new
polling, drains within the configured budget, and closes the client only after
worker shutdown completes.

Phase 0 must test this ordering against the actual GoForj runtime host. It must
not assume lifecycle hooks that the runtime host does not call.

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

- selected workflow engine
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
- Verify secrets are blanked from examples and diagnostics.

## Phase 0 Decision Spikes

No public configuration or generator should land before these spikes complete:

1. Add a private alternate-engine seam to queue and run one chain and one batch
   through the Temporal SDK test environment.
2. Adapt one registered queue handler into a Temporal activity without changing
   the handler's public signature.
3. Prove exact retry, backoff, timeout, delay, and error classification.
4. Prove `FindChain` and `FindBatch` behavior during execution, after
   completion, and after worker restart.
5. Prove worker start, poll, drain, stop, and client-close ordering under the
   real GoForj runtime host.
6. Decide the optional queue module path based on the code that remains after
   using the Temporal SDK directly.
7. Record every unsupported existing queue workflow option.
8. Measure dependency and generated-binary impact for enabled and disabled
   renders.

## Implementation Sequence

1. Complete Phase 0 without changing generated applications.
2. Add the minimal queue engine seam and compatibility contracts.
3. Add the optional Temporal module in the queue repository.
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

Temporal belongs in the queue workflow domain, but not in the physical queue
driver contract and not in `WorkflowStore`.

The existing chain and batch primitives provide a useful portable subset. A
Temporal workflow engine can implement that subset after callback durability,
state queries, policy translation, and lifecycle behavior are proven. Native
Temporal workflows remain available for capabilities beyond that subset.

This design preserves one owner for workflow concepts, one authoritative state
system per execution, and one honest boundary between portable queue workflows
and Temporal-specific orchestration.
