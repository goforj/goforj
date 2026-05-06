# CLI Lazy Infrastructure Initialization Design

## Problem

Today too much of the generated app assumes that infrastructure should be
constructed eagerly at process startup.

That creates bad command behavior:

- a CLI command that only wants to print config or scaffold code can still fail
  because MySQL is down
- a command that only needs local filesystem access can still try to connect to
  Redis
- boot wiring can force queues, event buses, caches, or storage drivers to
  initialize long before they are actually needed
- command authors have to understand too much about which dependencies are safe
  to touch at startup

This makes the CLI feel fragile and increases the chance that generated apps are
hard-coupled to infrastructure they do not need for a given execution path.

## Goals

- allow commands to start and run without hard-requiring unrelated
  infrastructure
- keep generated command code simple
- preserve fail-fast behavior once a command actually uses a dependency
- make lazy behavior framework-owned, not something each command author must
  reinvent
- support the same model for future `http`, `jobs`, and `scheduler` startup
  improvements where appropriate

## Non-Goals

- do not hide real dependency failures once a command actually needs the
  dependency
- do not convert every dependency into an ad hoc service locator
- do not make lazy loading so magical that it becomes hard to reason about
- do not weaken readiness semantics for runtimes that genuinely require their
  infrastructure at startup

## Current Failure Mode

The main issue is not just command execution. It is the shape of generated app
construction.

If `wire.App` or a manager constructor eagerly does work like:

- opening a DB connection
- dialing Redis
- pinging a queue backend
- creating a storage client that validates credentials immediately
- subscribing to buses during construction

then commands that never use those facilities still inherit those failures.

That means CLI behavior is effectively coupled to the entire app graph.

## Desired Model

The framework should separate:

- constructing a handle
- first use of the underlying infrastructure

The normal path should be:

1. command starts
2. app wiring constructs lightweight managers/providers
3. command executes
4. actual infrastructure initializes only when the command touches it

This means:

- `make:model` should not need MySQL
- `route:list` should not need Redis
- `test:render` should not need queue workers
- `monitor:retention` should only fail on DB/cache dependencies when it truly
  uses them

## High-Level Approach

Prefer lazy handles over eager clients.

Instead of:

- build and connect everything in constructors

use:

- lightweight manager/provider objects
- per-resource lazy initialization behind those providers
- explicit runtime startup hooks for components that truly need eager ownership

## Design Principles

### 1. Constructors should be cheap

Constructors should mostly:

- capture config
- capture factories
- capture logger/metrics/source context helpers
- prepare synchronization primitives

They should not:

- dial remote systems
- ping external services
- subscribe to remote consumers
- create network state that is only relevant on later usage

### 2. First use should initialize

If a command calls:

- `db.Users()`
- `queues.Dispatch(...)`
- `events.Publish(...)`
- `storage.Default().Put(...)`

then the framework may initialize the underlying dependency at that point.

That is the correct failure boundary.

### 3. Runtime startup remains explicit

Some runtimes do need eager behavior:

- HTTP server startup
- queue worker startup
- scheduler startup
- long-lived subscriber boot

Those should still explicitly initialize the dependencies they own.

Lazy CLI startup should not be confused with runtime readiness behavior.

### 4. Optionality must be modeled explicitly

If a dependency is required for a command or runtime, it should fail when used.
If it is not required, it should not be touched.

We should not sprinkle defensive nil checks to fake optionality.

## Framework Shape

## Lightweight Manager Pattern

Managers should hold lazy resource slots.

Conceptual shape:

```go
type Manager struct {
	config Config

	dbOnce sync.Once
	db     *gorm.DB
	dbErr  error
}

func (m *Manager) DB() (*gorm.DB, error) {
	m.dbOnce.Do(func() {
		m.db, m.dbErr = openDB(m.config)
	})
	return m.db, m.dbErr
}
```

That same pattern applies to:

- cache managers
- queue managers
- event bus managers
- storage managers
- mail providers

## Factory-Backed Resource Access

Generated code should prefer storing factories instead of concrete live clients.

Conceptually:

```go
type storageFactory func() (storage.Storage, error)

type managedStorage struct {
	once    sync.Once
	factory storageFactory
	inner   storage.Storage
	err     error
}

func (s *managedStorage) resolve() (storage.Storage, error) {
	s.once.Do(func() {
		s.inner, s.err = s.factory()
	})
	return s.inner, s.err
}
```

Then all operations go through `resolve()`.

## Runtime-Owned Eager Start

Long-lived runtimes should still have explicit startup phases that touch what
they need.

Examples:

- HTTP runtime may initialize router, middleware, listener, and any
  truly-required app services
- jobs runtime may initialize worker backends eagerly
- scheduler runtime may initialize lock providers and schedule execution state

This keeps readiness semantics honest while allowing CLI commands to stay light.

## Suggested Code Shape

## App Wiring

