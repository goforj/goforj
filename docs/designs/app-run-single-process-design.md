# `app run` Single-Process Host Design

## Summary

Move generated `app run` from a subprocess supervisor to a single-process runtime host that boots the enabled logical runtimes together:

- `api`
- `scheduler`
- `jobs`

The important constraint is that this changes the **hosting model**, not the **runtime identity model**.

`api`, `scheduler`, and `jobs` must remain first-class logical runtimes for:

- metrics labeling
- log prefixes / structured component fields
- Lighthouse agent identity
- lifecycle reporting
- operator-facing commands
- future scale-out mental model

This design is specifically for `app run` as the standalone path. It does **not** require changing:

- `http:serve`
- `schedule:run`
- `queue:work`
- `forj dev`
- containerized or distributed deployments

Those continue to represent explicit runtime boundaries and production-parity workflows.

## Why

Today `app run` is not actually “standalone in one process.” It shells back into the same binary and supervises separate child processes.

That is visible in:

- [`templates/internal/cmd/run_cmd.go.tmpl`](../../templates/internal/cmd/run_cmd.go.tmpl)

This creates a problem for process-local primitive drivers such as `inproc`:

- events do not cross `api` / `scheduler` / `jobs` boundaries
- memory-backed primitives behave as process-local islands
- the standalone story becomes surprising even though the app is running “locally”

If `app run` is meant to be the strongest zero-infra standalone path, it should host all enabled runtimes in one address space so process-local primitives behave honestly.

## Decision

`app run` should become a **single-process multi-runtime host**.

It should:

- boot enabled runtimes in one OS process
- preserve logical runtime identity as `api`, `scheduler`, and `jobs`
- keep leaf commands (`http:serve`, `schedule:run`, `queue:work`) available and unchanged
- leave `forj dev` and explicit distributed/process-isolated workflows intact

## Explicit Non-Goals

- Do not redefine `inproc` to mean cross-process.
- Do not build a hidden GoForj-only bridge transport for subprocesses.
- Do not merge runtime concepts into one undifferentiated “app” identity.
- Do not remove the ability to run the runtime commands separately.
- Do not change distributed deployment topology.

## Audit Findings

### 1. `app run` is currently a subprocess supervisor

The generated `RunCmd`:

- constructs a list of processes (`api`, `scheduler`, `jobs`)
- resolves the current executable path
- re-execs the same binary with `http:serve`, `schedule:run`, and `queue:work`
- injects subprocess-only env vars such as:
  - `FORJ_SUBPROCESS=1`
  - `FORJ_COMMAND_ORIGIN=run_command`
  - `APP_LOG_PREFIX=<App> › <Component>`

Source:

- [`templates/internal/cmd/run_cmd.go.tmpl`](../../templates/internal/cmd/run_cmd.go.tmpl)

This means runtime identity today is preserved largely by OS process boundaries plus env decoration.

### 2. Lifecycle is already app-level and reusable

Generated `wire.App` already owns a shared lifecycle:

- `App.Startup(ctx)`
- `App.Shutdown(ctx)`
- lifecycle hooks for DB, events, queue workerpool, custom registry hooks

Sources:

- [`templates/wire/app.go.tmpl`](../../templates/wire/app.go.tmpl)
- [`templates/internal/app/lifecycle.go.tmpl`](../../templates/internal/app/lifecycle.go.tmpl)
- [`templates/internal/app/lifecycle_registry.go.tmpl`](../../templates/internal/app/lifecycle_registry.go.tmpl)

This is good news: the codebase already has a root runtime coordination layer. We should extend that pattern rather than invent a parallel one.

### 2.1 `run` currently skips root lifecycle on purpose

Today `wire.App.Run()` explicitly skips app lifecycle startup/shutdown for the `run` command unless it is being invoked from the scheduler subprocess path.

Sources:

- [`templates/wire/app.go.tmpl`](../../templates/wire/app.go.tmpl)

