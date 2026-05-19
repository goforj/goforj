# Context-Scoped Runtime Access Design

## Goal

Reduce repetitive `WithContext(ctx)` usage at execution boundaries without
making context propagation implicit or magical.

Today, generated and template code often looks like:

```go
rows, err := c.monitorRepo.WithContext(ctx).EnabledOrderedByName()
if err != nil {
	return err
}

if err := c.events.Default().WithContext(ctx).Publish(payload); err != nil {
	return err
}

_ = cache.Set(c.caches.Sessions().WithContext(ctx), key, value, ttl)
```

This is correct, but verbose. It leaks a low-level binding primitive into
everyday controller/job/scheduler code.

The design goal is:

- bind execution context once at the boundary
- expose ctx-bound managers ergonomically
- keep explicit `ctx` in lower-level service/repository contracts

## Non-Goal

This is not a design for ambient global context.

GoForj should not:

- hide request context in thread-local or process-global state
- let arbitrary code pull context implicitly from nowhere
- replace explicit `context.Context` in service/repo method signatures
- flatten every dependency directly onto a generic transport context

The system should remain explicit about execution boundaries.

## Problem

`WithContext(ctx)` is currently used heavily across:

- caches
- databases/repositories
- queues
- events
- storage
- logger usage in some places

This has two costs:

1. boundary code is noisier than it needs to be
2. users are forced to understand the low-level context-binding pattern before
   they can do routine work

The right boundary code should feel closer to:

```go
func (c *Controller) MonitorStream(r http.Context) error {
	r.Logger().Info().Msg("starting stream")

	rows, err := r.Databases().Default().MonitorRepo().EnabledOrderedByName()
	if err != nil {
		return err
	}

	if err := r.Events().Default().Publish(payload); err != nil {
		return err
	}

	return nil
}
```

Instead of:

```go
func (c *Controller) MonitorStream(r http.Context) error {
	ctx := r.Context()

	c.logger.WithContext(ctx).Info().Msg("starting stream")

	rows, err := c.monitorRepo.WithContext(ctx).EnabledOrderedByName()
	if err != nil {
		return err
	}

	if err := c.events.Default().WithContext(ctx).Publish(payload); err != nil {
		return err
	}

	return nil
}
```

## Core Principle

Execution-bound ergonomic access should live on execution-bound objects.

That means:

- a richer GoForj-owned HTTP context for handlers
- job-scoped runtime accessor for queue handlers
- scheduler-scoped runtime accessor for scheduled runs
- CLI-scoped runtime accessor for commands

The context binding should happen once per execution boundary.

Below that line, services and repositories should still accept explicit
`context.Context`.

## Proposed Model

Introduce context-scoped runtime accessors that wrap existing managers.

### Boundary-Level Shape

For HTTP:

```go
type Context interface {
	web.Context

	Logger() *logger.AppLogger
	Caches() CacheScope
	Databases() DatabaseScope
	Queues() QueueScope
	Events() EventScope
	Storage() StorageScope
}
```

For non-HTTP boundaries, the exact carrier can vary, but the runtime accessor
surface should be parallel:

```go
type Scope interface {
	Context() context.Context
	Logger() *logger.AppLogger
	Caches() CacheScope
	Databases() DatabaseScope
	Queues() QueueScope
	Events() EventScope
	Storage() StorageScope
}
```

## Scope Types

These should be thin ctx-bound views over existing managers.

Example:

```go
type CacheScope interface {
	Default() cache.Store
	Sessions() cache.Store
	Settings() cache.Store
}

type cacheScope struct {
	ctx   context.Context
	inner *caches.Manager
}

func (s *cacheScope) Sessions() cache.Store {
	return s.inner.Sessions().WithContext(s.ctx)
}
```

Equivalent wrappers should exist for:

- caches
- database connections / repository managers
- queues
- event bus
- storage disks

The key constraint is that these wrappers must remain very thin. They are not a
second behavior layer. They are ergonomic binding helpers.

## Boundary Context Guardrails

This design is only clean if the boundary context has a hard stopping rule.

The boundary object must not become:

- a second service container
- a business-logic bucket
- a convenience surface for hidden DB/network work

The boundary context may expose only:

