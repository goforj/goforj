# Queue Temporal Workflow Driver Design

## Status

- Target: a complete, ergonomic, flexible Temporal integration in queue, with
  ordinary jobs, portable chains/batches, durable named continuations,
  worker-independent state reads, and native SDK workflows in one supported
  release. The user clarified this release scope on 2026-11-08.
- Design is ready for full-scope implementation with the required proof stages
  first, following fresh independent review and re-review across four scopes.
- A/B/C are internal implementation stages. A jobs/native-only supported release
  does not satisfy this design. Earlier A/B review approvals cover only those
  stages and do not establish complete-scope approval.
- The complete addition is not implemented or release-qualified. State-journal,
  continuation atomicity, SDK lifecycle, and real-server integration evidence
  remain mandatory before declaring it complete.
- Keep one optional queue driver module, no parallel GoForj Workflows component
  or sibling workflow library. Preserve existing-driver behavior and APIs.

## Complete Release Contract

The first supported Temporal driver release must deliver all of the following:

| Surface | Required behavior |
| --- | --- |
| Ordinary jobs | Public `Queue.Dispatch`, existing handlers, payload bytes, middleware, correlation, explicit retry/timeout/delay policy, and lifecycle |
| Portable workflows | Public `Chain` and `Batch`, sequential/fan-out execution, `AllowFailures`, exact documented aggregate semantics and bounded recovery |
| Durable continuations | Named success, failure, progress, and completion jobs, stable invocation identities, immutable trigger snapshots, recovery, and observable exhaustion |
| State reads | `FindChain`, `FindBatch`, and continuation outcomes before polling, while running, after worker loss, and for retained terminal histories |
| Native Temporal | Real SDK client, registration, typed activities, signals/queries/updates, timers, children, continue-as-new, and supported versioning/options |
| Ergonomics | One public Queue facade, one continuation binding helper, explicit ownership, useful unsupported-capability errors, and runnable examples |
| Integration proof | Real Temporal server, separate producer/worker/read clients, crash recovery, replay/rollback, security, all relevant modules, and complete generated composition |

No release gate may be passed by dropping portable workflows, calling named
continuations optional, requiring a live worker for state reads, or presenting
SDK unit tests as real-server evidence. Unsupported process-local closures and
administrative operations without honest Temporal equivalents remain explicit
capability limitations; they do not excuse omitting the durable alternatives.

### Implementation Stages Versus Publication

A may land as an independently reviewable queue refactor. B may implement job
execution and native hosting behind development-only configuration. C completes
portable coordination, state, and durable continuations. Publish and advertise
the complete Temporal driver only after A, B, and C pass the combined acceptance
suite. If a required proof fails, repair the design/implementation; do not
silently narrow the release to B. Public APIs and durable protocol are frozen
only after the bounded state-journal and continuation prototypes succeed.

## Review Record

Fifteen serial reviews were applied to this design. Each pass reviewed the
result of the prior pass rather than the original proposal.

| Pass | Focus | Finding resolved |
| --- | --- | --- |
| 1 | Queue API and driver construction | The existing bridge erases optional capabilities and the current workflow engine owns ordinary queue operations; an internal responsibility split and construction descriptor are prerequisites |
| 2 | Identity, state, and error semantics | Queue IDs, Temporal workflow and run IDs, ambiguous acceptance, complete state reconstruction, and not-found translation now have explicit contracts |
| 3 | GoForj generation and runtime lifecycle | Existing queue-resource selection remains authoritative, workflow-specific worker modes were removed, and partial startup now requires rollback |
| 4 | Enterprise operation and upgrades | Framework workflow definitions are versioned durable protocol, task queues are not security boundaries, and credential, codec, retention, and replay ownership are explicit |
| 5 | Adversarial compatibility and simplification | Typed option provenance, retry-layer ownership, batch cancellation interleavings, observer replay safety, and independently releasable milestones are explicit gates |
| 6 | Complete public API surface | Every exported queue operation, option, helper, and fake surface has an explicit Temporal support decision |
| 7 | Worker topology and failure propagation | Workflow and activity pollers have explicit roles, fatal SDK worker failures reach the runtime host, and repeated stop races are blocked on a lifecycle proof |
| 8 | Modules, releases, and SDK compatibility | The optional module has its own tag and downstream resolution proof, and SDK selection cannot raise the minimum Go version silently |
| 9 | Security boundaries | Direct workflow invocation, generic activity privilege, transport authentication, authorization, codecs, secrets, retention, and audit ownership have fail-closed requirements |
| 10 | Registration homogeneity | Reserved durable names, local collision checks, deployment fingerprints, and mixed-version task-queue isolation are explicit |
| 11 | Historical staged-release authority | Development-only ordinary-job execution declares portable workflows unsupported rather than falling back; the complete-release contract above supersedes the former partial-release plan |
| 12 | Concurrency and routing | The first release has one activity task queue per resource, and multiple queues remain blocked until total capacity and fairness are proven |
| 13 | Dispatch idempotency | Ambiguous starts reconcile the original history and immutable input, reject collisions, and never expose payload digests as visible metadata |
| 14 | Workflow ID semantics | Running conflict, closed-run reuse, and already-started return behavior are set independently and explicitly |
| 15 | Cumulative architecture audit | Public API, configuration, lifecycle, persistence, security, observability, generation, modules, releases, and operational failure modes contain no unresolved architecture-level contradiction |

The reviews found no reason to create a new top-level component or sibling
library. They did find prerequisites that block an immediate implementation.
Those prerequisites are represented below as Phase 0 decisions and release
gates. An unproven gate disables the affected capability; it does not permit a
silent behavior change. The historical cumulative pass reported no further major design change. The
subsequent comprehensive audit found additional design gaps, including an
outdated transport premise, retention-bounded deduplication, and insufficiently
specified native SDK integration. Review counts do not establish correctness.

## Independent Review Passes

Four separate reviewers independently inspected the design and relevant source:
API/native integration, Temporal semantics, runtime/lifecycle, and security.
The first pass found concrete gaps; it did not approve implementation closure.
After those revisions, all four reviewers completed a second pass and reported
no remaining architecture blocker to starting A, then scoped B implementation.
The parent review reconciled their findings and retained the implementation
acceptance gates. Recorded on 2026-11-08.

| Independent review scope | First-pass outcome | Second-pass outcome |
| --- | --- | --- |
| Queue API and native SDK integration | Session/tuner capacity and fluent configuration conflicts required resolution | Ready for A, then B; public API and capability boundaries explicit |
| Temporal semantics | Transport stability, total-attempt recovery, and zero-value policies required decisions | Ready for A/B; no SQL redelivery parity or experimental transport claim |
| Runtime and lifecycle | Client-only cleanup and native activity routing required correction | Ready for A/B; ownership and routing closed at design level |
| Security and operations | Pre-entry decoding, SDK logs, and opaque borrowed configuration needed contracts | Ready for A/B; bounded conversion, logging, and authoritative binding contracts explicit |

The reviewed corrections are:

| Finding | Design resolution |
| --- | --- |
| Standalone stability versus open transport comparison | Stable wrapper selected; experimental standalone APIs deferred |
| Worker-loss consumes Temporal retry budget | Explicit total-attempt semantics, failure boundaries, and kill/restart tests |
| Unset timeout/backoff and replay configuration drift | Required activity timeout, fixed/rounded intervals, resolved immutable start policy |
| Native SDK session/tuner capacity | Rejected in initial support matrix; deferred fluent errors preserve API signatures |
| Native default activity routing/local registration | Combined role default, persisted explicit split binding, correct role registries |
| Client-only/unselected resources and failed construction | Resource cleanup for every queue plus reverse construction rollback |
| Opaque borrowed client configuration | Module-produced immutable client binding and context-aware reconciliation |
| Decoding before envelope validation | Bounded conversion/propagation contract and malformed-input tests |
| SDK diagnostics bypass failure encryption | SDK logger policy and real-log sentinel tests |
| Shutdown completion/deadlines | Own completion observation and retryable caller timeouts |
| Production replay artifacts | Synthetic committed fixtures and restricted production capture process |

Runtime experiments remain acceptance gates. Review approval means the
implementation contract is sufficiently explicit to begin those stages, not
that SDK/server behavior or release qualification has already been proved.
These approvals were for the former narrower implementation stages. The user
subsequently required the complete addition in one supported release. The
complete-release contract, selected state journal, and mandatory continuations
below supersede that partial-release plan and require a full-scope review.
A remains an independently reviewable refactor; B remains an internal stage.
No earlier review count or A/B approval is complete-release acceptance.

## Complete-Scope Review Follow-Up

After the user clarified the release target, the same four independent review
scopes assessed the expanded design and its revisions. Their earlier A/B-only
approval was not carried forward as complete-release approval. The follow-up
resolved the previously optional state/continuation scope, chose the journal
protocol and additive binding API, and specified actual batch/drain semantics.

The final review findings were incorporated as explicit contracts: scheduled
versus physical-running continuation status; interruption versus exhaustion;
trusted parent/intent association and journal invariants; checked continuation
amplification/history budgets; and compatible SQL outbox schema/rollout rules.
The reviewers found the complete architecture coherent for implementation
prototypes, subject to those corrections and the required empirical proofs.

Implementation proceeds against the full target. Public API/protocol freeze
still requires successful journal and atomic-continuation prototypes. Release
requires the complete integration matrix. This record is design evidence only;
no real-server, outbox, replay or full-composition pass is claimed here.

## Fresh Complete-Design Review

On 2026-11-08, four fresh sub-agents independently reviewed the complete design
against queue, GoForj runtime sources and versioned SDK/server evidence. They
were told to ignore historical review counts and approvals. A second pass
reviewed the revised contract. This review covers the full release scope,
not only the earlier ordinary-job/native-hosting stages.

| Scope | First-pass findings | Reconciled outcome |
| --- | --- | --- |
| API and library integration | Missing SQL recovery-runner owner, unsafe pruning eligibility, circular publication gate | Runtime-hosted fenced runner, continuation-aware retention/prune and two-phase publication specified |
| Temporal protocol | Memo values not checked by SDK replay, conflicting serial scheduling/atomicity, undefined reset branching | Semantic replay parity, atomic pending intents plus head-only scheduling, explicit unsupported portable reset |
| Runtime and lifecycle | Jobs client could close during HTTP drain; SDK stop did not prove user-work quiescence | App-owned borrowed bindings, single stop arbiter, distinct stop/drain states and adapter admission/cleanup fencing |
| Security | Pre-entry propagator panic could retry indefinitely, retained intent authority could disappear, direct starts could add retry/cron | Approved rejection adapters, protected authority lifetime and actual execution-policy validation |

The second-pass reviews clear full-scope implementation with proof stages first.
The runtime reviewer required the explicit stop/admission/adapter-drain caveats
now incorporated above. No remaining architecture decision from these findings
is left to an implicit fallback or a narrowed release. Public API/protocol freeze
still requires the selected prototype proofs. No implementation, real-server
integration, shutdown, replay, SQL recovery or publication result is claimed by
this review record.

