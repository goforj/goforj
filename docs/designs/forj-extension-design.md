# GoForj Extensions Design

## Status
- Proposed
- Target: next minor release
- Scope: generated application architecture and package authoring

## Summary
GoForj should support reusable compile-time extension packages that are installed as normal Go modules and synced into the generated app through GoForj-owned glue.

This is not runtime plugin loading.

This is also not a render-time construct. `render` is for initial project creation and occasional structural regeneration. Extensions are an app/runtime composition feature that can be added after a project already exists.

The model should be:
- install an extension as a normal Go module
- let the extension define its contract in Go code
- let GoForj read that contract during `forj generate`
- let GoForj generate the app-local `wire` plumbing, named primitive accessors, and central extension registry glue
- let `wire` construct dependencies
- let the app provide typed hook objects for app-specific behavior such as middleware, wrappers, and policy

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
- named primitive accessors such as caches, storage, queues, and events

But today they are app-local. A reusable package author cannot cleanly publish a package that says:
- "install me and I will add routes"
- "install me and I will add queue handlers"
- "install me and I will add event subscribers"
- "install me and I need a cache instance for sessions"

The existing plugin sketch in this repo had two major flaws:
- it treated extensions too much like a render/config concern
- it expected external packages to import generated app `internal/...` packages

That does not work for a reusable package, because:
- generated app `internal` packages are not importable from another module
- app module paths differ per project
- a published package cannot depend on one app’s generated internals

So the real design has to introduce a stable public contract package, keep app-specific glue inside the generated app, and preserve GoForj’s existing primitive model cleanly.

## Goals
1. Let reusable Go packages contribute routes, jobs, event subscribers, schedules, lifecycle hooks, and commands.
2. Let extensions declare primitive resource needs such as caches, storage instances, queues, and event buses.
3. Keep dependency wiring compile-time through `wire`.
4. Keep install/uninstall explicit and easy to audit.
5. Preserve GoForj’s existing package ownership lines and process boundaries.
6. Keep generated apps readable for humans and agents.
7. Keep `.env` as the runtime configuration source, with user values always taking precedence.
8. Let GoForj optionally publish extension env defaults into the user’s `.env` without making that mandatory.

## Non-Goals
1. Runtime plugin discovery.
2. Downloading or loading untrusted code at runtime.
3. Reflection-based registration magic.
4. A global IoC container.
5. Replacing first-party `components` with third-party packages.
6. Using `.goforj.yml` as the source of truth for extension resource definitions.

## Design Principles
1. Extensions are regular Go dependencies.
2. Construction happens in `wire`; registration happens through one central extensions registry.
3. App owner remains in control of what is installed.
4. Framework-owned contracts must live in a public package, not generated app `internal/...`.
5. Extension needs should be declared in code from the extension package.
6. User `.env` should always override extension defaults.
7. Publishing env defaults into the user’s `.env` should be optional.
8. Resource kinds should be typed, not stringly-typed.

## Terminology

### Component
A first-party GoForj render-time capability such as `Auth`, `WebAPI`, `Jobs`, or `Scheduler`.

### Extension
A reusable Go module that plugs into a generated app through stable registration points and typed resource contracts.

This document recommends using `extension` rather than `plugin`, because the model is compile-time composition rather than runtime loading.

## Recommended Architecture

### 1. Introduce a public extension contract package
Add a reusable package such as:

```text
github.com/goforj/extension
```

This package owns the stable contribution types and typed resource contract.

It should contain:
- manifest types
- metadata types
- typed resource kinds
- route contribution types and hook types
- command contribution types
- queue consumer contribution types
- scheduler entry contribution types
- lifecycle hook contribution types
- event subscriber contribution types
- env default/spec types

It should not own app runtime behavior.

### 2. The extension package owns its manifest in code
The extension package should export a manifest function. That manifest is the source of truth for:
- extension identity
- resource requirements
- extension env defaults
- contribution surface metadata

The manifest should live in the extension module itself, versioned with the extension code. If the manifest changes, that is just a normal package version update.

Example:

