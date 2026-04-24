# GoForj Extensions Design

## Status
- Proposed
- Target: next minor release
- Scope: generated application architecture and package authoring

## Summary
GoForj should support reusable compile-time extension packages that an app owner declares in project config and installs with normal Go modules.

This is not runtime plugin loading.

The model should be:
- declare the extension in `.goforj.yml`
- install it with `go get`, or let `forj extension add` do that
- let GoForj generate the app-local registration glue
- let `wire` construct its dependencies
- let the app call its registration hooks for routes, commands, schedules, jobs, events, and lifecycle hooks

This fits GoForj’s current framework shape far better than dynamic plugins:
- thin `main`
- explicit registries
- compile-time DI
- no service locator
- no hidden runtime loading

## Problem Statement
Generated apps already have strong extension points:
- routes
- commands
- scheduler registry
- lifecycle registry
- queue/event packages

But today they are app-local. A reusable package author cannot cleanly publish a package that says:
- "install me and I will add routes"
- "install me and I will add queue handlers"
- "install me and I will add event subscribers"

The existing plugin sketch in this repo has one major flaw:
- it expects external packages to import generated app `internal/...` packages

That does not work for a reusable package, because:
- generated app `internal` packages are not importable from another module
- app module paths differ per project
- a published package cannot depend on one app’s generated internals

So the real design has to introduce a stable public contract package and keep app-specific glue inside the generated app.

## Goals
1. Let reusable Go packages contribute routes, jobs, event subscribers, schedules, lifecycle hooks, and commands.
2. Keep dependency wiring compile-time through `wire`.
3. Keep install/uninstall explicit and easy to audit.
4. Preserve GoForj’s existing package ownership lines and process boundaries.
5. Keep generated apps readable for humans and agents.
6. Give end users a clean product flow that does not require understanding generated glue, `wire`, or `module_replaces`.
7. Still work with normal Go modules and current `render.module_replaces` for framework and extension authors.

## Non-Goals
1. Runtime plugin discovery.
2. Downloading or loading untrusted code at runtime.
3. Reflection-based registration magic.
4. A global IoC container.
5. Replacing first-party `components` with third-party packages.

## Design Principles
1. Extensions are regular Go dependencies.
2. Construction happens in `wire`; registration happens in registries.
3. App owner remains in control of what is installed.
4. Framework-owned contracts must live in a public package, not generated app `internal/...`.
5. The generated app should have one obvious enable/disable surface.

## Terminology

### Component
A first-party GoForj render-time capability such as `Auth`, `WebAPI`, `Jobs`, or `Scheduler`.

### Extension
A reusable Go module that plugs into a rendered app through stable registration points.

This document recommends using `extension` rather than `plugin`, because the model is compile-time composition rather than runtime loading.

## Recommended Architecture

### 1. Introduce a public extension contract package
Add a reusable package such as:

```text
github.com/goforj/extensionsdk
```

It owns the stable contribution types and narrow registries.

This package should contain:
- route contribution types
- command contribution types
- queue consumer contribution types
- scheduler entry contribution types
- lifecycle hook contribution types
- event subscriber contribution types
- extension metadata types

It should not own app runtime behavior.

### 2. GoForj config owns user intent
The primary user-facing declaration should live in `.goforj.yml`.

Proposed shape:

```yaml
render:
  extensions:
    - module: github.com/acme/goforj-billing-ext
      package: billingext
    - module: github.com/acme/goforj-monitoring-ext
      package: monitoringext
```

Normal users should not need to think about:
- `wire`
- generated registration files
- local replace directives

They should declare intent in GoForj config and let GoForj own the glue.

### 3. Generated app owns the install point
Each rendered app gets:

```text
internal/extensions/
  install.go
  providers.go
```

These are generated files. They are not the primary user-facing install surface.

Responsibilities:

`internal/extensions/providers.go`
- aggregates extension `wire.ProviderSet`s

`internal/extensions/install.go`
- calls extension registration functions
- feeds contributions into app registries