1. execution-scoped infrastructure access
2. already-resolved request/execution state
3. passive metadata about the current execution

That rule should be treated as a design constraint, not a suggestion.

### Allowed on the Boundary Context

Examples that fit the intended contract:

- `r.Logger()`
- `r.Caches()`
- `r.Databases()`
- `r.Queues()`
- `r.Events()`
- `r.Storage()`
- `r.GetUser()`
- `r.GetSession()`
- `r.InspectID()`
- `r.TraceID()`
- `r.TenantID()` if tenant was already resolved earlier in middleware
- `r.FeatureFlags()` if it exposes already-resolved passive state

These are acceptable because they expose execution-local access or resolved
state. They do not perform domain work.

### Not Allowed on the Boundary Context

Examples that should be explicitly rejected:

- `r.Services()`
- `r.App()`
- `r.Container()`
- `r.Resolve(...)`
- `r.CreateMonitor()`
- `r.SendEmail()`
- `r.LoadUser()`
- `r.RequireTenant()` if it triggers lookup work
- `r.Policy()` if it becomes a gateway for business decisions

These turn the boundary context into either:

- a service locator
- an implicit loader
- a business workflow surface

That is exactly what this design should avoid.

## Service Contract Rule

Lower-layer services and repositories must not depend on boundary context types.

They should continue to accept explicit inputs:

- `context.Context`
- actor/session metadata when needed
- concrete request payloads or identifiers

Example:

```go
func (s *MonitorService) Create(ctx context.Context, actor *auth.User, req CreateMonitorRequest) error
```

Then handler code can stay ergonomic without leaking the boundary object
downward:

```go
func (c *Controller) CreateMonitor(r http.Context) error {
	actor := r.GetUser()
	return c.monitorService.Create(r.Context(), actor, req)
}
```

This keeps:

- transport independence
- testability
- explicit contracts
- a clear separation between boundary and domain layers

## HTTP-Specific Recommendation

The preferred near-term shape is to keep `github.com/goforj/web` unchanged and
introduce a richer GoForj-owned request context in `internal/http`.

Handlers should receive:

```go
func (c *Controller) MonitorStream(r http.Context) error
```

while the generated route wiring continues to adapt from the lower-level
transport abstraction:

```go
func wrapContext(r web.Context, deps ...) http.Context
```

This keeps:

- `web.Context` in the adapter/wiring layer
- `http.Context` as the user-facing boundary object

That is simpler for users than forcing a `Scope(...)` constructor into every
handler body.

Recommended shape:

```go
r.Logger().Info().Msg("...")
r.Caches().Sessions()
r.Databases().Default()
r.Queues().Default()
r.Events().Default()
r.Storage().Favicons()
```

This is terse without turning `r` into a junk drawer of one-off accessors like:

- `r.DefaultDB()`
- `r.SessionsCache()`
- `r.DefaultQueue()`

Grouped access is the right middle ground.

This also avoids a naming collision with the upstream `web` library. A local
package named `web` would be awkward. A richer `internal/http.Context` is the
cleaner app-facing surface for now.

## Non-HTTP Recommendation

For jobs, scheduler, and CLI, the same idea should exist, but not necessarily
through `web.Context`.

Recommended user-facing shape:

```go
func (j *MonitorCheckJob) Handle(ctx context.Context, payload Payload) error {
	r := j.Scope(ctx)

	r.Logger().Info().Str("job", "monitoring:check").Msg("start")

	if err := r.Events().Default().Publish(payload); err != nil {
		return err
	}

	return nil
}
```

or:

```go
scope := app.Scope(ctx)
```

The important thing is API parity, not the exact constructor name.

## Keep Explicit Context In Lower Layers

This design should stop at the execution boundary.

Services should still look like:

```go
func (s *Service) Reconcile(ctx context.Context, monitorID string) error
```

Repositories should still look like:

```go
func (r *MonitorRepo) EnabledOrderedByName(ctx context.Context) ([]Monitor, error)
```

or continue using their current `WithContext(ctx)` internals if that is still
the chosen repository contract.

Reason:

- explicit cancellation remains visible
- tests stay simple
- lower layers do not become coupled to HTTP or job wrappers
- background work and nested operations remain honest