```go
package authsessionsext

import "github.com/goforj/extension"

func Manifest() extension.Manifest {
	return extension.Manifest{
		ID:   "auth_sessions",
		Name: "Auth Sessions",
		Resources: []extension.ResourceSpec{
			{
				Kind: extension.ResourceKindCache,
				Name: "sessions",
				Env: extension.EnvDefaults{
					"DRIVER": "redis",
					"TTL":    "720h",
				},
			},
			{
				Kind: extension.ResourceKindEvents,
				Name: "audit",
				Env: extension.EnvDefaults{
					"DRIVER": "inproc",
				},
			},
		},
		Env: extension.EnvDefaults{
			"AUTH_SESSION_COOKIE_NAME": "session",
			"AUTH_SESSION_IDLE_TTL":    "2h",
		},
	}
}
```

### 3. GoForj owns generated `wire` plumbing
Generated apps already compose dependencies through top-level `./wire`, so extension plumbing should live there too.

Recommended generated files:

```text
wire/
  extensions_gen.go
  inject_extensions.go
```

Responsibilities:

`wire/extensions_gen.go`
- aggregates extension `wire.ProviderSet`s
- provides generated adapters from app primitive managers into extension `Resources`
- provides typed hook objects for extension registration callbacks

`wire/inject_extensions.go`
- owns the aggregate extension `wire` set
- constructs the central extensions registry

This mirrors the generated app shape that already exists today:
- composition stays explicit
- app-specific ownership stays local
- extension plumbing follows the same provider pattern as caches, queues, storage, auth, and jobs

### 4. The app owns one central extensions registry
We should not sprinkle extension registration logic across router, jobs, scheduler, lifecycle, and events independently.

Instead, the app should construct one central registry of extension contributions, and the existing subsystems should consume from that.

Example shape:

```go
type Registry struct {
	Routes      []extension.RouteContribution
	Commands    []any
	Consumers   []extension.ConsumerContribution
	Subscribers []extension.SubscriberContribution
	Schedules   []extension.ScheduleContribution
	Lifecycle   []LifecycleHookContribution
}
```

Generated app code should:
- build extension instances in `wire`
- call each extension’s registration callbacks once
- collect all contributions into one registry
- let router/jobs/events/scheduler/lifecycle consume from that registry

The extension package never talks directly to app internals.
The app adapts stable `extension` contracts into its internal runtime.

## Public Contract Shape

### Metadata and Manifest
```go
package extension

type Manifest struct {
	ID        string
	Name      string
	Resources []ResourceSpec
	Env       EnvDefaults
}

type EnvDefaults map[string]string
```

### Typed resource kinds
```go
package extension

type ResourceKind uint8

const (
	ResourceKindCache ResourceKind = iota + 1
	ResourceKindStorage
	ResourceKindQueue
	ResourceKindEvents
)

type ResourceSpec struct {
	Kind ResourceKind
	Name string
	Env  EnvDefaults
}
```

This avoids string keys like `"cache"` or `"events"` in extension code.

### Routes
```go
package extension

import "net/http"

type RouteContribution struct {
	Method  string
	Path    string
	Handler http.Handler
}

type RouteRegistrar interface {
	AddRoutes(...RouteContribution) error
}
```

Extensions should not declare string mount points like `"public"` or `"protected"`.

Instead, the extension should define typed hook structs for app-owned policy.

Example:

```go
package extension

import "net/http"

type RouteHooks struct {
	Wrap func(http.Handler) http.Handler
}
```

### Commands
```go
package extension

type CommandRegistry interface {
	AddCommands(...any) error
}
```

### Queue consumers / job handlers
```go
package extension

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
package extension

type ScheduleRegistry interface {
	RegisterSchedules(...ScheduleContribution) error
}

type ScheduleContribution struct {
	Name     string
	Register func() error
}
```

### Lifecycle
```go
package extension

import "context"

type HookFn func(context.Context) error

type LifecycleRegistry interface {
	OnStartup(name string, fn HookFn) error
	OnShutdown(name string, fn HookFn) error
}
```

### Events
```go
package extension

import "context"

type EventRegistry interface {
	AddSubscribers(...SubscriberContribution) error
}

type SubscriberContribution struct {
	Name     string
	Register func(ctx context.Context) error
}
```