This behavior exists because:

- `run` currently only supervises child processes
- the child runtime commands own the real long-lived work
- the root process should not also start buses, workers, or other background resources

This becomes a critical implementation pivot for single-process hosting:

- once `run` becomes the real host, it must no longer skip root lifecycle in the same way
- lifecycle ownership must be made explicit so host mode starts shared resources exactly once
- direct leaf commands must continue to avoid double-owning lifecycle concerns

### 3. Leaf runtime commands are the current runtime boundaries

The actual long-lived runtime behavior is encapsulated in:

- [`templates/internal/http/serve_cmd.go.tmpl`](../../templates/internal/http/serve_cmd.go.tmpl)
- [`templates/internal/scheduler/cmd.go.tmpl`](../../templates/internal/scheduler/cmd.go.tmpl)
- [`templates/internal/jobs/worker_cmd.go.tmpl`](../../templates/internal/jobs/worker_cmd.go.tmpl)

Each of these does real runtime-owned work:

- starts dedicated metrics endpoint
- starts runtime-specific Lighthouse agent
- runs the long-lived server / scheduler / queue worker

This means `app run` cannot simply “call the commands one after another.” It needs a host that can run these runtime services concurrently while preserving their identity.

### 4. Metrics identity is currently process-derived at the command boundary

Per-runtime metrics endpoints are started here:

- API: [`templates/internal/http/serve_cmd.go.tmpl`](../../templates/internal/http/serve_cmd.go.tmpl)
- Scheduler: [`templates/internal/scheduler/cmd.go.tmpl`](../../templates/internal/scheduler/cmd.go.tmpl)
- Jobs: [`templates/internal/jobs/worker_cmd.go.tmpl`](../../templates/internal/jobs/worker_cmd.go.tmpl)

The helper itself is generic:

- [`templates/internal/metrics/endpoint.go.tmpl`](../../templates/internal/metrics/endpoint.go.tmpl)

Current assumptions:

- one process per runtime role
- one metrics listener per role
- scrape config expects:
  - `METRICS_API_PORT`
  - `METRICS_JOBS_PORT`
  - `METRICS_SCHEDULER_PORT`

Single-process hosting breaks this if left untouched because:

- one process cannot naturally own three independent “per-process” identities unless we model them explicitly
- binding three listeners from one process is possible, but they would all expose the same registry unless we change runtime metrics partitioning

### 5. Lighthouse identity is also currently process-derived

Each runtime creates its own Lighthouse agent with a distinct `Source`:

- API uses `"api"`
- Scheduler uses `"scheduler"`
- Jobs uses `"jobs"`

Sources:

- [`templates/internal/http/lighthouse.go.tmpl`](../../templates/internal/http/lighthouse.go.tmpl)
- [`templates/internal/scheduler/lighthouse.go.tmpl`](../../templates/internal/scheduler/lighthouse.go.tmpl)
- [`templates/internal/jobs/lighthouse.go.tmpl`](../../templates/internal/jobs/lighthouse.go.tmpl)
- agent config loader: [`templates/internal/lighthouse/agent.go.tmpl`](../../templates/internal/lighthouse/agent.go.tmpl)

This identity is worth preserving. It is a product feature, not just an implementation detail.

### 6. Logging identity is currently env/prefix-derived

Structured logging reads:

- `APP_LOG_PREFIX`
- `FORJ_COMMAND_ORIGIN`
- `FORJ_SUBPROCESS`

and derives user-visible prefix / structured fields from those values.

Source:

- [`templates/internal/logger/app.go.tmpl`](../../templates/internal/logger/app.go.tmpl)

Single-process hosting will need an explicit runtime-scoped logger pattern, because env vars are not enough once multiple logical runtimes coexist inside one process.

### 7. Some subprocesses remain intentional even after this change

Scheduler command jobs still shell out intentionally:

- [`templates/internal/scheduler/scheduler.go.tmpl`](../../templates/internal/scheduler/scheduler.go.tmpl)