`wire.App` should prefer holding managers/providers rather than fully-live
resources.

For example:

```go
type App struct {
	databases *databases.Manager
	caches    *caches.Manager
	queues    *queues.Manager
	events    *events.Manager
	storages  *storages.Manager
}
```

Those managers should be safe to construct without opening everything.

## Command Execution

Commands should continue to receive `ctx` from the app runner.

They should not need to think about lazy loading directly.

Good:

```go
func (c *RetentionCmd) Run(ctx context.Context) error {
	return c.retentionService.Run(ctx, opts)
}
```

Bad:

```go
func (c *RetentionCmd) Run(ctx context.Context) error {
	if err := c.databases.ConnectNow(); err != nil {
		return err
	}
	...
}
```

The service/repo/manager path should naturally initialize what it needs.

## Repositories and Services

Repositories should depend on manager/provider accessors rather than assuming the
underlying primitive is already hot.

Good:

```go
func (r *UserRepo) db() (*gorm.DB, error) {
	return r.databases.Default()
}
```

Then methods call `db()` on demand.

## Where Eager Initialization Is Still Correct

Eager startup is still appropriate for:

- worker runtimes that must register and poll immediately
- scheduler processes that must acquire locks and tick immediately
- HTTP serve commands that must fail if the listener cannot start
- startup/preseed flows that intentionally validate a required dependency

But even there, eager startup should happen in runtime-owned start paths, not in
plain object construction.

## Startup Classification

We should think about infrastructure in three buckets.

### Bucket 1: Safe lazy by default

- cache clients
- storage clients
- event publish clients
- DB handles for repos not touched by the current command
- queue dispatch handles

### Bucket 2: Runtime-owned eager

- queue workers
- scheduler lock/tick loop
- HTTP listener startup
- long-lived subscriptions

### Bucket 3: Explicit bootstrap/setup

- migrations
- preseeding
- startup checks
- readiness probes

These should be intentional operations, not side effects of plain construction.

## Risks

### Hidden latency on first use

Lazy initialization moves some cost from startup to first use.

That is acceptable for CLI commands and many request paths, but it should be
visible.

Mitigations:

- emit structured logs/metrics on first initialization
- make first-use failures clear
- optionally warm selected dependencies in runtimes that care

### Concurrency races

Lazy resources must be concurrency-safe.

Use:

- `sync.Once`
- `sync.Mutex` where needed
- explicit cached error storage

### Error caching semantics

If first initialization fails, the framework must decide whether that error is
sticky or retryable.

Default recommendation:

- sticky within the current manager instance

This keeps behavior deterministic. If we want retries, that should be explicit.

### Over-laziness

If everything becomes deeply lazy, debugging can become harder.

So the standard should be:

- lazy at manager/provider/resource boundaries
- not lazy at every tiny helper layer

## Observability Considerations

Lazy initialization should still preserve:

- `source`
- trace/span context
- execution recorder context when present

If a command first touches Redis under `source=cli`, the first-use
initialization logs/metrics should also show `source=cli`.

This means lazy initialization should happen inside context-aware operations when
possible, not only at process-global startup.

## Readiness Considerations

CLI and runtime readiness are different concerns.

We should not require:

- DB connectivity to run `make:*`
- Redis connectivity to run `route:list`

But we should still require:

- queue backend connectivity for `queue:work`
- scheduler lock backend connectivity for `schedule:run`
- listener bind success for `http:serve`

This suggests two separate questions:

- can the process start?
- is this runtime/command actually ready for its owned behavior?

## Recommended Incremental Plan

### Phase 1

Refactor generated managers so construction is cheap and resource access is lazy.

Targets:

- databases
- caches
- storages
- events publish clients
- queue dispatch clients

### Phase 2

Move eager startup behavior out of constructors and into runtime start methods.

Targets:

- jobs runtime
- scheduler runtime
- HTTP runtime

### Phase 3

Add targeted tests for CLI startup independence.

Examples:

- `route:list` should run with MySQL down
- `make:model` should run with Redis down
- `test:render` should run without queue backend
- `queue:work` should fail when queue backend is unavailable

### Phase 4

Add optional explicit warming hooks for commands/runtimes that want predictable
first-use behavior.

## Open Questions

- Should failed lazy initialization be retried automatically or cached per
  process?
- Should managers expose explicit `Warm(ctx)` methods for runtimes that want
  eager startup without changing lazy defaults?
- Should some drivers remain eager because their client construction is itself
  trivial and side-effect free?
- How much startup/preseed logic should be moved off of plain app construction?

## Recommendation

The framework should adopt this default rule:

- app construction should be cheap
- runtime start should eagerly initialize only what that runtime owns
- CLI commands should initialize dependencies only on first real use

That is the cleanest model for avoiding unnecessary hard requirements on MySQL,
Redis, and similar infrastructure while preserving honest failure behavior where
it actually matters.