This mirrors GoForj’s current architecture:
- composition stays explicit
- generated registries stay readable
- app-specific ownership stays local

### 4. App registries delegate to extension install
Generated registries become the stable app-side bridge.

Examples:
- routes registry calls `extensions.RegisterRoutes(...)`
- command registry calls `extensions.RegisterCommands(...)`
- scheduler registry calls `extensions.RegisterSchedules(...)`
- lifecycle registry calls `extensions.RegisterLifecycleHooks(...)`
- job/queue registry calls `extensions.RegisterConsumers(...)`
- events bootstrap calls `extensions.RegisterSubscribers(...)`

The extension package never talks directly to app internals.
The app adapts stable SDK contracts into its internal runtime.

## Public SDK Shape

### Metadata
```go
package extensionsdk

type Metadata struct {
	ID          string
	Name        string
	Description string
	Version     string
}
```

### Routes
```go
package extensionsdk

import "github.com/goforj/web"

type RouteContribution struct {
	Group      string // e.g. "public", "protected"
	RouteGroup web.RouteGroup
}

type RouteRegistry interface {
	AddRouteGroups(...RouteContribution) error
}
```

### Commands
```go
package extensionsdk

type CommandRegistry interface {
	AddCommands(...any) error
}
```

This stays intentionally narrow because GoForj command wiring is already explicit.

### Queue Consumers / Job Handlers
```go
package extensionsdk

import "github.com/goforj/queue"

type ConsumerContribution struct {
	QueueName string
	Consumer  queue.Consumer
}

type ConsumerRegistry interface {
	AddConsumers(...ConsumerContribution) error
}
```

### Scheduler
```go
package extensionsdk

type ScheduleRegistry interface {
	RegisterSchedules(...ScheduleContribution) error
}

type ScheduleContribution struct {
	Name      string
	Register  func() error
}
```

The SDK should not try to recreate the entire scheduler DSL.
Instead, the app-side scheduler adapter can expose a narrow registration callback.

### Lifecycle
```go
package extensionsdk

import "context"

type HookFn func(context.Context) error

type LifecycleRegistry interface {
	OnStartup(name string, fn HookFn) error
	OnShutdown(name string, fn HookFn) error
}
```

### Events
```go
package extensionsdk

import "context"

type EventRegistry interface {
	AddSubscribers(...SubscriberContribution) error
}

type SubscriberContribution struct {
	Name     string
	Register func(ctx context.Context) error
}
```

This reflects the real need:
- published packages often want to subscribe to app event buses
- event type definitions can live in the extension package itself

## Extension Package Shape
An extension package should export two things:
- a `ProviderSet`
- one or more registration functions

Example:

```go
package billingext

import (
	"github.com/google/wire"
	"github.com/goforj/extensionsdk"
)

var ProviderSet = wire.NewSet(
	NewService,
	NewController,
	NewSyncCustomersCommand,
	NewInvoiceProjectionConsumer,
	NewEventSubscriber,
)

type Deps struct {
	Service              *Service
	Controller           *Controller
	SyncCustomersCommand *SyncCustomersCommand
	InvoiceProjection    *InvoiceProjectionConsumer
	Subscriber           *EventSubscriber
}

func Metadata() extensionsdk.Metadata {
	return extensionsdk.Metadata{
		ID:          "acme.billing",
		Name:        "Billing",
		Description: "Billing routes, jobs, and projections.",
		Version:     "v0.1.0",
	}
}

func RegisterRoutes(r extensionsdk.RouteRegistry, d Deps) error {
	return r.AddRouteGroups(
		extensionsdk.RouteContribution{
			Group: "protected",
			RouteGroup: d.Controller.RouteGroup(),
		},
	)
}

func RegisterCommands(r extensionsdk.CommandRegistry, d Deps) error {
	return r.AddCommands(d.SyncCustomersCommand)
}

func RegisterConsumers(r extensionsdk.ConsumerRegistry, d Deps) error {
	return r.AddConsumers(
		extensionsdk.ConsumerContribution{
			QueueName: "default",
			Consumer:  d.InvoiceProjection,
		},
	)
}

func RegisterSubscribers(r extensionsdk.EventRegistry, d Deps) error {
	return r.AddSubscribers(
		extensionsdk.SubscriberContribution{
			Name: "billing.invoice_projection",
			Register: d.Subscriber.Register,
		},
	)
}
```