## Resource Naming And Env Model

### Deterministic named primitive instances
Because resource requirements are defined in the extension manifest, GoForj can derive stable instance names from:
- extension ID
- resource kind
- resource name

Example:
- extension ID: `auth_sessions`
- resource: `cache:sessions`

Derived env prefix:

```text
CACHE_AUTH_SESSIONS_SESSIONS_
```

Example effective envs:

```env
CACHE_AUTH_SESSIONS_SESSIONS_DRIVER=redis
CACHE_AUTH_SESSIONS_SESSIONS_TTL=720h
EVENTS_AUTH_SESSIONS_AUDIT_DRIVER=inproc
AUTH_SESSION_COOKIE_NAME=session
AUTH_SESSION_IDLE_TTL=2h
```

This keeps extension-backed infrastructure aligned with GoForj’s normal env model. There is no special `EXT_` namespace requirement.

### Effective config resolution
During `forj generate`, GoForj should resolve configuration in this order:

1. explicit values in user `.env`
2. extension manifest defaults
3. primitive/framework fallback defaults if still needed

User `.env` always wins.

This means an extension can work out of the box with its own defaults, while still allowing the app owner to override anything with ordinary env values.

### Optional env publishing
GoForj should support publishing extension defaults into the user’s `.env`, but it should not require that step.

Example commands:

```bash
forj extension env:publish auth_sessions
```

or:

```bash
forj extension add github.com/acme/goforj-auth-sessions-ext --publish-env
```

Publishing behavior:
- write missing env defaults into `.env`
- do not overwrite existing user values
- group the entries with clear comments

Without publishing:
- GoForj still uses the extension manifest defaults
- generate still knows what accessors and glue to emit

## Extension Package Shape
An extension package should export:
- a `Manifest`
- a `ProviderSet`
- one or more registration functions

Example:

```go
package billingext

import (
	"github.com/google/wire"
	"github.com/goforj/extension"
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

func Manifest() extension.Manifest {
	return extension.Manifest{
		ID:   "billing",
		Name: "Billing",
		Resources: []extension.ResourceSpec{
			{
				Kind: extension.ResourceKindQueue,
				Name: "projection",
				Env: extension.EnvDefaults{
					"DRIVER": "asynq",
				},
			},
			{
				Kind: extension.ResourceKindEvents,
				Name: "audit",
				Env: extension.EnvDefaults{
					"DRIVER": "inproc",
				},
			},
		},
	}
}

type Resources struct {
	ProjectionQueue *queue.Queue
	AuditBus        events.Bus
}

type RouteHooks struct {
	Wrap func(http.Handler) http.Handler
}

func RegisterRoutes(r extension.RouteRegistrar, d Deps, hooks RouteHooks) error {
	wrap := hooks.Wrap
	if wrap == nil {
		wrap = func(next http.Handler) http.Handler { return next }
	}

	return r.AddRoutes(
		extension.RouteContribution{
			Method:  "GET",
			Path:    "/billing/invoices",
			Handler: wrap(http.HandlerFunc(d.Controller.Index)),
		},
		extension.RouteContribution{
			Method:  "POST",
			Path:    "/billing/invoices/sync",
			Handler: wrap(http.HandlerFunc(d.Controller.Sync)),
		},
	)
}

func RegisterCommands(r extension.CommandRegistry, d Deps) error {
	return r.AddCommands(d.SyncCustomersCommand)
}

func RegisterConsumers(r extension.ConsumerRegistry, d Deps) error {
	return r.AddConsumers(
		extension.ConsumerContribution{
			QueueName: "default",
			Consumer:  d.InvoiceProjection,
		},
	)
}

func RegisterSubscribers(r extension.EventRegistry, d Deps) error {
	return r.AddSubscribers(
		extension.SubscriberContribution{
			Name:     "billing.invoice_projection",
			Register: d.Subscriber.Register,
		},
	)
}
```

## Generated App Shape

### `wire` bridge providers
```go
package wire

import (
	"github.com/acme/goforj-billing-ext/billingext"
	"github.com/google/wire"
)

var extensionSet = wire.NewSet(
	billingext.ProviderSet,
	provideBillingResources,
	provideBillingRouteHooks,
)
```