## Summary

GoForj should integrate Temporal as an optional driver in
`github.com/goforj/queue`.

The queue library already owns two related but distinct responsibilities:

1. Physical delivery of ordinary jobs through sync, worker pool, SQL, Redis,
   NATS, SQS, and RabbitMQ runtimes.
2. Workflow orchestration through durable chains, batches, state stores,
   callbacks, recovery, testing helpers, and lifecycle events.

Temporal can own both responsibilities when it is the selected driver.
The initial supported transport runs ordinary jobs as framework-owned,
single-activity workflows. Standalone activities remain a future transport until
the SDK marks the required APIs stable. Chains
and batches use a driver-native workflow capability so Temporal owns their
history, replay, and state transitions. Temporal must not implement
`queue.WorkflowStore`.

The integration has two levels:

- Existing `queue.Chain` and `queue.Batch` operations run through the
  Temporal driver's native coordinator with the explicit portable contract
  defined below; these are required for the complete release.
- Applications may register native Temporal workflows for capabilities that
  cannot be represented by static chains and batches, including signals,
  updates, durable timers, child workflows, queries, and continue-as-new.

Existing drivers continue to use queue's built-in workflow coordinator.
An incomplete development build declares portable workflows unsupported.
The supported release advertises native portable coordination only after the
complete implementation passes its gates. Queue reads the explicit mode during construction. This keeps one driver
selection and one orchestration authority per queue instance without hiding
Temporal's programming model behind an inaccurate portable abstraction.

## Comprehensive Audit

Reviewed against queue revision `2e01ce48b1d61ae62042044656d7659f3c16cf24`
and GoForj revision `2ce10b2ada1a814058dd19fd69c4b3588ca55cef`, plus the
versioned SDK source and official Temporal documentation linked below. These
revisions identify the source inspected, not released integration support.

### Architectural Verdict

Keeping the optional Temporal integration in queue is sound. Forcing Temporal
applications through only `Chain`, `Batch`, and `queue.Message` would lose key
functionality. The public portable API is a convenience subset, not the native
Temporal programming model. Native SDK workflows must be an independently
usable, documented path with the same resource configuration and lifecycle.
Native API design remains independent of the portable facade, while the
complete supported release requires both paths.

The root queue abstraction remains small. The optional module is both a queue
driver and a native SDK hosting integration. This is an intentional boundary,
not a claim that every Temporal feature is portable across queue drivers.

### Findings And Required Changes

| Finding | Consequence | Design change |
| --- | --- | --- |
| Native access was described but not specified enough to establish feature preservation | A client alone cannot configure registrations, worker versioning, tracing, or typed result-producing activities | Require SDK client options, worker options, full registration options, and native acceptance scenarios |
| Ordinary jobs were said to require a workflow wrapper | Temporal has standalone activities, but inspected SDK APIs remain experimental | Select the stable wrapper initially; reconsider standalone only with a stable supported API and migration review |
| Reject-duplicate was treated as permanent protection | Closed executions stop protecting an ID after retention or deletion | Bound reconciliation to retained executions and prohibit automatic restarts when prior acceptance cannot be established |
| Same-ID retry was implied through the current public API | Each ordinary, chain, and batch dispatch currently allocates fresh IDs | Separate internal RPC retry from a new public dispatch; leave cross-call idempotency to an explicit future contract or native SDK use |
| Staging was confused with release scope | Narrow A/B approval did not cover the requested complete driver | Use internal stages, but require portable workflows/state/continuations and native SDK support together before publication |
| Workflow validation could consult a live application registry | Changing process registrations can change replay decisions; dispatch-only producers may have no handlers | Keep replayed validation deterministic and distinguish a declared producer catalog from the worker execution registry |
| SDK customization and injected-client ownership were unspecified | An advanced user could need a second client or worker just to regain SDK functionality, or suffer double close | Preserve native options with explicit ownership and validate only actual lifecycle/routing conflicts |
| Observer uniqueness was underspecified | Stable IDs alone cannot guarantee exactly-once external publication around a crash | Keep telemetry best effort unless an independently proven delivery mechanism exists |
| One worker count was described too broadly | Local activities and Nexus do not necessarily consume the same remote activity slots | Define the limit as remote activity concurrency per process and resource, with separate native budgets |
| Cross-driver switching and native workflow upgrades were easy to conflate | Changing a driver does not migrate open histories or make native workflows portable | Require explicit draining, retained workers, and an operator-owned transition plan |

### Native Capability Preservation

| Temporal value | Portable queue path | Native path required from this module |
| --- | --- | --- |
| Durable history, retries, worker-loss recovery | Ordinary jobs and the proven chain/batch subset | SDK workflows and activities with their own policies |
| Typed results and result-dependent branching | Existing queue handlers return only errors; chains do not pipe results | Native activities returning typed results and workflow futures |
| Human approvals, entity workflows, signals, queries, validated updates | No portable equivalent | SDK message handlers and client APIs, including with-start operations |
| Durable waits, timers, selectors, compensation, child workflows | Static chains/batches cover only a subset | SDK workflow APIs, cancellation scopes, and parent-close policy |
| Long-lived execution and bounded history | Portable workflows remain bounded | Explicit continue-as-new with application-owned state and handler completion |
| Deployment safety | Driver-owned protocol replay tests | SDK registration versioning behavior and worker deployment configuration |
| Heartbeats, progress recovery, asynchronous completion | Ordinary queue handlers retain their established contract | SDK activity context, client completion APIs, and typed native activity registration |
| Visibility and operational intervention | Queue capabilities only when their semantics match | SDK visibility, search attributes, history, cancel, terminate, reset, and Temporal Web |
| Durable recurring starts | Existing GoForj scheduler remains independent | Native Temporal Schedule client with explicit schedule ownership |
| SDK integrations and cross-service orchestration | No invented portable equivalents | Supported SDK interceptors, propagators, converters, metrics, and Nexus registration |

Feature preservation means these capabilities are reachable through the selected
SDK without another framework-owned workflow facade. It does not mean every
feature is implemented or certified by this proposed driver. Each advertised
native capability needs a tested SDK/server version and an executable example.
Newer or experimental SDK features remain outside the tested support claim.