## Generated App Shape

### App install point
```text
internal/extensions/
  install.go
  providers.go
```

`providers.go`
```go
package extensions

import (
	"github.com/google/wire"
	"github.com/acme/goforj-billing-ext/billingext"
)

var ProviderSet = wire.NewSet(
	billingext.ProviderSet,
)
```

`install.go`
```go
package extensions

import (
	"github.com/acme/goforj-billing-ext/billingext"
	"github.com/goforj/extensionsdk"
)

type Deps struct {
	Billing billingext.Deps
}

func RegisterRoutes(r extensionsdk.RouteRegistry, d Deps) error {
	return billingext.RegisterRoutes(r, d.Billing)
}

func RegisterCommands(r extensionsdk.CommandRegistry, d Deps) error {
	return billingext.RegisterCommands(r, d.Billing)
}

func RegisterConsumers(r extensionsdk.ConsumerRegistry, d Deps) error {
	return billingext.RegisterConsumers(r, d.Billing)
}

func RegisterSubscribers(r extensionsdk.EventRegistry, d Deps) error {
	return billingext.RegisterSubscribers(r, d.Billing)
}
```

This is intentionally explicit in generated code.

It gives the app owner one place to:
- inspect what GoForj generated
- audit what is installed
- understand what each extension contributes

But the app owner should usually not hand-edit these files.
GoForj should generate them from `.goforj.yml`.

## Install Flow

### Primary end-user flow

```bash
forj extension add github.com/acme/goforj-billing-ext
```

Expected behavior:
1. run `go get github.com/acme/goforj-billing-ext`
2. add the extension to `.goforj.yml`
3. regenerate:
   - `internal/extensions/providers.go`
   - `internal/extensions/install.go`
4. run `wire generate`

Matching remove flow:

```bash
forj extension remove github.com/acme/goforj-billing-ext
```

### Config-driven flow

Users can also edit `.goforj.yml` directly:

```yaml
render:
  extensions:
    - module: github.com/acme/goforj-billing-ext
      package: billingext
```

Then run:

```bash
forj render
```

GoForj should then:
- ensure the module is present
- regenerate extension glue
- run `wire generate`

### Author/developer flow

For framework development, extension authoring, or local monorepo work, `render.module_replaces` remains valid.

That is an advanced workflow, not part of the main product story.

Example:

```yaml
render:
  module_replaces:
    github.com/acme/goforj-billing-ext: ../goforj-billing-ext
```

### Direct Go module flow

Advanced users can still install directly with:

```bash
go get github.com/acme/goforj-billing-ext@latest
```

But the intended product flow should stay config-first and CLI-assisted.

## How Each Capability Fits The Current Framework

### Routes
Best fit: app registry delegation.

Current route composition already happens centrally in:
- `internal/router/routes_registry.go`

Clean change:
- introduce an app-side route adapter implementing `extensionsdk.RouteRegistry`
- let `internal/router/routes_registry.go` append extension route groups after first-party routes

Policy:
- group must be explicit: `public` or `protected`
- collision policy should fail fast on duplicate method+path

### Commands
Best fit: command registry delegation.

Current command construction is already explicit through `wire` and command packages.

Clean change:
- introduce `internal/cmd/extensions.go`
- gather extension commands there
- let the existing command list append them

Policy:
- duplicate command names are an error

### Queue job handlers
Best fit: consumer registry, not worker struct growth.

Current worker shape directly injects specific jobs into `Worker`.
That does not scale for reusable packages.

Recommended framework change:
- add `internal/jobs/registry.go`
- `wire` constructs all extension consumers
- registry returns a flat list of `queue.Consumer`s or named `ConsumerContribution`s
- `Worker` consumes the registry output instead of accumulating one field per job