### Generated resource adapter
The extension should receive typed resources. It should not import app `internal/...` packages directly.

Example:

```go
func provideBillingResources(
	queues *queues.Manager,
	events *events.Manager,
) billingext.Resources {
	return billingext.Resources{
		ProjectionQueue: queues.BillingProjection(),
		AuditBus:        events.BillingAudit(),
	}
}
```

### App-owned hook provider in `wire`
```go
package wire

import (
	"github.com/acme/goforj-billing-ext/billingext"
)

func provideBillingRouteHooks(
	authMw *middleware.Auth,
	orgMw *middleware.RequireOrg,
) billingext.RouteHooks {
	return billingext.RouteHooks{
		Wrap: func(next http.Handler) http.Handler {
			return authMw.Handle(orgMw.Handle(next))
		},
	}
}
```

### Central extensions registry construction
```go
package wire

func provideExtensionsRegistry(
	billingDeps billingext.Deps,
	billingRouteHooks billingext.RouteHooks,
) (*app.ExtensionsRegistry, error) {
	reg := app.NewExtensionsRegistry()

	if err := billingext.RegisterRoutes(reg.Router(), billingDeps, billingRouteHooks); err != nil {
		return nil, err
	}
	if err := billingext.RegisterCommands(reg.Commands(), billingDeps); err != nil {
		return nil, err
	}
	if err := billingext.RegisterConsumers(reg.Consumers(), billingDeps); err != nil {
		return nil, err
	}
	if err := billingext.RegisterSubscribers(reg.Subscribers(), billingDeps); err != nil {
		return nil, err
	}

	return reg, nil
}
```

This keeps the plumbing explicit:
- resource adapters come from app primitive managers
- app-specific hooks come from `wire`
- extensions register once into a central registry
- the rest of the app consumes that registry

## Install Flow

### Primary end-user flow
```bash
forj extension add github.com/acme/goforj-billing-ext
```

Expected behavior:
1. run `go get github.com/acme/goforj-billing-ext`
2. discover the extension manifest from code
3. regenerate:
   - `wire/extensions_gen.go`
   - `wire/inject_extensions.go`
   - any named primitive accessors implied by extension resources
4. run `wire generate`

Optional env publishing:

```bash
forj extension add github.com/acme/goforj-billing-ext --publish-env
```

Matching remove flow:

```bash
forj extension remove github.com/acme/goforj-billing-ext
```

### Author/developer flow
For framework development, extension authoring, or local monorepo work, local module replaces remain valid.

That is an advanced workflow, not part of the main product story.

### Direct Go module flow
Advanced users can still install directly with:

```bash
go get github.com/acme/goforj-billing-ext@latest
```

Then run:

```bash
forj generate
```

GoForj should read the installed extension manifests, resolve env/defaults, and regenerate glue.

## How Each Capability Fits The Current Framework

### Routes
Best fit: route contributions collected in the central extensions registry.

Current route composition already happens centrally in:
- `internal/router/routes_registry.go`

Clean change:
- introduce an app-side adapter implementing `extension.RouteRegistrar`
- let extensions receive app-owned route hooks from `wire`
- let `internal/router/routes_registry.go` consume route contributions from the central extensions registry

Policy:
- collision policy should fail fast on duplicate method+path
- app-owned middleware is applied through typed hook objects, not string mount points

### Commands
Best fit: command contributions collected in the central extensions registry.

Current command construction is already explicit through `wire` and command packages.

Clean change:
- let the central extensions registry expose contributed commands
- let the existing command list append them

Policy:
- duplicate command names are an error

### Queue job handlers
Best fit: consumer contributions collected in the central extensions registry, not worker struct growth.

Current worker shape directly injects specific jobs into `Worker`.
That does not scale for reusable packages.

Recommended framework change:
- add `internal/jobs/registry.go`
- `wire` constructs extension resources and any app-owned hooks
- the central extensions registry returns a flat list of `queue.Consumer`s or named `ConsumerContribution`s
- `Worker` consumes the registry output instead of accumulating one field per job

This is the most important runtime cleanup to make extensions viable.