Temporal's [Go message-passing guide](https://docs.temporal.io/develop/go/workflows/message-passing)
provides the native interactive model. Its
[continue-as-new guide](https://docs.temporal.io/develop/go/workflows/continue-as-new)
and [child workflow model](https://docs.temporal.io/child-workflows) describe
execution boundaries the static queue API cannot express. This audit therefore
rejects any implementation that makes those features contingent on adding
portable methods to the root queue package.

### Initial Transport Decision

Use a single-activity workflow for ordinary jobs in the initial supported
Temporal driver. Evaluate SDK v1.45.0 against the selected server without
increasing queue's Go 1.24.4 floor. Its module declares Go 1.24.0. Exact server
image, SDK dependency closure, and lifecycle viability are Phase 0 evidence
outputs; they cannot be guessed from the developer-service template.

Standalone activities are a natural future mapping for ordinary queue jobs,
but the versioned SDK client interfaces mark `ExecuteActivity` and
`GetActivityHandle` experimental in both
[v1.45.0](https://github.com/temporalio/sdk-go/blob/v1.45.0/internal/client.go)
and [v1.49.0](https://github.com/temporalio/sdk-go/blob/v1.49.0/internal/client.go).
The [official standalone guide](https://docs.temporal.io/develop/go/activities/standalone-activities)
describes the capability, but a tutorial does not override SDK stability
annotations. The v1.49.0
[module manifest](https://github.com/temporalio/sdk-go/blob/v1.49.0/go.mod)
also requires Go 1.26.0. That is a consumer toolchain cost, not the reason the
initial stable portable transport excludes it.

This settles the initial durable protocol. Benchmark wrapper overhead during
Phase 0 and document its cost, but a faster benchmark cannot make an experimental
API stable. Reconsider standalone transport only after selecting stable SDK and
server APIs and reviewing identity, reconciliation, worker topology, security,
observability, and migration of retained executions together. Do not negotiate
transport automatically or silently change it in an existing driver release.
Native callers deliberately using experimental SDK features are outside the
integration's tested support claim.

### Internal Implementation Stages

| Stage | Required work | Publication rule |
| --- | --- | --- |
| A: construction seam | Existing-driver contracts, shared execution, typed options, explicit coordinator modes, compatibility-preserving continuation seam | May land independently as a queue refactor |
| B: Temporal execution | Wrapper jobs, native SDK access, ownership, routing, lifecycle, security and SDK feasibility tests | Development-only Temporal integration |
| C: complete portable workflows | Chains, batches, durable continuations, history-journal state reads, shared continuation-store capability and fake support | Required with A/B for the first supported Temporal release |
| D: framework integration | Named resources, Wire, runtime host, developer services, docs/examples, dependency projection and complete composition | Enabled only against the complete qualified queue driver |

These stages organize changes without splitting the requested product into
separate promises. Native SDK access is developed early to prevent portability
work from distorting the SDK model, while complete-release acceptance still
requires portable coordination. No optional convenience generator substitutes
for runnable native registration and public Queue examples.

### Audit Evidence And Limits

Queue `runtime.go` routes registration, ordinary dispatch, startup, and shutdown
through `workflow.Engine`. `internal/driverbridge/bridge.go` adapts backend
interfaces, and `runtime_hook.go` constructs the high-level queue afterward.
`internal/workflow/runtime.go`, `chain.go`, and `batch.go` allocate identities
inside each dispatch. `workflow_models.go` exposes error-only handlers and
static stored job policy, establishing the portable abstraction's limits.

The candidate SDK's [client contract](https://github.com/temporalio/sdk-go/blob/v1.45.0/internal/client.go)
contains native with-start, history/service, Schedule, converter, propagator,
and interceptor surfaces. Its [worker registry](https://github.com/temporalio/sdk-go/blob/v1.45.0/worker/worker.go)
and [worker options](https://github.com/temporalio/sdk-go/blob/v1.45.0/internal/worker.go)
provide the native registration and deployment seams to preserve. Source
presence is not proof that every feature is stable or supported by the chosen
server. Phase 0 must establish that support matrix.

No Temporal driver exists in this checkout. This audit does not claim a running
integration, benchmark result, replay proof, or production qualification.

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

`Queue.Dispatch` always uses the selected driver. Coordinator selection is an
explicit three-state internal value, not a boolean capability check:

| Mode | Meaning |
| --- | --- |
| built-in | Existing drivers use queue's built-in chain and batch coordinator |
| native | The driver provides a coordinator that owns workflow execution and state |
| unsupported | Ordinary jobs work, but chain, batch, state-read, and prune operations return an explicit unsupported error |

An unspecified mode is valid only for existing driver descriptors migrated to
the built-in default. A new driver must declare its mode. The ordinary-jobs
development stage declares unsupported. It must never fall through to the
built-in coordinator. Stage C changes the mode to native after its gates pass;
that mode is mandatory for the complete supported release.

The built-in and native coordinator paths use the shared registry and executor
for application jobs. In native mode, `Chain`, `Batch`, `FindChain`,
`FindBatch`, and workflow-specific lifecycle behavior delegate to the driver's
coordinator. In unsupported mode, their error-returning boundary reports the
unsupported capability before any execution is accepted.

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
the ordinary driver runtime, workflow mode, and optional domain-neutral
capabilities together.
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
| `OnQueue(name)` | Activity task queue | Initial release accepts only the instance default; multiple targets require shared-capacity proof |
| `AllowFailures()` | Collect member failures without early workflow failure | Must pass aggregate and interleaving contracts |
| Worker restart recovery | Workflow replay resumes orchestration; activity recovery requires remaining total attempt budget | Explicit Temporal driver behavior; not SQL same-attempt redelivery parity |
| `FindChain` and `FindBatch` | Versioned history-journal reduction without workers | Required; queries alone cannot pass acceptance |
| `Progress`, `Then`, `Catch`, and `Finally` closures | Process-local functions cannot be replayed | Not durable |
| `UniqueFor` on individual jobs | No equivalent activity-level uniqueness contract | Not clean |
| `WorkflowStore` mutation methods | Temporal owns history and transitions | Incompatible |
| `Prune(before)` | Temporal retention and administrative deletion | Incompatible |
| Queue middleware | Activity wrapper or Temporal interceptor | Must be proven |
| Fake queue workflow assertions | Portable definitions, metadata and continuation outcomes | Domain tests; SDK and real-server tests remain separate |

The Temporal driver must reject an unsupported option instead of silently
changing its meaning.

## Ordinary Jobs On The Temporal Driver

Under the compatibility baseline selected for evaluation above, `Queue.Dispatch`
starts a framework-owned job workflow that executes one registered job activity.
This is the selected stable integration policy for the candidate SDK/toolchain,
not a universal Temporal limitation.

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

Every framework-owned workflow start sets three options explicitly:

- workflow ID conflict policy is fail for an already-running execution
- workflow ID reuse policy is reject duplicate for a closed execution
- the client returns an already-started error instead of silently returning the
  existing run

The driver must reject duplicate starts while the original execution remains
open or retained, and must not bypass reconciliation by accepting an existing
run handle. This does not prevent administrative reset, which is a separate
operator action rather than a dispatch retry.
Application native workflows choose their own reuse and conflict policies
explicitly and do not inherit this framework default invisibly.

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

Reconciliation must verify that the existing execution has the expected
framework workflow type, protocol version, queue identity, and immutable input
before classifying an already-started response as prior acceptance. A mere
workflow ID collision is not proof. Verification reads and decodes the original
start event through the configured data converter and payload codec; it must
not publish a payload digest into search attributes or other visible metadata,
where low-entropy sensitive values could be guessed. A mismatch is a collision
or tampering error and never dispatch success.

The dispatch credential therefore needs the minimum history-read permission
required for reconciliation in addition to workflow-start permission. If
deployment policy withholds that permission, an ambiguous start remains an
explicit ambiguous error for the caller; the driver must not guess acceptance.
That reduced-permission mode and its operational recovery procedure must be
documented separately.

Duplicate rejection is retention-bounded. Temporal's
[workflow ID policy](https://docs.temporal.io/workflow-execution/workflowid-runid)
checks retained closed executions; expiry or administrative deletion removes
that protection. The driver must not claim permanent deduplication. A bounded
internal retry must retain its original immutable request and identity. An
operator reconciling an older ambiguous start must not submit another start
when the original history is no longer available and prior acceptance cannot
be determined. Return an unresolved outcome for application/operator handling;
external business idempotency is required beyond the service retention window.

The current public API does not accept a caller-selected dispatch ID. Queue's
ordinary dispatch and its chain/batch builders allocate new IDs on each call.
Calling `Dispatch` again, including on the same builder, is a new logical
submission. Returning an ID beside an ambiguous error provides correlation, not
a portable same-ID retry API. The driver may retry/reconcile the original
request internally within its bounded call. Cross-call or post-restart safe
resubmission needs a separately designed additive contract; it must not be
implied by this design or implemented by hashing application payloads. Native
applications can instead use the SDK's explicit ID and conflict/reuse policies.

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

The adapter must use the following explicit policy rather than inherit SDK
or service defaults. It stores original queue policy and resolved Temporal
policy separately in the immutable start envelope. Replay reads resolved policy
from that envelope, not current process configuration.

| Input | Initial Temporal driver policy |
| --- | --- |
| Unset retry or `Retry(0)` | `MaximumAttempts=1` |
| `Retry(n)` for positive n | `MaximumAttempts=n+1`; reject values that overflow the SDK field before acceptance |
| Unset backoff or `Backoff(0)` | Fixed one-second interval |
| Positive backoff | Round up to whole seconds, with minimum one second; set coefficient `1` and matching maximum interval |
| Unset timeout or `Timeout(0)` | Resolve from required positive `temporalqueue.Config.DefaultActivityTimeout` |
| Positive timeout | Start-to-close timeout, rounded up to whole seconds |
| Unset delay or `Delay(0)` | No workflow timer |
| Positive delay | One deterministic workflow timer before the first activity, rounded up to whole seconds |

`DefaultActivityTimeout` must be at least one second and representable by the
SDK. Construction requires this setting even if current callers supply their
own job timeouts. GoForj must project an explicit value into each Temporal
resource rather than invent a hidden framework default. Root, named, and
App-prefixed environment key spelling is a generator-contract output. Zero
never means unlimited Temporal execution. The one-second minimum/rounding is a
driver policy, not a claim that every Temporal API has that resolution. Native
SDK workflows retain their own timeout, timer, and retry policies.

`Retry(n)` on this driver bounds total Temporal activity attempts, including
worker-loss and heartbeat/start-to-close timeouts. This deliberately differs
from SQL queue stale-claim recovery and `busruntime.Uncommitted`, which can
redeliver without consuming an application attempt. At `Retry(0)`, losing the
worker after an activity starts can leave that activity terminally failed
without another handler invocation. Workflow replay still recovers orchestration
state; it does not replenish an exhausted activity budget. Documentation must
not promise unconditional job recovery, SQL redelivery parity, or at-least-once
handler invocation under every bounded-failure scenario.

The initial generic executor supports ordinary handler errors and permanent
errors. An application/middleware `busruntime.Uncommitted` outcome is unsupported
by this driver and becomes a sanitized non-retryable unsupported-outcome failure;
it must not be silently interpreted as budget-free redelivery or allowed to
loop indefinitely. This limitation applies only to the new driver. Existing
drivers keep their established outcome classification. Apply the Temporal
unsupported-outcome translation before shared retry/settlement observers classify
or publish the result. Do not change `busruntime.ClassifyAttempt` globally or
let the old built-in coordinator emit a conflicting redelivery fact first.
Contract tests cover the full handler, middleware, translation, and observer
path. A different logical versus infrastructure retry model would require its
own design and proof, not an extra retry loop behind the current policy.

Native queue handlers see `Attempt=TemporalAttempt-1`, explicitly the total
Temporal delivery-attempt index for this driver. Milestone C must prove the
portable workflow contract accepts this documented driver distinction, or stay
unsupported. It must not claim full existing-driver parity by narrowing the
contract silently.

Queue delay uses workflow time, never `time.Sleep` in replayed workflow code.
`OnQueue` selects the declared activity queue; workflow and activity roles
remain distinct even when they use the same default task queue name.

Framework-owned workflows must not add a second automatic retry layer around
activity retries. Ordinary job, chain, and batch workflow executions use no
workflow-level retry policy unless queue later defines one independently. The
activity retry policy alone implements the documented total-attempt bound
and `Backoff`; otherwise one exhausted activity could cause the entire workflow
to run again and exceed that bound.

Batch behavior follows the existing aggregate contract. Schedule every bounded
member before awaiting results. On the first terminal member failure without
`AllowFailures`, set public `Cancelled=true` and `Completed=true` immediately,
while retaining `Pending>0` for members not yet settled. Do not cancel already
accepted activities or discard their results. They continue executing and
updating `Processed`, `Pending`, and `Failed`. With `AllowFailures`, completion
occurs after every member settles, with `Cancelled=false` even when `Failed>0`.

Portable aggregate completion and Temporal execution completion are different.
The Temporal workflow remains open until all admitted members and durable
continuation attempts settle. First failure must not return from the workflow
and strand remaining activities. Workflow-level failure is reported only after
that drain, using a safe aggregate outcome; continuation exhaustion remains a
separate recorded outcome. Explicit operator cancellation/termination is a
different interruption path and cannot promise remaining business side effects.

Within an activation, collect all ready member outcomes and apply them in a
stable declared order (input member index), assigning monotonic transition
sequences. The journal records the actual resulting order. Named continuation
triggers use that order; replay must not depend on Go map or selector order.
Cancellation delivery through heartbeats remains cooperative and must not be
advertised as prompt interruption of a nonparticipating handler. See the
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
exposes total Temporal attempt index minus one before middleware, handlers, or
observers see it. This is not SQL's infrastructure-redelivery index.

Error translation must be bidirectional and tested:

| Queue outcome | Temporal representation |
| --- | --- |
| handler success | successful activity completion |
| ordinary handler error with retry budget | retryable application failure |
| `queue.Permanent(err)` | non-retryable application failure |
| exhausted queue retry budget | final activity failure |
| handler panic | Sanitized application failure under the configured total activity-attempt budget; no implicit unlimited retry |
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
| constructor `WithWorkers` and `Queue.WithWorkers` | Total remote activity concurrency per process and queue instance; local activity and Nexus budgets are separate; multiple activity task queues require the shared-capacity gate |
| `WithLegacyDirectEnvelope` | Rejected because it selects an obsolete physical-delivery envelope rather than a Temporal execution contract |
| `WithContext` | Supported for client calls; cancellation after acceptance does not cancel the workflow |
| `Driver` | Returns `temporal` |
| `Register` | Registers the shared handler as a Temporal activity target |
| `StartWorkers` and `Run` | Start the instance's workflow and activity pollers as one lifecycle unit |
| `Shutdown` | Drains and stops pollers, then closes the client within the supplied context budget |
| `Ready` | Supported with distinct client and worker states |
| `Chain` and `Batch` | Dispatch returns unsupported in the internal ordinary-job stage; native coordination is enabled only after portable workflow gates pass |
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
so the Temporal driver maps it to total concurrent activity execution across
the queue instance, not a value repeated for every task queue or a fleet-wide limit. Native remote
activities on those workers share the configured slots. SDK local activities
and Nexus work have separate budgets and must not be included in this claim.
Milestone B rejects SDK `Tuner` and `EnableSessionWorker` settings at
construction. Sessions create additional internal task queues and separate
activity slots, so they are outside the initial capacity/routing claim. A later
session/tuner capability requires a reviewed budget, routing, and versioning
contract; preserving SDK options does not imply every combination is supported. Workflow-task
poller count and concurrency belong in `temporalqueue.Config` because they do
not represent additional application job handlers. Both values must have
documented defaults and bounds.

The internal ordinary-job stage supports one activity task queue per queue resource.
Additional isolation uses existing named queue resources. Multiple activity
task queues within one Temporal instance remain disabled until Phase 0 proves a
fair shared-capacity implementation that preserves the total `WithWorkers`
limit, does not prefetch tasks into local waiters, does not cause
schedule-to-start or start-to-close timeouts while waiting for a local slot, and
cannot starve a low-volume task queue. Repeating the configured worker count for
each SDK worker is not acceptable.

The root queue currently has admin-specific unsupported errors but no general
workflow-coordinator unsupported error. Milestone A must add a small domain-neutral
error contract for unsupported workflow operations, with `errors.Is` tests; do
not pretend an existing sentinel already covers this case. Existing driver
behavior remains compatible. Generated Temporal providers must not inject the
built-in workflow store or clock implicitly and then trigger their rejection.

Every unsupported constructor option must fail during construction. The
existing `Queue.WithWorkers` fluent signature is preserved: invalid changes
that cannot be reported immediately are retained as configuration errors and
reported by `StartWorkers` before any poller starts. A late worker-count change
must not silently retune a running SDK worker; record the conflict so readiness
and the next error-returning lifecycle operation report it. Existing drivers
retain their current worker-count behavior. Every
unsupported job or workflow option must fail before the service accepts an
execution. A post-construction operation with an error return must return its
established unsupported-capability error. Capability helpers must report the
same result as the operation.

## Durable Continuations And Ergonomics

Named continuations are required, consistent with queue's existing D-007
roadmap. Existing closure callbacks remain explicitly ephemeral on existing
drivers. Temporal rejects closure-bearing workflows before acceptance. Durable
named jobs provide the supported alternative in the first complete release.
Observers never schedule continuations or control state.

### Compatibility-Preserving API

`ChainBuilder` and `BatchBuilder` are exported interfaces. Do not add required
methods to them, change `Queue.Chain`/`Batch` return types, or silently break
external builders and mocks. Add one domain-neutral binding helper and a
concrete continuation builder. Proposed API, not an existing executable example:

```go
workflowID, err := queue.Continue(q.Chain(first, second).OnQueue("reports")).
    OnFailure(queue.NewJob("reports:failed").PayloadJSON(report)).
    OnCompletion(queue.NewJob("reports:finished").PayloadJSON(report)).
    Dispatch(ctx)
```

The same helper accepts a configured batch. It exposes `OnSuccess(Job)`,
`OnFailure(Job)`, `OnCompletion(Job)`, and batch-only `OnProgress(Job)`; repeated
calls append jobs in declaration order. The existing builder is configured
before binding. An internal queue-owned capability provides the immutable
workflow definition, so unsupported custom builders fail at `Dispatch` before
any acceptance. Mixed ephemeral closures and named hooks on the new wrapper
also fail there. Existing closure-only behavior on existing drivers is unchanged.

Keep each configured Job's type, payload bytes, queue and policy intact. Add
`Message.Continuation() (Continuation, bool)` as a domain-neutral accessor for
stable invocation identity, parent correlation, phase/sequence, and an immutable
safe trigger summary. It does not replace the application's Job payload with a
new envelope. Trigger summaries omit unrelated node payloads; handlers needing
full state use an authorized injected read service. Add
`Queue.FindContinuations(ctx, workflowID)` for pending, scheduled, succeeded,
exhausted and interrupted invocation outcomes, safe failure categories and
timestamps, with no Temporal SDK types. Return triggered durable intents only;
untriggered hook definitions are not phantom pending invocations. Scheduled
means durably scheduled and unsettled, not proof that a remote handler has
physically started. Administrative interruption is not retry exhaustion.
Fakes separately record definitions, invocation metadata and outcomes.

### Trigger And Ordering Contract

| Hook | Durable trigger |
| --- | --- |
| Chain success | All business nodes succeed |
| Chain failure | The first terminal business-node failure |
| Batch failure | First terminal member failure, including with `AllowFailures` |
| Batch success | Aggregate `Completed && !Cancelled`, matching existing `Then`; allowed member failures can coexist with this hook |
| Batch progress | Each first committed member outcome, success or failure, never each retry |
| Completion | First aggregate terminal transition, not physical execution drain |

Record each trigger's immutable snapshot at its transition. For one batch
transition, append progress intents first, then first-failure/success intents
as applicable, then completion intents. Within a hook use declaration order.
Drain the continuation ledger in that deterministic order. Business-result
collection continues through SDK workflow concurrency while a continuation is
waiting; it must not stop updating admitted members' outcomes. Exhaustion of one
continuation is recorded and does not suppress later completion work or rewrite
the parent's business counters. Public aggregate state can be terminal while
continuation outcomes are still pending or scheduled.

Persist definitions and resolved policies in the initial workflow input.
Invocation identity includes resource/namespace, parent workflow lineage and
DispatchID, phase, triggering transition/member, and declared job index. Activity
retries retain that identity. Parent/workflow retry and administrative reset
semantics must not produce a false exactly-once side-effect promise. Handlers
receive a stable idempotency key and remain responsible for idempotent external
effects; replay alone must not create an extra logical continuation intent.
Every continuation has bounded attempts/timeouts under the same explicit
Temporal policy as other registered jobs.

### Commit Boundary And Existing Coordinators

Temporal atomically records the parent transition and all newly triggered
pending intents in one successful workflow-task commit. It schedules only the
eligible ledger head: that head's scheduled delta and activity command commit
together, in the same task if ready or a later task otherwise. Later heads are
scheduled only after predecessor settlement. Do not schedule every pending hook
at once and claim serial handler execution. Its execution stays open until all
admitted business activities and the ledger settle. Record pending, scheduled
and settlement deltas without observer delivery or mark-before-invoke claims
as durability evidence.

For existing built-in coordination, implement an additive store capability for
atomic workflow transition plus continuation intents/outbox rows. Preserve the
existing `WorkflowStore` interface. A custom store lacking that capability
rejects named continuation dispatch before acceptance; it must not commit state
and then try an untracked enqueue. Memory mode is explicitly process-local.
SQL durable mode commits intents with the parent transition and recovers their
publication after process death. SQLite, MySQL, and PostgreSQL integration tests
must prove atomicity and recovery. This shared capability is a prerequisite for
publishing the new root helper, not a Temporal-only public API.

Continuation schema ownership follows the store's existing auto-managed versus
externally managed mode. Automatic mode adds versioned continuation/outbox
relations without changing established rows. Managed mode performs no DDL and
validates the continuation capability before accepting named-hook workflows.
Do not require new tables merely to keep existing closure-only workflows or
ordinary store operations working. Custom stores lacking the capability retain
all existing behavior while rejecting the new durable binding before acceptance.

Roll out capable workers and the continuation schema before producers emit the
new durable envelope. Old workers must not consume or misinterpret new
continuation deliveries. Before rollback to an incapable binary, stop new named
workflows and drain/complete or explicitly migrate their outstanding intents
and invocations; retain additive tables and protocol-decoding workers as needed.
This is an operational/schema and new-envelope rollout, not a change to the
existing WorkflowStore source contract. Test old-schema upgrades, managed
no-DDL behavior, worker/producers order and rollback across SQLite/MySQL/Postgres.

### Built-In Recovery Runner And Retention

The built-in coordinator owns continuation state/outbox operations, not physical
worker lifecycle. A queue resource's shared runtime hosts its selected
coordinator's continuation runner as an explicit lifecycle participant. Temporal
uses workflow tasks as that runner and creates no separate SQL recovery process.
Built-in durable stores use one resource-scoped recovery runner per active
consumer process; multiple processes compete safely through leased, fenced
store claims. Dispatch-only producers do not need a runner.

After physical workers start, the shared runtime starts this runner and rolls
back the worker start if runner startup fails. It scans committed eligible
intents even when no original business delivery survives. Store operations
atomically claim the next ordered invocation, confirm publication, record
settlement, and release a successor. They include resource identity, stable
invocation identity, lease generation and expiry; stale owners cannot settle
or advance a newer claim. The recovery loop is bounded and backs off on
transient errors. Fatal runner errors reach `Queue.Run` alongside physical
worker errors; readiness reflects a failed runner.

The publisher uses the internal canonical dispatch seam with the preallocated
invocation metadata, not a new public `Queue.Dispatch` call that allocates
another identity. Ambiguous enqueue retries keep that invocation identity.
Delivery checks the authoritative parent intent and claim before application
code, and commits the outcome through the store capability before releasing
the next ledger head. Duplicate publication can still produce duplicate
physical deliveries and effects around acknowledgement loss; invocation keys
and idempotent handlers remain necessary. Observers do not acknowledge intents.

On shutdown, stop new recovery claims, finish or release in-flight publication,
drain admitted handlers/settlement within budget, then stop the runner and
physical runtime. Leases permit another process to recover abandoned work.
Never wait for every global outbox item to finish before stopping one process.
Prototype concurrent runners, lost enqueue responses, lease expiry, publisher
crash with no surviving business delivery, ordered settlement and fatal errors
before freezing the additive store capability.

For named-hook workflows, business-terminal flags alone never establish prune
eligibility. Retain the parent definition, admitted member state, trigger
snapshots, intents and association/deduplication authority until business drain
and every continuation settles or is explicitly interrupted. Then retain the
necessary redelivery/recovery authority through the declared backend horizon.
If no finite safe horizon is configured/proven, do not automatically discard
that authority; require an explicit quiescent cleanup policy. Document the
storage and sensitive-data retention cost.

Prune related parent, member, intent, outcome and receipt records atomically
only when that eligibility is established under the same store synchronization
as claiming and settling. A claim, late member outcome, retry or unsettled
publication prevents pruning. Preserve the established pruning behavior for
workflows without named hooks. Rollback and old binaries must not run an
incompatible pruning path while named obligations remain. Race prune with
publication, acknowledgement, late settlement and rollback on all SQL dialects.

## Worker-Independent Workflow State

Select a versioned, history-derived state journal as the authoritative read
path. Do not depend on live query workers, eventual visibility, a second
workflow database, or replaying application functions in a read process.
The same pure transition reducer builds in-workflow state and external reads.
Queries may be a diagnostic optimization but cannot change the public contract.

### Journal Protocol

The initial accepted envelope contains complete original definitions, IDs,
metadata, continuation specs, original Job policy, resolved Temporal policy,
and a protocol version. It supplies initial state immediately after start,
before any workflow task is polled. Never replace original stored Job policy
with rounded Temporal values in `ChainState.Nodes`.

Persist compact versioned transition deltas through public SDK
`workflow.UpsertMemo` under one reserved key. Read the ordered
`WorkflowPropertiesModified` history events and their workflow-task commit
association, not the current `Describe.Memo` value. SDK v1.45 exposes
[UpsertMemo](https://github.com/temporalio/sdk-go/blob/v1.45.0/workflow/workflow.go)
without an experimental annotation and requires server support from 1.18.
The selected server still needs explicit compatibility and real-server proof.

A delta contains a monotonic sequence, protocol version, opaque member or
continuation identity, transition kind, safe outcome category, and workflow-time
timestamp. It contains no raw job payload, arbitrary handler error, credential,
or personal data. One delta is constant-sized; do not copy the entire chain
into every event. Only committed workflow-task deltas advance public state;
raw activity-completion events alone do not advance it. Workflow replay consumes
the existing history without repeating business handlers or creating new
logical deltas. Upsert failures are framework errors, never silent state loss.

A required prototype must prove multiple updates to the same memo key within
one workflow task retain separate ordered history events, command/intent
atomicity, worker death before commit, pagination, old histories, and approved
converter/context/codec behavior. Do not substitute private SideEffect marker
layouts or a new authoritative database if this proof fails; revisit the design
before implementing the dependent protocol.

SDK replay is necessary but insufficient for journal compatibility. In the
candidate SDK, replay matching of ModifyWorkflowProperties verifies the event
type without comparing memo contents. A worker-code change can therefore alter
legal deltas while retaining the same command count and pass ordinary SDK
replay. Add semantic replay fixtures that capture every regenerated delta and
compare decoded keys, fields, sequence, snapshot/intent identities and ordering
to the committed history. Include a negative control that changes a delta with
the same command count and must fail qualification. The in-workflow reducer,
external history reducer and expected public state must agree at each commit.
Do not infer protocol safety from SDK replay success alone.

### Public Field And Consistency Contract

| Field | Source and semantics |
| --- | --- |
| IDs, queue, batch name/failure policy, chain nodes and original Job policy | Immutable accepted envelope |
| Chain `NextIndex` | Number of committed successful nodes |
| Chain `Completed` | All business nodes succeeded |
| Chain `Failed` / `Failure` | First committed terminal node failure; Completed remains false; Failure is bounded safe category/text, not arbitrary `Error()` preservation |
| Batch `Total`, `Pending`, `Processed`, `Failed` | Initial members plus each first committed member settlement; Processed includes successes and failures |
| Batch `Completed`, `Cancelled` | First-failure/AllowFailures contract above; terminal aggregate may still have pending members |
| `CreatedAt` | Timestamp fixed in accepted input |
| `UpdatedAt` | Last committed portable state-transition workflow time, not SQL physical-start/retry touch timestamps |
| Continuation outcomes | Triggered committed intent/scheduling/settlement deltas; untriggered definitions are not invocations |

Resolve the exact RunID and a history watermark, then reduce one bounded history
prefix for that run. Include only complete committed journal records through
the watermark; never splice history from different runs. Return the latest
fully committed portable state in that prefix, without claiming a transaction
with concurrent external side effects. Bound pages, total encoded/decoded bytes,
work and deadline. If limits or decoding fail, return an explicit error, not
partial or invented state. Any read cache is non-authoritative and isolated by
cluster, namespace, resource, run lineage, protocol and serialization policy.
Validate workflow type, envelope version, strict transition sequence, member
membership, legal state transitions, counter invariants and continuation phase
association. Reject missing/duplicate/out-of-order sequences, unknown transition
kinds or member/specification IDs, duplicate settlement, impossible phase changes
and contradictory counters. Unknown or contradictory journal records are protocol errors,
not permission to skip records or execute compensating application work.

Bounded portable workflows do not continue as new in this release. Native
workflows retain continue-as-new independently. For operator cancellation,
termination, or execution timeout before business completion, expose an explicit
safe interruption outcome: chain Failed=true/Completed=false, or batch
Completed=true/Cancelled=true with actual committed counters retained. If the
business aggregate was already terminal, do not rewrite it when continuation
drain is interrupted; report interruption in continuation outcomes. Never
fabricate completion of pending members or continuation side effects.

Unknown or expired/deleted histories map to `ErrWorkflowNotFound` when the
service confirms absence; permission denial, unavailable service, unexpected
workflow type, unsupported protocol, partial history and unavailable keys remain
distinct. Reads are resource/namespace-bound authorized decryption surfaces:
`FindChain` legitimately returns the original stored payloads. Preserve those
fields for authorized callers while redacting diagnostics; application-facing
HTTP authorization remains the application's responsibility. Reads never invoke
handlers, continuations, observers, or workflow commands.

Retention remains operator-owned. `Queue.Prune` returns unsupported for
Temporal rather than deleting execution history to imitate store pruning.

### Portable Run Identity And Administrative Reset

The first supported portable protocol uses one initial run: no workflow retries,
cron starts, continue-as-new or administrative reset. Native application
workflows retain their SDK reset/versioning behavior independently. A privileged
operator can still force server operations outside the portable contract; the
module does not claim to prevent arbitrary administration.

For portable reads, resolve the current execution once, pin its exact RunID,
and compare that run with the service's first-execution-run identity. Require
the initial-run relationship and workflow attempt before reduction. If a reset
has created a new run, return an explicit unsupported-recovery-mode error;
do not silently select an older branch, merge histories, reuse its current
memo, or report the branch as ordinary retained state. Existing-run replay
without a reset remains supported.

Portable generic activities and continuation validation likewise verify actual
parent/run association from service history. A reset-created branch is not an
approved parent and must not invoke application handlers. Workers and producer
credentials should not receive reset/cron administration permissions. Document
that an unsupported reset branch must be terminated or handled by an explicit
operator migration, not retried by queue. Already-running old activities may
still have effects; no reset or cancellation promise implies undo or exactly
once. Invocation identity uses the original parent DispatchID and initial
service run identity, not an ambiguous latest-run alias. Any future portable
reset support requires a separate branching/idempotency/intent design.

Test SDK-issued resets before/after intent scheduling and settlement: reads
report the explicit mode error and new-branch activity validation rejects
execution. Keep native-reset acceptance tests separate. Do not silently accept
new branches because normal workflow-ID start reuse is reject-duplicate.

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

### Native SDK Integration Contract

Native applications receive the actual resource-scoped `client.Client` and SDK
registration interfaces. Preserve `client.Options`, SDK workflow/activity
registration options, and role-specific `worker.Options` through the optional
module. Do not reduce this to an environment-only list of settings or a custom
start/signal/query facade. SDK client methods remain available subject to the
process credential's permissions; using them does not imply administrative
privileges are granted by default.

The integration must establish these ownership rules before construction APIs
are finalized:

- A module-created client is owned and closed once after all its workers stop,
  including when no worker ever started. Borrowing requires a module-produced
  client binding with private immutable namespace, converter, failure-converter,
  propagator, and logger-policy configuration captured from its SDK options.
  The binding factory creates the actual client; Milestone B does not accept an
  arbitrary opaque `client.Client` plus separately claimed settings. One owner
  closes the binding after every borrower stops. Native consumers receive its
  actual SDK client and never close it. Generated providers expose that same
  configured client, not a second dial with divergent serialization settings.
  Queue construction rejects namespace/configuration conflicts with its binding.
  Original-history decoding uses the binding's authoritative converter and
  required workflow serialization context, including SDK interceptors/context
  propagation. Ownership is independent of whether the SDK client is lazy.
- Application registration receives an SDK-compatible registry before polling
  starts. It can register native result-producing activities, workflows with
  registration options, and supported Nexus services. It does not receive
  ownership of worker `Start`, `Run`, or `Stop`.
- Apply caller SDK options, then validate actual conflicts with driver-owned
  routing, lifecycle callbacks, worker roles, and concurrency. Initial session
  workers and custom tuners are explicitly unsupported for the reasons above.
  Compose compatible interceptors and metrics hooks. Do not silently overwrite a deployment policy,
  converter, fatal callback, or tuner. Reject conflicting settings explicitly.
- A registration fingerprint must describe the final registration set, including
  struct activity methods and explicit SDK names. Reserved names cannot be
  replaced with `DisableAlreadyRegisteredCheck` or shadowed by dynamic handlers.
- Native SDK type registration is completed before polling. Queue's logical
  `Register` contract is separate: existing code uses stable handler slots and
  tests replacement during startup and after registration. A generic Temporal
  activity must resolve the concurrency-safe logical handler registry without
  re-registering SDK types or reading that registry from workflow code. Preserve
  replacement behavior in the shared refactor, and test late logical registration
  separately. Fingerprints must not falsely report an unchanged logical handler
  catalog after its names change; they still cannot certify remote deployments.
- A workflow function or dependency-free method uses SDK `workflow.Context`.
  It never calls `queue.Dispatch`, queue builders, a live client, a database, or
  a queue clock directly. Those perform ordinary I/O and are not replay-safe.
  Use workflow activity/child-workflow commands instead.
- Queue handlers remain error-only. They may share domain service methods with
  native activities, but native typed results, heartbeat checkpoints, async
  completion, and Temporal-specific errors must not be forced through
  `queue.Message` or the portable error translator. Native activity execution
  retains the SDK activity context and behavior.
- Queue middleware applies to registered queue handlers. Native activities use
  SDK interceptors or application wrappers. Do not claim queue observer coverage
  for every native execution; SDK telemetry remains available independently.

Workflow and activity task queues with different names must register compatible
worker deployment identities when activities are intended to remain pinned to
workflow code. Temporal documents
[versioning behavior across activity task queues](https://docs.temporal.io/worker-versioning).
The integration must test pinned and auto-upgrade policies and reject or
document any independent-activity routing. A workflow-only build identity does
not protect a separately polled activity task queue automatically.

Native Temporal Schedules remain managed through the
[SDK Schedule client](https://docs.temporal.io/develop/go/workflows/schedules).
GoForj must not create a second recurring trigger for the same work, delete
Schedules on worker shutdown, or equate its process-local scheduler lifecycle
with server-owned scheduling. Native schedule creation is an explicit
application/deployment action.

### Native Acceptance Scenarios

Before claiming native Temporal support, prove through this module's public
construction and registration path, without constructing a second worker:

1. An approval workflow waits on a durable timer and signal, exposes a query
   and validated update, survives worker restart, then returns a typed result.
2. A result-producing activity controls workflow branching and compensation.
   A child workflow has explicit cancellation and parent-close behavior.
3. A long-lived workflow continues as new with state and completed message
   handlers. Client interaction follows workflow identity across run changes.
4. Custom data conversion, context propagation, interceptors, and SDK metrics
   work on both client and worker; native activity heartbeat/checkpoint and
   asynchronous completion remain available.
5. The chosen supported deployment versioning policy works across both worker
   roles during rollout and rollback. Native APIs absent from the selected
   SDK/server remain explicitly unadvertised.
6. Dispatch-only use, multiple named resources, borrowed-client shutdown, and a
   queue worker hosting both portable handlers and native activities retain
   unambiguous identity and ownership.

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
		TaskQueue:           input.ActivityTaskQueue,
		StartToCloseTimeout: time.Minute,
	})

	var result PaymentResult
	if err := workflow.ExecuteActivity(ctx, ChargePaymentActivityName, input).Get(ctx, &result); err != nil {
		return PaymentResult{}, err
	}
	return result, nil
}
```

`PaymentInput.ActivityTaskQueue` is an application-owned, persisted routing
field supplied by the resource's native starter, not an environment read inside
the workflow. The starter validates that it is the resolved default activity
queue or a declared external native target. In the default combined topology,
the SDK's omitted-task-queue behavior would also route correctly. The explicit
field keeps this example correct if an application later selects split roles.
Native SDK routing is never silently rewritten by the queue module.

An empty workflow struct is only a useful default shape. It does not enforce
determinism. Generated tests and CI must run Temporal's replay and static
workflow checks against representative histories.

### Driver Changes And Open Executions

Changing `QUEUE_DRIVER` affects new dispatches; it does not migrate open Temporal
workflows, histories, signals, or native application code to another backend.
Keep the old namespace, codecs, credentials, and compatible workers available
until executions drain or an explicit application migration completes. Stop
new submission to the old resource deliberately, and keep native message APIs
routed to its retained executions. Unsupported-capability errors must not
trigger automatic fallback dispatch to another driver.

Native applications intentionally depend on Temporal semantics. Portable jobs
retain their handler signatures, but durability, state availability, callback
support, and operational limits still follow the selected driver's capability
matrix. A driver switch is an operational transition, not a guarantee of
in-flight portability.

## History And Scale Limits

Temporal persists commands and results in workflow history. Passing an entire
queue chain or batch definition means every job payload becomes durable
workflow input, and every scheduled activity adds history events.

The compatibility adapter must define and validate before workflow start:

- maximum encoded chain and batch input size
- maximum portable chain length
- maximum portable batch fan-out
- maximum individual job payload accepted by the configured Temporal service
- maximum total potential continuation invocations, including batch member count
  multiplied by progress binding count plus all potentially triggered hooks
- a conservative history-event/byte budget for business and continuation work,
  journal transitions, retries, timeouts and reserved safety headroom
- behavior when activity results or failure details approach service limits

Use checked arithmetic before acceptance for the complete invocation and
projected history budgets. Reject overflow and an otherwise-small input whose
hook multiplication exceeds the measured supported budget. Include the largest
accepted member/hook/attempt composition and immediate-boundary rejections in
real-server tests. Portable continue-as-new is excluded, so history headroom
cannot be delegated to an unimplemented escape path. Unexpected external input
or failure growth still needs explicit bounded failure handling.

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

- synthetic, non-sensitive committed replay fixtures for the default CI path
- a separate restricted process for production-history capture and replay:
  approve sanitization, storage, CI access, retention, and key handling first;
  never commit raw production history or keys, and do not use rewritten histories
  as proof that an unchanged original history replays
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

Queue's root module declares Go 1.24.4. The versioned
[SDK v1.45.0 manifest](https://github.com/temporalio/sdk-go/blob/v1.45.0/go.mod)
declares Go 1.24.0. Version 1.45.0 remains the workflow-wrapper compatibility
candidate, not an unconditional pin. Standalone activities and newer SDK
requirements are future reconsiderations under the stability policy above.
Phase 0 must
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

For the initial internal ordinary-job stage, that declared activity set contains
exactly the queue resource's effective default queue. A different `OnQueue`
target fails before workflow acceptance. The plural topology below describes a
later capability that remains behind the shared-capacity and routing gates.

### Task Queue Topology

Milestone B defaults workflow and remote activity roles to the same effective
application queue name, derived from the queue resource's `NAME`. One SDK worker
hosts both roles; do not create competing half-registered workers on that name.
This preserves the SDK's native default that an omitted activity task queue
uses the workflow task queue. `NAME` retains its existing application queue
meaning while also supplying the workflow role's default.

An explicit split configuration may use a different workflow task queue, but
must pass both-role routing and versioning checks before being advertised. Its
native starter supplies the activity binding in persisted workflow input as in
the example above. Missing `TaskQueue` then retains the SDK's own behavior and
may be inappropriate for that application; the module never rewrites it.
One SDK worker is bound to one task queue, so split roles own a worker set:

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

In split mode the workflow-task worker disables remote activity polling unless
its task queue is also the declared activity target. Activity-only workers
disable workflow polling. Registration applies native remote activities to the
remote activity worker and native local activities to the workflow worker's
registry. If one native activity is used both ways, register it in both
registries. All remote activity workers register the shared queue handler
executor. Combined mode performs both registrations on one worker.
Configuration must reject duplicate, empty, or conflicting task queue roles and
must validate every effective default queue before startup.

The exact environment keys remain a Phase 0 decision, but the model has four
distinct values: queue resource identity, Temporal namespace, workflow task
queue, and the declared set of activity task queues. The initial activity set
has one member. `QUEUE_NAME` retains its established meaning as the default application job queue and supplies both role defaults;
explicit workflow routing remains a separate optional SDK configuration.

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
routing mechanism. It is computed from registered job, workflow, and activity
type names plus framework protocol versions, without including function
pointers, source paths, payloads, or secrets. A separate deployment identity
and Temporal worker-versioning policy account for code changes that preserve
those names.

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
admitted work and close an owned client only after all its workers have stopped.
Borrowed clients remain open for their supplying owner. When the
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

The SDK worker stop operation is not an idempotent public contract. Use one
module-owned stop arbiter per SDK worker; all explicit shutdown, rollback and
fatal paths join the same operation. The module fatal callback first signals
the runtime error, then synchronously initiates or joins that arbiter before
returning. The candidate SDK subsequently sees its stop channel closed and
skips its automatic second Stop. Do not merely signal a goroutine and return
from the fatal callback while that stop still races SDK ownership.

The callback can be executing inside the poller that Stop is waiting for. Its
self-poller wait may consume the configured SDK grace period. Treat that as a
forced SDK stop, not evidence of graceful drain. This design choice requires a
real-SDK proof before freezing the adapter; do not depend on panic recovery,
experimental stop plugins, sleeps or log inspection. User fatal hooks are also
bounded/drain-tracked application callbacks and cannot erase the fatal signal.

Distinguish three facts: admission has closed, SDK Stop has returned, and tracked
application work has drained. SDK Stop return is not proof all SDK poller/task
goroutines have exited. Its internal timeouts can leave cancellation-resistant
work alive. SDK Run can return earlier still. The arbiter provides its own
Stop-return result, while module admission/drain guards track application code.
Only the required combination permits owned dependency cleanup.

The module's admission/drain tracker wraps user activity interceptors outermost,
including remote and local native activities and portable handlers. Approved
converter, codec, propagator, failure-converter, logger/metrics callbacks and
other application adapters have the same protection because SDK decoding may
precede activity interception and encoding may follow it. If Nexus operations
are advertised, their handlers are covered too. Construction rejects unsupported
callback modes rather than pretending activity-handler tracking covers them.
Application-spawned goroutines and external asynchronous-completion workers are
application-owned; their lifecycle must be joined by their owner before shared
bindings/dependencies close.

Close admission and register active entries under one synchronization boundary.
After cleanup observes zero tracked work, late SDK callbacks must not reenter
application code or custom conversion dependencies. They receive a safe
shutdown classification without invoking a closed user adapter. Already-admitted
work retains its tracked lifetime until it actually returns. A pure SDK cleanup
callback can unwind later, but cannot reopen admission to user code.

Concurrent shutdown callers may expire while the arbiter or tracked drain is
still incomplete. Their return is bounded and reports incomplete shutdown; the
operation/result channel outlives those callers, retains required dependencies,
and supports later joins. Do not close a client or underlying service merely
because the first caller timed out. This follows the
[SDK worker contract](https://github.com/temporalio/sdk-go/blob/v1.45.0/worker/worker.go)
and requires tests against the candidate implementation, not assumptions about
an unexposed poller-termination signal.

Phase 0 must test this ordering against the actual GoForj runtime host. It must
not assume lifecycle hooks that the runtime host does not call.

The current runtime host cancels sibling runtime contexts and waits for every
`Run` method to return. Application lifecycle shutdown hooks execute only after
the command returns. Temporal worker shutdown must therefore occur inside the
jobs runtime after its run context is canceled. Deferring it to an application
shutdown hook would make the runtime host wait for a worker that is itself
waiting for a later hook.

The failure-aware Temporal `Queue.Run` uses required positive
`temporalqueue.Config.ShutdownTimeout` after cancellation, without changing the
established `Run(ctx)` signature or other drivers' defaults. Generated providers
resolve this from the App's queue shutdown budget. An explicit `Shutdown(ctx)`
uses the earlier of its caller deadline and this resource budget. The worker
adapter configures SDK drain bounds consistently and bounds caller return.
SDK Stop itself can continue beyond that deadline because it has sequential
waits and a shutdown RPC. Equating WorkerStopTimeout with the resource budget
does not prove an aggregate deadline. Keep its arbiter alive for completion
and subsequent joins rather than waiting unboundedly in the caller.

A caller deadline expiring does not establish terminal cleanup or authorize
closing the client while pollers still drain. Subsequent `Shutdown` calls can
wait for the same in-progress stop and finish cleanup, preserving queue's
retry-after-timeout behavior. Do not latch a caller-specific timeout as the
resource's permanent shutdown result. SDK `Run` returning is not drain-complete
evidence: the candidate SDK closes its stop channel before the drain finishes.
The stop adapter needs its own completion observation and must not depend on
experimental stop plugins.

### Generated App Binding Lifetime

Generated Apps own every resource's client binding at the App composition root.
Queue runtimes borrow those bindings even when only one queue uses them. Jobs
shutdown stops its workers and recovery participants, but does not close the
shared client while HTTP, scheduler, CLI, or other native SDK consumers may
still be draining. After RuntimeHost has joined all runtime consumers and the
tracked application-adapter/handler drain has completed, final App cleanup
closes each owned binding once, before its remaining dependency cleanup.
If a shutdown deadline expires with tracked work still alive, report incomplete
shutdown and preserve that work's dependencies. Generated process shutdown must
skip destructive dependency cleanup and take its explicit failure/exit path;
it must not report graceful completion or close the database/client underneath
live handlers. Embedded library callers retain responsibility for that process
policy and may join cleanup later. Go cannot forcibly terminate arbitrary user
goroutines safely.

A direct library consumer may let the queue own its binding; that consumer
must stop its other native-client users before `Queue.Shutdown`. Sharing raw SDK
client access does not create invisible reference counting around arbitrary
application calls. Borrowing explicitly moves close ownership to the supplying
owner. Add a barrier test where an admitted HTTP request continues using the
native client while Jobs has stopped, and verify the binding remains usable
until the request and the other runtime consumers exit.

### Construction And Client-Only Cleanup

Worker ownership and client-resource ownership are separate lifetimes.
Generated managers register application cleanup for every constructed queue,
including dispatch-only HTTP/CLI use and unselected named resources. A later
queue-constructor failure rolls back all earlier constructed owned resources in
reverse construction order before returning the original and cleanup errors.
A module constructor failing after it creates its own client closes that client
before returning. Borrowed bindings remain the supplying owner's responsibility.

Active worker shutdown still happens inside Jobs `Run` before it returns.
Final application cleanup then closes never-started resources and retries or
joins remaining cleanup idempotently, subject to the tracked-work fence above. It must not be the mechanism needed
to unblock a running Jobs runtime. Generated binding-owner cleanup runs after
all borrowing queues; partial App construction must have the same rollback
ownership. Direct consumers must call `Shutdown` even if they never start
workers. Construction and cleanup tests cover lazy clients that later dial.

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
job and workflow vocabulary. The bridge must prevent workflow replay alone
from invoking observers again.
It must distinguish physical attempts from logical settlement and must not
claim exactly-once external event delivery.

Application observers must never execute directly from replayed workflow code.
Portable events require stable event IDs and an emission path with a documented
delivery guarantee, such as activity-side emission with durable deduplication
or an external history consumer. If that path cannot preserve an existing
event kind, the driver reports that event as unsupported rather than emitting a
plausible duplicate.

Stable event IDs identify duplicates; they do not atomically publish external
side effects. An activity-side observer can publish twice if the worker dies
between publication and activity completion, and can lose a fact if completion
and publication are not atomic. The default observer remains best-effort
operational telemetry with those limits stated. A durable publication mechanism
is a separate capability and must not be an implicit requirement for the first
native SDK milestone. Ordinary activity-attempt telemetry must also distinguish
retries from workflow replay.

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
validate envelope version, encoded size, queue-owned identity shape, option
bounds, and allowed task queue using deterministic versioned input/protocol
rules. It must not read a live application registry, environment, remote
catalog, or injected service during replay. Dispatch-only producers may use an
explicit declared job catalog without constructing handlers; they must not be
required to register runnable handlers merely to dispatch. Where unknown-job
rejection is advertised before acceptance, the producer catalog must establish
that claim. The activity always verifies the actual worker handler registry
before execution; producer validation does not replace this check. It repeats
security-relevant checks before invoking application code. Unknown versions,
job types, or task queues fail permanently and must not enter an unlimited
retry loop.

Reserved framework workflow entry also validates actual service start policy,
not only fields in its decoded envelope. Read `workflow.GetInfo` and reject
workflow retry policy, cron schedule, unexpected workflow attempt, queue/type,
or identity incompatible with the bounded single-run protocol before scheduling
any application commands. Direct SDK starts can set those values outside the
input envelope. Return a sanitized non-retryable policy error; do not let a
caller-supplied workflow retry layer multiply job/continuation attempts. This
protects the per-execution contract, not service quota against namespace writers.

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

Portable error adapters must use a bounded, safe error representation rather
than serializing arbitrary handler error text, stack traces, or nested details.
Preserve stable categories such as permanent failure separately from diagnostic
text. Expose the SDK failure-converter setting alongside the payload converter;
a payload codec alone does not establish safe failure-message handling. Native
application errors remain subject to the application's configured converter
and redaction policy, which must be documented and tested. Inspection must bound and redact payload previews. Metrics must not
use workflow IDs, payload values, or other unbounded identifiers as labels.

### Continuation Association And Read Permissions

Before a continuation handler runs, validate its immutable declared Job spec,
index, type, exact payload, resolved policy, applicable phase, trigger member
and sequence against the parent workflow's committed intent. Derive the actual
parent WorkflowID, RunID, workflow type and ActivityID from SDK activity
execution context, not caller payload or propagated headers. Require a reserved
framework workflow type and matching parent/lineage identity.

The authoritative association is reconstructed from that exact parent run's
accepted envelope and committed journal through the scheduling intent. Compare
the stable invocation identity and actual scheduled ActivityID to that intent.
The worker therefore needs bounded history-read/decode permission through its
resource binding in addition to activity polling permission. An incremental
cache may reuse an already validated prefix under the same resource/run/codec
identity; it must not substitute caller assertions or current mutable memo.
If association cannot be established because the service/key is temporarily
unavailable, do not run application code; return a sanitized infrastructure
failure under the explicit bounded attempt policy. A definite mismatch is a
terminal rejected invocation.

Direct standalone invocation, unrelated native-workflow invocation, cross-parent
substitution, undeclared specification and false phase/member/sequence claims
must fail before application code. A namespace writer that legitimately starts
a framework workflow can still exercise that namespace's allowed jobs; these
checks do not create caller authentication or replace privilege isolation.
Apply the same validation and confidentiality rules to durable outbox delivery
from existing coordinators, using their atomically committed parent intent.

### Decoding And SDK Logging Boundaries

Typed workflow/activity validation runs after SDK argument decoding. The module
must bound payload count and encoded bytes before converter/propagator work,
and enforce expanded-byte and decoding-work limits inside any decompression,
decryption, or custom converter path. Checking the decoded envelope afterward
is insufficient. The candidate SDK's stock zlib codec reads expanded bytes
without a bound; it is not an approved untrusted-input codec for this driver.
Custom converter/codec/propagator support requires an explicit bounded-input
contract and direct-invocation tests, not a blanket claim that wrappers can
make arbitrary user code safe. Caller headers are untrusted too.

Deterministically malformed inputs fail with a sanitized terminal category
without invoking application handlers. Transient key-service/decryption-service
availability failures remain distinguishable and follow a bounded recovery
policy. The selected pre-entry mechanism must preserve native panic policy. SDK
propagator extraction can otherwise panic before workflow entry and retry
workflow tasks indefinitely under BlockWorkflow. Do not switch every native
workflow to global FailWorkflow to solve this. Approved module adapters bound
extraction and translate deterministic malformed-header rejection into a
sanitized context marker while returning a valid SDK context. A module-owned
workflow execution interceptor checks that marker before framework or native
application entry and returns the safe non-retryable rejection. No application
function executes for that marker. Actual application panics/nondeterminism
retain the selected native SDK policy.

Unadapted propagators/converters that can throw pre-entry panics, bypass the
bounds, or cannot provide this rejection contract are unsupported configuration
and fail construction. Arbitrary third-party callbacks are not made safe merely
by passing their SDK interface through. Transient extraction/key-service errors
also receive a bounded, explicit classification; they are not secretly retried
forever as malformed workflow tasks. Verify the exact SDK converter and
propagator paths independently, including native registrations. If the adapter
cannot satisfy the contract, do not freeze that mode or the dependent API. Test both framework entrypoints and native
registrations, including attacker-selected retry policies. This does not claim
protection against an authorized namespace writer consuming service quota by
starting arbitrary executions.

The SDK can log handler errors, panic values, stacks, and late activity output
before applying a failure converter. History encryption and portable error
translation therefore do not secure those logs. The module's client binding
must install or validate an SDK logger policy that drops unsafe values and
stacks by default, preserves safe categories and correlation, and bounds every
message and field. A caller-supplied logger must meet that contract; do not
silently route raw SDK diagnostics around it. Apply the same policy to borrowed
bindings, converter errors, native activities, and completion after timeout.
Sentinel tests must inspect actual SDK log output as well as history payloads.

An error-only portable handler can preserve permanent/retryable categories
without retaining arbitrary Go error strings. Native application error details
remain available only under the explicit safe failure-converter/codec policy.
Propagated actor/tenant headers and workflow input are caller-controlled claims,
not authenticated caller identity. Any deployment-owned authorization must
obtain trusted identity from an independently verified boundary.

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

## Complete Integration Acceptance Matrix

Real-server tests must use the public Queue construction/registration path.
SDK mocks and the in-memory workflow test environment supplement these tests;
they cannot pass lifecycle, persistence or worker-loss acceptance alone.

Use a pinned retained Temporal server, separate producer and reader processes,
a killable worker subprocess, and an external side-effect witness with explicit
barriers. Kill after handler start, after an external effect but before SDK
completion, before/after journal commit, and during continuation execution.
Restart without the original producer or its memory. Avoid timing-only sleeps.

| Area | Required real integration cases |
| --- | --- |
| Ordinary jobs | Payload bytes, middleware/context, all zero/explicit policies, permanent error/panic, actual attempt budgets and delay |
| Chains | Order, predecessor replay safety, first/final node failure, timeout, restart, original state fields and durable hooks |
| Batches | Every success/failure interleaving, AllowFailures, first-failure aggregate completion, late member settlement, bounded fan-out and execution drain |
| Continuations | Every hook, concurrent progress, stable identity/snapshot, declaration order, scheduled versus physical-start distinction, exhaustion/interruption, later completion hook, crash at each scheduling/effect/ack boundary |
| State journal | Multiple same-key deltas per task, commit loss, pagination, bounded decoding, malformed transition mutations and semantic replay parity including a same-command-count negative control |
| No-worker reads | Immediately after acceptance before polling, running/retry/delay state, all workers stopped, retained terminal histories and continuation outcomes |
| Native coexistence | Typed results, signals, queries, validated updates/with-start, timers, children, continue-as-new and the supported versioning policy beside portable work |
| Resource isolation | Default/named resources, identical IDs in different namespaces, owned/borrowed bindings, combined/split roles and actual activity capacity |
| Lifecycle | Constructor/start rollback, HTTP client use after Jobs stop, recovery-runner failure, self-poller fatal stop, separate SDK/admission/adapter drain, late callback fencing, resistant handlers, bounded caller return and later joins |
| Service outages | Start-response loss, reconciled acceptance, service loss during reads and continuation settlement, ambiguous outcomes and expired/deleted history |
| Security | Direct malformed job/chain/batch/continuation input, false phase/association, expansion/header abuse, authorization failure and safe SDK logs/failures |
| Serialization | Default plus approved encrypted/context-aware conversion, key unavailability/rotation, backward decoding, read/continuation confidentiality and sentinel secrets |
| Evolution | Prior synthetic histories, semantic replay parity, deploy/rollback, registry/version routing, codecs, portable-reset rejection and independent native reset support |
| Built-in stores | Real SQL transition/outbox atomicity, competing/fenced runner recovery with no business delivery, prune races, schema upgrade, managed no-DDL, worker-first rollout/rollback and custom-store rejection |
| Framework composition | Largest supported multi-App composition with jobs, workflows, continuations, HTTP, schedules, metrics, named resources and client cleanup |
| Scale | Largest supported member/hook/attempt composition, continuation multiplication, history budgets, checked arithmetic and immediate-boundary pre-acceptance rejection |
| Modules and release | Independent root/driver/integration/examples checks, race/vet, generation idempotence and published consumer without workspace/replacements |

Extend the existing `integration/root/workflow_contract_integration_test.go`
public contract suite and its capability expectations. Add Temporal to backend
factories, required CI matrices, matrix guards, coverage inventories and test
orchestration; do not hide it in a driver-only opt-in suite. Use the existing
nested integration module command surface (`go -C integration test
-tags=integration ./...`), and validate root, driver, integration and examples
with `GOWORK=off`. The existing root-oriented integration script needs adjustment
if it does not enter that nested module; do not claim root `./...` covered it.

For every required case, retain exact module/SDK/server versions, command,
result and artifact identity. A skipped scenario, unsupported temporary stage,
SDK-only fixture or documentation example is not full integration evidence.
There is no claim that these future tests have already passed.

## Testing Strategy

### Queue repository

- Run existing chain and batch contracts against the built-in engine.
- Define a compatibility subset and run it against Temporal.
- Assert unsupported options fail before execution starts.
- Prove retry, timeout, backoff, delay, task-queue, and error translation.
- Race start-response loss with an execution that completes before retry. Prove
  reject-duplicate reuse prevents a second run and reconciliation verifies the
  original start input before reporting prior acceptance.
- Prove the configured activity execution timeout is always nonzero, activity
  retry defaults are fully overridden, and heartbeat behavior matches every
  cancellation claim.
- Prove no application activity is repeated by workflow replay.
- Run the native acceptance scenarios through the module's actual public
  construction and registration path, including SDK configuration and client
  ownership, not only against standalone SDK test code.
- Preserve logical handler replacement during concurrent startup and execution
  without attempting SDK activity re-registration. Distinguish the logical
  handler catalog from native SDK type registration and replayed protocol data.
- Verify the unsupported workflow error contract with `errors.Is`, and prove
  generated Temporal providers do not add rejected built-in store/clock options.
- Prove workflow replay resumes orchestration and independently test activity
  recovery with remaining/exhausted budgets. Kill workers after task acceptance
  and during handlers under `Retry(0)` and `Retry(1)`; assert attempts, state,
  and observer facts without claiming SQL same-attempt recovery.
- Prove journal-derived state reads for running and retained terminal
  executions with every worker stopped; optional queries do not pass this gate.
- Prove shutdown ordering and bounded activity drain, including retrying a
  caller-timed-out shutdown and observing actual completion rather than SDK
  `Run` return. Test shutdown before worker startup and client-only use.
- Prove manager and module constructor rollback, cleanup of unselected queues,
  and binding-owner close ordering after all borrowers stop.
- Reject sessions/custom tuners, test deferred fluent configuration errors,
  and verify native activity routing in combined and explicitly split roles.
- Test corrupt encodings, compressed expansion, malicious propagation headers,
  and bounded converter work before framework validation. Verify sentinel
  secrets are absent from SDK errors, panic/stack logs, converter failures,
  late output, and encrypted context-aware borrowed-binding reconciliation.
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
- For any proposed multi-task-queue mode, prove one total activity concurrency
  limit, bounded SDK polling, no local slot-wait timeouts, and no starvation.
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
- Verify the jobs runtime performs active Temporal worker shutdown before
  returning. Separately verify final App cleanup for all constructed queues,
  including dispatch-only/unselected resources and constructor rollback.
- Verify a later queue startup failure shuts down every earlier queue that
  started successfully, in reverse start order and within the shared budget.
- Verify existing `queue:work` queue-resource filters select Temporal instances
  without introducing a workflow-specific command mode.
- Verify generated task queue defaults remain distinct across project, App, and
  named queue scopes, and inspection exposes a redacted registration
  fingerprint without claiming fleet-wide verification.
- Verify secrets are blanked from examples and diagnostics.

## Phase 0 Decision Spikes

The stages organize this backlog. State reads and durable continuations may be
implemented after internal ordinary-job work, but their proofs block complete
release and public configuration. No existing A/B approval waives these gates:

1. Split shared handler execution from workflow coordination without changing
   public queue behavior. Prove `Register`, ordinary `Dispatch`, worker start,
   worker shutdown, and readiness no longer depend on coordinator ownership.
   Preserve typed provenance for store, clock, middleware, observer, and worker
   options without changing the public `queue.Option` API.
2. Add a private native-workflow driver capability and prove every existing
   driver still selects the built-in coordinator. Add and prove the explicit
   unsupported mode. Prove the optional-driver bridge preserves the declared
   mode and capability instead of erasing them during adaptation.
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
22. Keep one activity task queue per instance, or prove a multi-queue capacity
    controller that preserves runtime-wide `WithWorkers` semantics without
    prefetch timeout or starvation.
23. Prove ambiguous-start reconciliation before and after fast workflow
    completion, with matching input, mismatched input, inaccessible history,
    encrypted payloads, namespace authorization failures, and expired/deleted
    history. Prove separate public dispatch calls produce separate identities.
24. Validate the chosen wrapper transport with a version-specific SDK/server
    matrix, real-server acceptance behavior, toolchain impact, and measured
    overhead. Standalone activity APIs remain excluded while experimental.
25. Prove the native acceptance scenarios with custom SDK options, reserved-name
    enforcement, declared producer catalogs, borrowed-client ownership, and
    replay-safe validation.

## Release Gates

The complete supported Temporal driver remains hidden and unqualified until
all required ordinary, portable, continuation, native and cross-cutting boundaries
have recorded evidence. Internal A/B progress does not bypass C acceptance.
Behavioral, protocol, schema, build and local-consumer rows are prepublication
qualification. The module-publication row is postpublication verification that
blocks advertising/consuming the release as supported, not the tag operation
needed to perform that verification:

| Boundary | Required outcome |
| --- | --- |
| Existing driver isolation | Every existing driver contract passes unchanged when the Temporal module is available but not selected |
| Coordinator selection | Existing drivers select built-in, incomplete development builds select unsupported, and workflow-capable Temporal selects native with no implicit fallback |
| Public surface | Every existing method and option is supported with tested semantics or returns an established unsupported-capability error at the earliest possible boundary |
| Native driver ownership | Root and named Temporal queues have unambiguous client, registry, worker-set, and owned/borrowed lifecycle rules without root-module Temporal types; only Milestone C requires a native portable coordinator |
| Native capability preservation | Milestone B passes the native acceptance scenarios through the same resource, with SDK options and registration preserved |
| Transport selection | Initial wrapper transport passes SDK/server/toolchain evidence and measured overhead; experimental standalone APIs and automatic fallback remain excluded |
| Temporal ordinary dispatch | `Queue.Dispatch` starts one framework-owned workflow and reports acceptance consistently |
| Dispatch idempotency | Framework starts reject conflicts and retained duplicate IDs, verify original immutable input during reconciliation, and document retention/deletion limits and fresh IDs on each public dispatch |
| Portable handler execution | The same registered handler and middleware execute through an existing driver and a Temporal activity |
| Callback durability | Durable named continuations, immutable trigger snapshots, outcome reads and recovery pass; closure callbacks still fail before Temporal acceptance |
| State reads | History-journal `FindChain`, `FindBatch`, and continuation outcomes meet the no-worker contract; failure blocks the complete supported driver release |
| Uniqueness | Exact `UniqueFor` parity is proven, or the option fails before acceptance |
| Retry and timeout policy | The explicit policy table, immutable resolved values, required activity timeout, and total-attempt worker-loss behavior are tested; no SQL redelivery parity is claimed |
| Retry ownership | Activity policy is the only automatic retry layer for framework workflows and never exceeds queue's attempt budget |
| Batch cancellation | Failure interleavings preserve established handler, counter, callback, and event behavior |
| Task routing | Every accepted task queue has a known local or declared external poller |
| Registry homogeneity | Shared task queues have compatible type and handler registrations, with reserved-name collisions rejected before polling |
| Activity capacity | Remote activity slots obey the per-process/resource limit; session workers/custom tuners are rejected initially; additional activity queues require fair capacity and bounded polling |
| History limits | Payload and fan-out limits are measured and enforced before acceptance |
| Evolution | Previous-version histories replay on forward deploy and rollback |
| Driver protocol | Persisted framework envelopes are versioned and every supported prior driver history replays under the proposed release |
| Lifecycle | Partial startup, cancellation, timeout, and repeated shutdown converge safely |
| Fatal worker failure | A post-start poller failure reaches `Queue.Run` and the GoForj runtime host without polling or log inspection |
| Stop idempotency | Concurrent lifecycle paths stop each worker and close each owned client at most once, never close a borrowed client, and return one stable result |
| Worker selection | Existing `queue:work` filters select whole queue resources, and a Temporal resource owns all pollers required by its declared task queues |
| Observer delivery | Portable events have stable identities and workflow replay cannot create duplicates |
| Security | Authentication modes, server identity, authorization boundaries, encryption, key rotation, redaction, retention, and audit ownership are documented and tested |
| Untrusted history input | Direct malformed or unauthorized invocation of framework workflow and activity types cannot reach application handlers or retry indefinitely |
| Audit evidence | Service-side audit evidence ownership and retention are documented without treating application observer events as an audit control |
| Disabled output | Renders that omit the Temporal driver contain no Temporal dependency or configuration |
| Toolchain compatibility | The selected Temporal SDK supports the established minimum Go version, or an unavoidable increase has an explicit migration |
| Module publication | The driver has its own planned tag, release-time sibling pin, proxy-resolvable module, and `GOWORK=off` downstream proof |

An unresolved row blocks the affected advertised capability, portable or native.
It does not justify a silent semantic downgrade, a second source of truth, or a new top-level
component.

## Implementation Sequence

1. Prove the complete-scope protocol prerequisites: history journal, continuation
   atomicity, SDK stop adapter, bounded serialization and selected server/SDK.
   These are prototypes, not a supported partial driver.
2. Land the compatibility-preserving shared execution/construction seam and
   continuation/store capability with unchanged existing-driver contracts.
3. Build ordinary Temporal jobs and native hosting internally, including
   resource-scoped clients, registration, routing, security and lifecycle.
4. Complete portable chains/batches, journal reads and durable continuations;
   implement root helper, fake support and built-in store atomicity together.
5. Pass the complete real-server matrix, relevant module/race/vet checks,
   synthetic replay fixtures, forward/rollback tests, and runnable examples.
6. Validate the largest GoForj composition with default/named queue resources,
   native and portable workflows, continuations, jobs, schedules, HTTP, metrics
   and multiple Apps. Framework renders stay under `/tmp`.
7. Review the full diff and compatibility/migration notes. Pass complete
   prepublication qualification against the exact intended artifact family,
   then publish synchronized root/driver tags with the repository scripts.
   Independently verify every published module through downstream resolution
   with `GOWORK=off` and no local replacements before advertising support.
   Publication is not claimed verified merely because tag creation succeeded.
8. Enable framework configuration/generators/docs against the qualified module.
   Stages may remain independently reviewable; they are not separate reduced
   product releases. Experimental SDK transports require a later explicit design.

## Decision

Temporal belongs in the queue driver catalog and must not implement
`WorkflowStore`.

The complete supported addition includes ordinary jobs, portable chains and
batches, worker-independent state reads, durable named continuations, and native
SDK access. Native capabilities remain directly usable without forcing richer
Temporal behavior into a portable facade. Internal implementation sequencing
does not permit a jobs/native-only release. Completion requires the combined
integration, replay, security, lifecycle and module-publication evidence.

The initial supported ordinary-job transport is the workflow wrapper.
Standalone activities remain a natural future mapping but are excluded while
the required SDK APIs are experimental. Duplicate suppression is bounded by
retention, and each public dispatch currently creates a new identity. These
limits must remain visible in implementation and documentation.

This design preserves one owner for workflow concepts, one authoritative state
system per execution, and one honest boundary between portable queue workflows
and Temporal-specific orchestration.