This is the most important runtime cleanup to make extensions viable.

### Events
Best fit: subscriber bootstrap registry.

Events are slightly different:
- publishers usually do not need registration
- subscribers do

Recommended change:
- app boot calls `extensions.RegisterSubscribers(...)`
- app-side event adapter opens subscriptions during startup
- shutdown closes them through lifecycle hooks

This keeps event subscriber lifecycle tied to the app runtime instead of hiding it in package init magic.

### Scheduler
Best fit: declarative schedule registry.

Current generated apps already have:
- `internal/scheduler/scheduler_registry.go`

Recommended change:
- keep schedule registration app-owned and declarative
- extension install point adds schedules through an adapter
- avoid raw cron string bags if possible; keep names explicit and first-class

### Lifecycle hooks
Best fit: direct extension of existing lifecycle registry.

This already aligns with:
- `internal/app/lifecycle_registry.go`

Recommended change:
- add extension hook registration after app-local hooks
- keep hook names explicit
- aggregate errors the same way current lifecycle does

## Collision And Validation Policy
Default behavior should fail fast.

### Commands
- duplicate command names: error

### Routes
- duplicate method+path in same group: error

### Queue consumers
- duplicate consumer registration with same semantic name: error

### Schedules
- duplicate schedule name: error

### Lifecycle hooks
- duplicate hook name in same phase: error

### Extension IDs
- duplicate extension metadata ID: error

## Configuration Model
Extension config should stay app-owned.

That means:
- extension packages do not own `.env` rendering in the first iteration
- extension packages read configuration from injected config/services
- if GoForj later adds extension install tooling, it can optionally scaffold config sections or env comments

This is cleaner than letting packages mutate app env templates arbitrarily.

## Database Migrations And Assets
This is the hardest open area.

Recommended first iteration:
- support routes, commands, schedules, jobs, events, and lifecycle
- do not try to solve package-owned migrations or frontend assets in the first pass

Why:
- migrations need ordering, discovery, and rerender policy
- frontend assets need starter-kit aware composition rules
- both are materially different problems from backend registration

Second iteration can add:
- extension-owned migration discovery
- extension asset publication rules
- extension UI modules for starter kits

## Why This Fits GoForj Cleanly
This design matches existing GoForj ownership lines:
- GoForj owns render-time structure
- generated app owns app composition
- sibling packages own reusable primitives
- external packages remain plain Go modules

It also fits the current framework model:
- routes already compose centrally
- scheduler already uses a registry
- lifecycle already has an explicit extension point
- `wire` already provides compile-time composition

And it gives a cleaner product story:
- users declare extensions in `.goforj.yml`
- GoForj generates the glue
- local replace workflows remain available, but secondary

## Recommended Rollout

### Phase 1: Make the runtime extension-ready
1. Add `extensionsdk` public package.
2. Add `render.extensions` to project config.
3. Add generated `internal/extensions/providers.go` and `install.go`.
4. Add route, command, scheduler, lifecycle delegation.

### Phase 2: Fix jobs/events for scalable composition
1. Add queue consumer registry in generated apps.
2. Add event subscriber bootstrap registry.
3. Stop modeling reusable jobs as fields on `Worker`.

### Phase 3: Tooling
1. Add docs for authoring an extension package.
2. Add `forj extension add`.
3. Add `forj extension remove`.
4. Add `forj extension list`.

### Phase 4: Harder surfaces
1. Migrations
2. Frontend assets / starter-kit UI modules
3. Lighthouse extension metadata

## Recommendation
Yes, GoForj should support extensions.

But the clean product model is:
- compile-time extension packages
- a public extension SDK
- `.goforj.yml` as the user-facing declaration surface
- GoForj-generated install glue
- registry delegation into the current runtime

Not runtime plugin loading.

If we follow that shape, a package can cleanly provide:
- routes
- commands
- queue consumers
- event subscribers
- scheduler entries
- lifecycle hooks

And it will fit the current framework model without turning GoForj into a dynamic plugin system.