Services that need request-derived state should receive it explicitly rather
than reaching back into the boundary object.

## Why Not Make Everything Ambient

The tempting alternative is to make calls like:

```go
app.Caches().Sessions()
app.Databases().Default()
```

implicitly pick up “the current context”.

That is the wrong trade.

It introduces hidden execution state, makes concurrency behavior harder to
reason about, and turns tests into inference games.

GoForj should prefer:

- explicit boundary-local scope
- explicit lower-layer `ctx`

over:

- implicit global context

## Why Not Flatten Everything Onto `r`

This is also the wrong trade:

```go
r.DefaultDB()
r.DefaultCache()
r.DefaultQueue()
r.FaviconsDisk()
```

It does not scale well as capabilities grow.

Grouped managers keep the API legible:

- `r.Caches().Sessions()`
- `r.Storage().Favicons()`
- `r.Events().Default()`

That structure reflects ownership clearly.

Just as important, grouped managers make it easier to reject container-like
escape hatches such as `r.Services()` or `r.App()`.

## Rollout Strategy

### Phase 1

Add ctx-bound scoped accessors without removing existing APIs.

That means:

- keep `WithContext(ctx)` intact
- add scope wrappers over existing managers
- expose them on a GoForj-owned `internal/http.Context`
- keep `web.Context` unchanged
- expose equivalent scope access for jobs/scheduler/CLI

### Phase 2

Switch generated boundary code to the new ergonomic shape.

Targets:

- HTTP controllers
- middleware where appropriate
- queue jobs
- scheduler-owned entrypoints
- generated commands

### Phase 3

Evaluate whether repositories should move from `repo.WithContext(ctx).X()`
toward methods that take `ctx` explicitly.

This is intentionally not required for the ergonomic scope design itself.

It can be decided independently.

## Open Questions

### 1. Should the richer `internal/http.Context` directly satisfy `context.Context`?

There are two reasonable shapes:

1. embed only `web.Context`
2. also satisfy `context.Context` directly

Direct `context.Context` compatibility can make some call sites terser, but it
also makes the richer boundary type heavier.

Current lean is:

- keep `web.Context` unchanged
- let `internal/http.Context` expose `Context() context.Context` through the
  embedded transport abstraction unless there is a strong reason to go further

### 2. Should repositories remain `WithContext(ctx)` based?

This design does not require an immediate answer.

The ergonomic win comes from hiding `WithContext(ctx)` at the boundary, not from
forcing every repository API to change immediately.

Still, the longer-term repo shape may be cleaner as:

```go
repo.EnabledOrderedByName(ctx)
```

instead of:

```go
repo.WithContext(ctx).EnabledOrderedByName()
```

That should be evaluated separately from the boundary scope work.

### 3. Where do these scoped wrappers live?

Likely ownership:

- `web.Context` stays in the `web` abstraction layer
- HTTP-facing ergonomic accessors live in GoForj `internal/http`
- ctx-bound manager wrappers likely belong in GoForj runtime packages unless
  they become broadly reusable library-level primitives

This follows the current web boundary guidance:

- reusable transport abstraction in `web`
- app/runtime composition and request ergonomics in GoForj

## Recommendation

Adopt boundary-local context-scoped runtime access.

Specifically:

- add grouped ctx-bound accessors to a richer GoForj-owned `internal/http.Context`
- add parallel scope access for jobs, scheduler, and CLI
- keep explicit `context.Context` in lower-layer service/repo contracts
- keep `WithContext(ctx)` as the low-level primitive during rollout
- do not introduce ambient global context
- explicitly ban service-container and implicit-loader patterns on boundary
  contexts

This gives the ergonomic win where developers actually feel the pain while
preserving explicitness where it still matters.

## Success Condition

Normal generated boundary code should read like this:

```go
func (c *Controller) Monitors(r http.Context) error {
	r.Logger().Info().Msg("listing monitors")

	monitors, err := r.Databases().Default().MonitorRepo().EnabledOrderedByName()
	if err != nil {
		return err
	}

	if err := r.Events().Default().Publish(payload); err != nil {
		return err
	}

	return r.JSON(http.StatusOK, monitors)
}
```

and not force users to keep rebinding ctx into every collaborator manually.