That is a separate concern from `app run`.

This design should not try to eliminate all subprocesses from the generated app. It should only eliminate the top-level `run` supervision subprocess fan-out.

## Core Design

### New Concept: Runtime Host

Introduce a generated internal abstraction that hosts multiple logical runtimes within one process.

Suggested package ownership:

- `internal/app`

Suggested types:

- `Runtime`
- `RuntimeHost`
- `RuntimeIdentity`

Example shape:

```go
type Runtime interface {
	Name() string
	Run(ctx context.Context) error
}

type RuntimeHost struct {
	logger   *logger.AppLogger
	runtimes []Runtime
}
```

This host should:

- start enabled runtimes concurrently
- cancel sibling runtimes on first non-graceful failure
- propagate shared shutdown
- own shared host-mode lifecycle boundaries explicitly
- preserve runtime labels in logs / metrics / operator surfaces
- return aggregated or primary failure cleanly

### Runtime Identity Must Be Explicit

Do not infer runtime identity from:

- current command name
- `FORJ_SUBPROCESS`
- log prefix env only

Instead introduce a runtime identity concept at the application level:

```go
type RuntimeIdentity struct {
	Name  string // api, scheduler, jobs
	Label string // API, Scheduler, Jobs
}
```

This should be passed to runtime-owned adapters:

- runtime loggers
- Lighthouse agents
- metrics endpoint startup
- runtime host status reporting

### Preserve Existing Leaf Commands

Do not rewrite the leaf commands into special `app run` only behavior.

Instead:

- keep `http:serve`, `schedule:run`, `queue:work`
- factor their long-lived work into runtime-owned services if needed
- let both:
  - the leaf commands
  - the new `RuntimeHost`
  call the same underlying runtime entrypoints

That avoids drift between standalone and explicit runtime operation.

### Lifecycle Ownership Model

The clean ownership split should be:

- `app run`
  - owns shared app lifecycle in host mode
  - starts shared primitives once
  - starts enabled logical runtimes concurrently
- `http:serve`
  - continues to own direct API runtime execution
- `schedule:run`
  - continues to own direct scheduler runtime execution
- `queue:work`
  - continues to own direct jobs runtime execution

What must not happen:

- root lifecycle starts event buses and worker pools
- then a leaf runtime starts overlapping runtime-owned resources again

Implementation implication:

- shared primitive lifecycle and runtime-local long-lived loops need to be clearly separated
- `shouldSkipAppLifecycle(command, origin)` should not stay as a hidden policy hack once host mode exists
- host mode likely needs an explicit branch rather than more origin/env-driven exceptions

## Recommended Refactor Shape

### Phase 1: Introduce hostable runtime services

Refactor command-adjacent runtime logic into hostable services without changing behavior yet.

Examples:

- `http.ServeCmd` should remain a CLI command wrapper, but the long-lived runtime work should be callable as a hostable `http.Runtime` or equivalent.
- `scheduler.Cmd` should wrap a hostable scheduler runtime.
- `jobs.WorkerCmd` should wrap a hostable jobs runtime.

The command wrappers can continue to:

- bind flags
- create signal contexts when run directly

But they should stop being the only place where runtime startup semantics live.

Concrete likely additions:

- new files under `templates/internal/app/` for:
  - runtime identity
  - runtime host
  - runtime registration / assembly helpers
- thin runtime wrappers in:
  - `internal/http`
  - `internal/scheduler`
  - `internal/jobs`

### Phase 2: Add runtime-scoped logger derivation

Introduce a way to derive a child logger for a logical runtime:

- `api`
- `scheduler`
- `jobs`

Requirements:

- console output still visually shows component identity
- structured JSON logs still include `component`
- no dependence on fake subprocess env

This likely belongs in `internal/logger`.

### Phase 3: Add runtime-scoped Lighthouse startup

Lighthouse agent creation should become runtime-owned, not process-owned.