### Events
Best fit: subscriber contributions collected in the central extensions registry.

Events are slightly different:
- publishers usually do not need registration
- subscribers do

Recommended change:
- app boot consumes the registry’s subscriber contributions
- app-side event adapter opens subscriptions during startup
- shutdown closes them through lifecycle hooks

### Scheduler
Best fit: declarative schedule contributions collected in the central extensions registry.

Current generated apps already have:
- `internal/scheduler/scheduler_registry.go`

Recommended change:
- keep schedule registration app-owned and declarative
- extension registration adds schedules to the central registry
- avoid raw cron string bags if possible; keep names explicit and first-class

### Lifecycle hooks
Best fit: lifecycle contributions collected in the central extensions registry.

This already aligns with:
- `internal/app/lifecycle_registry.go`

Recommended change:
- add extension hook registration to the central registry after app-local hooks are built
- keep hook names explicit
- aggregate errors the same way current lifecycle does

### Primitive accessors
Best fit: generate them from extension resource manifests and bridge them through `wire` providers.

This is a key benefit of the design:
- if an extension declares a cache, storage instance, queue, or event bus
- GoForj can generate the same named accessor treatment the rest of the framework already uses

That keeps extension-backed infrastructure first-class instead of introducing a second-class configuration path.

### App-owned hooks
Best fit: typed hook structs provided from `./wire`.

This is the right seam for app-specific behavior:
- middleware chains for routes
- wrappers around queue consumers
- wrappers around event subscribers
- any app/provider-owned policy the extension should not hardcode

The extension defines the hook struct shape.
The app provides the concrete hook object in `wire`.
Generated extension registration glue passes that hook object into the extension callback.

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

### Resource names
- duplicate resource kind+name within one extension: error

## Database Migrations And Assets
This is the hardest open area.

Recommended first iteration:
- support routes, commands, schedules, jobs, events, lifecycle, and primitive resource declarations
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
- primitive accessors are already a first-class part of the runtime model
- app-specific middleware and policy can already be wired in provider functions

And it gives a cleaner product story:
- users install extensions as normal Go packages
- extensions define their contract in code
- GoForj generates the `wire` glue and central registry wiring
- user `.env` overrides any extension defaults
- publishing env values into `.env` is optional

## Recommended Rollout

### Phase 1: Make the runtime extension-ready
1. Add public `github.com/goforj/extension` package.
2. Add manifest, typed `ResourceKind`, contribution interfaces, and hook types.
3. Add generated `wire/extensions_gen.go` and `wire/inject_extensions.go`.
4. Add a central app extensions registry.
5. Add route, command, scheduler, and lifecycle consumption from that registry.

### Phase 2: Primitive-backed extension resources
1. Teach `forj generate` to read installed extension manifests.
2. Resolve effective config from user `.env`, then extension defaults, then framework fallbacks.
3. Generate named primitive accessors implied by extension resources.
4. Add optional `forj extension env:publish`.

### Phase 3: Fix jobs/events for scalable composition
1. Add queue consumer registry in generated apps.
2. Add event subscriber bootstrap registry.
3. Stop modeling reusable jobs as fields on `Worker`.

### Phase 4: Tooling
1. Add docs for authoring an extension package.
2. Add `forj extension add`.
3. Add `forj extension remove`.
4. Add `forj extension list`.

### Phase 5: Harder surfaces
1. Migrations
2. Frontend assets / starter-kit UI modules
3. Lighthouse extension metadata

## Recommendation
Yes, GoForj should support extensions.

But the clean product model is:
- compile-time extension packages
- a public `extension` contract package
- extension contracts defined in Go code
- normal envs with user `.env` always taking precedence
- optional env publishing
- GoForj-generated `wire` glue
- one central extensions registry consumed by the current runtime
- app-owned typed hooks provided from `./wire`
- generated named primitive accessors for extension-backed resources

Not runtime plugin loading.

If we follow that shape, a package can cleanly provide:
- routes
- commands
- queue consumers
- event subscribers
- scheduler entries
- lifecycle hooks
- primitive resources such as caches, storage instances, queues, and event buses

And it will fit the current framework model without turning GoForj into a dynamic plugin system.