That means a single OS process may host multiple agents:

- one for `api`
- one for `scheduler`
- one for `jobs`

This is acceptable as long as:

- each uses a unique `Source`
- each keeps its existing capabilities
- each reports its own heartbeats and logs

### Phase 4: Decide metrics strategy explicitly

This is the biggest design choice.

There are two viable directions:

#### Option A: Keep one shared registry, expose one `/metrics`

Pros:

- simplest standalone story
- no three extra listeners
- aligns with single-process reality

Cons:

- dashboards and scrape config must stop assuming one endpoint per runtime
- metric labeling must carry enough runtime identity to preserve separation
- existing `process=api/jobs/scheduler` scrape labels go away unless replaced by metric labels

This is likely the best long-term fit for single-process `app run`.

#### Option B: Keep separate metrics listeners per logical runtime

Pros:

- closer to current observability topology
- easier Grafana/Victoria compatibility initially

Cons:

- awkward in one process
- all listeners would still reflect one OS process
- requires runtime-specific filtered registries or prefixed handlers
- much more plumbing for less conceptual clarity

Recommendation:

- for `app run`, move toward **one metrics endpoint** for the host process
- preserve runtime identity inside metrics via labels, not scrape-target process identity
- keep separate metrics listeners for direct runtime commands if needed

This will require dashboard updates, but it is cleaner than pretending one process is three scrape targets.

### Phase 5: Replace `RunCmd` subprocess supervision with in-process hosting

Once the above exists, generated `RunCmd` becomes:

- gather enabled runtimes
- build host
- run host with shared cancellation
- invoke root lifecycle in host mode rather than skipping it

No re-exec.

### Phase 6: Keep `forj dev` topology explicit

Do not automatically change `forj dev` in the same pass.

`forj dev` should remain free to preserve explicit runtime/process topology for parity and watcher behavior.

This lets GoForj support:

- `app run`: strongest standalone mode
- `forj dev`: realistic multi-process development mode

## Impacts By Area

### Logging

Need:

- runtime-scoped child loggers
- stable component field / prefix without subprocess env dependence

Likely touched files:

- [`templates/internal/logger/app.go.tmpl`](../../templates/internal/logger/app.go.tmpl)
- [`templates/internal/cmd/run_cmd.go.tmpl`](../../templates/internal/cmd/run_cmd.go.tmpl)

### Metrics

Need:

- host-mode metrics strategy decision
- likely collapse to one endpoint in `app run`
- preserve runtime identity in metric labels where needed
- observability docs and scrape config updates

Likely touched files:

- [`templates/internal/http/serve_cmd.go.tmpl`](../../templates/internal/http/serve_cmd.go.tmpl)
- [`templates/internal/jobs/worker_cmd.go.tmpl`](../../templates/internal/jobs/worker_cmd.go.tmpl)
- [`templates/internal/scheduler/cmd.go.tmpl`](../../templates/internal/scheduler/cmd.go.tmpl)
- [`templates/internal/metrics/endpoint.go.tmpl`](../../templates/internal/metrics/endpoint.go.tmpl)
- observability templates under [`templates/containers/observability/`](../../templates/containers/observability)

### Lighthouse

Need:

- multiple logical agents in one process
- no regression in source-based routing / command dispatch

Likely touched files:

- [`templates/internal/http/lighthouse.go.tmpl`](../../templates/internal/http/lighthouse.go.tmpl)
- [`templates/internal/scheduler/lighthouse.go.tmpl`](../../templates/internal/scheduler/lighthouse.go.tmpl)
- [`templates/internal/jobs/lighthouse.go.tmpl`](../../templates/internal/jobs/lighthouse.go.tmpl)
- [`templates/internal/lighthouse/agent.go.tmpl`](../../templates/internal/lighthouse/agent.go.tmpl)

### Runtime Wiring

Need:

- hostable runtime abstraction in `internal/app`
- `RunCmd` rewrite
- leaf commands become wrappers over reusable runtime services
- lifecycle skip policy rewrite in `wire.App.Run`

Likely touched files:

- [`templates/internal/cmd/run_cmd.go.tmpl`](../../templates/internal/cmd/run_cmd.go.tmpl)
- [`templates/internal/cmd/root_cmd.go.tmpl`](../../templates/internal/cmd/root_cmd.go.tmpl)
- [`templates/wire/app.go.tmpl`](../../templates/wire/app.go.tmpl)
- new files in `templates/internal/app/`

### Docs

Need:

- update runtime architecture docs
- update practical workflows
- explain difference between `app run` and `forj dev`

Likely touched files:

- [`docs/context/runtime-architecture.md`](../context/runtime-architecture.md)
- [`docs/context/practical-workflows.md`](../context/practical-workflows.md)

## Risks

### Risk: Collapse of runtime identity

If we move to one process without explicit runtime identity objects, we will blur:

- logs
- metrics
- Lighthouse agents
- operator expectations

This is the biggest product risk.

Mitigation:

- treat runtime identity as a first-class abstraction

### Risk: Duplicate startup/shutdown behavior

If both:

- app lifecycle
- runtime host
- leaf commands

try to own the same primitive startup, we will get double-starts or double-shutdowns.

Mitigation:

- define a single owner for each long-lived concern
- keep command wrappers thin
- rewrite lifecycle ownership deliberately rather than layering more `origin` conditionals on top

### Risk: Metrics topology churn

Dashboards and scrape config currently assume per-runtime listeners.

Mitigation:

- make the host-mode metrics model explicit early
- do not hack around it late in implementation

### Risk: `forj dev` semantics accidentally change

Users likely still want multi-process parity in `forj dev`.

Mitigation:

- scope this change to generated `app run`
- do not change `forj dev` in the same implementation slice

## Recommendation

Proceed with this design.

The cleanest version is:

- `app run` becomes single-process, multi-runtime
- runtime identity remains `api` / `scheduler` / `jobs`
- `forj dev` remains explicit multi-process supervision
- `inproc` stays honest and now behaves correctly in standalone mode

## Suggested Implementation Order

1. Introduce runtime identity + host abstractions in `internal/app`.
2. Refactor API, scheduler, and jobs command logic behind hostable runtime services.
3. Add runtime-scoped logger derivation.
4. Refactor Lighthouse startup to be runtime-scoped rather than process-scoped.
5. Decide and implement host-mode metrics topology.
6. Rewrite generated `RunCmd` to use the host instead of re-exec.
7. Update observability docs, runtime docs, and workflow docs.
8. Add rendered integration coverage for:
   - `app run` standalone startup
   - runtime-labeled logs
   - Lighthouse agent visibility for all runtimes
   - metrics exposure in host mode

## Acceptance Criteria

The implementation should not be considered complete until all of the following are true:

- `app run` no longer re-execs `http:serve`, `schedule:run`, or `queue:work`
- `app run` hosts enabled runtimes concurrently inside one OS process
- `inproc` events can cross API / scheduler / jobs boundaries when launched via `app run`
- direct `http:serve`, `schedule:run`, and `queue:work` remain valid and behaviorally intact
- logs still clearly identify `api`, `scheduler`, and `jobs`
- Lighthouse still shows `api`, `scheduler`, and `jobs` as distinct logical agents
- observability remains attributable by logical runtime identity
- `forj dev` remains multi-process unless changed by a separate design
- no shared primitive starts twice during boot or shutdown

## Test Plan To Add Later

- generator tests for rendered `RunCmd` shape
- rendered app build test for the new host runtime
- integration test proving:
  - one `app run` process boots all enabled runtimes
  - events with `inproc` cross runtime boundaries inside `app run`
  - Lighthouse still reports separate logical agents
  - metrics remain attributable by runtime identity
  - direct `http:serve`, `schedule:run`, and `queue:work` still work unchanged
