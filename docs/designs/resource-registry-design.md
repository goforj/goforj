# Resource Registry Design Sketch

This document sketches a first-class resource registry primitive for GoForj.

Status:

- exploratory
- intended to unify local resource discovery across multiple surfaces
- intended to prevent link and utility presentation logic from being reimplemented in many places

## Goal

Create one centralized source of truth for discoverable project resources such as:

- app URL
- API URL
- Lighthouse URL
- Swagger or OpenAPI docs URL
- Mailpit URL
- Grafana URL
- VictoriaMetrics URL
- any future runtime-discovered utility or dashboard

That registry should be consumable from:

- `forj render`
- `forj dev`
- Lighthouse
- systray
- any future control-plane or local UX surface

## Problem

We already have several ingress points that want to present useful links:

- `forj render` next steps
- `forj dev` startup summary
- Lighthouse
- systray

Right now, each one is at risk of developing its own partial view of:

- which resources exist
- how they are named
- when they should be shown
- how they are grouped
- how they are discovered

That creates drift quickly.

Examples of drift:

- one surface says `App`, another says `Frontend`
- one surface knows about Mailpit, another does not
- one surface hardcodes Swagger, another derives it dynamically
- one surface shows disabled resources, another hides them

## Core Idea

GoForj should have a first-class resource registry abstraction.

That registry should answer:

- what resources exist for this project
- which are enabled
- which are currently reachable or expected
- how they should be labeled
- where they should appear

The registry should separate:

- resource definition
- runtime resolution
- presentation

## Design Principles

- one canonical resource model
- resource definitions should be reusable across surfaces
- consumers should not need to know how each URL is derived
- resources may be static, config-derived, or runtime-discovered
- presentation surfaces may choose subsets, but should not invent resource semantics

## Proposed Model

Suggested core type:

```go
type Resource struct {
	ID          string
	Name        string
	Category    string
	URL         string
	Description string
	Enabled     bool
	Priority    int
	Source      string
}
```

Suggested supporting concepts:

- `ID`
  - stable machine identifier
  - examples: `app`, `api`, `mailpit`, `grafana`, `lighthouse`, `swagger`
- `Name`
  - user-facing label
- `Category`
  - examples: `app`, `docs`, `observability`, `mail`, `devtools`
- `URL`
  - fully resolved launchable URL
- `Enabled`
  - whether the resource should be shown
- `Priority`
  - used for ordering
- `Source`
  - where it came from
  - examples: `config`, `runtime`, `component`, `lighthouse`

## Resource Registry Interface

One possible shape:

```go
type Registry interface {
	List(ctx context.Context) ([]Resource, error)
}
```

That is probably too small for real use. A more practical shape:

```go
type Registry interface {
	List(ctx context.Context) ([]Resource, error)
	ByID(ctx context.Context, id string) (Resource, bool, error)
}
```

And an internal builder shape:

```go
type Resolver interface {
	Resolve(ctx context.Context) ([]Resource, error)
}
```

Then the registry can compose many resolvers:

- static app resolver
- local dev services resolver
- observability resolver
- OpenAPI or Swagger resolver
- Lighthouse resolver
- future plugin or extension resolvers

## Static Versus Dynamic Resources

We need to support both:

### Static or Config-Derived

Examples:

- app URL
- API URL
- Grafana URL when enabled
- Mailpit URL when Docker mail is enabled

These can often be derived from config and env.

### Runtime-Discovered

Examples:

- dynamically chosen local ports
- dev-only service URLs
- Lighthouse URL discovered from active session
- resource URLs coming from extensions or plugins

These may only exist once a dev session is running.

The registry should support merging both without consumers caring which kind they are looking at.

## Recommended Resource Sources

Suggested first sources:

- project config
- rendered env
- component enablement
- runtime dev session state
- Lighthouse session state

That means the registry likely needs two layers:

- a base project resource resolver
- a live session resource resolver

## Suggested Categories

Useful initial categories:

- `app`
- `docs`
- `mail`
- `observability`
- `devtools`
- `admin`

Examples:

- `app`
  - App
  - API
- `docs`
  - Swagger
  - OpenAPI
- `mail`
  - Mailpit
- `observability`
  - Grafana
  - VictoriaMetrics
  - Lighthouse

## Presentation Rules

The registry should define semantics, not full UI.

But it should support enough metadata so all surfaces behave consistently.

Recommended defaults:

- hide disabled resources
- order by category then priority
- use stable names
- allow surfaces to filter categories

Examples:

- `forj render`
  - show likely next useful local resources
- `forj dev`
  - show active resources available in the current session
- systray
  - show highest-priority launch links
- Lighthouse
  - show the complete session resource set

## Why This Should Be Separate From Systray

The systray is only one consumer.

The registry belongs lower in the stack because:

- `forj render` needs it
- `forj dev` needs it
- Lighthouse needs it
- systray needs it
- future surfaces will likely need it too

If the registry is designed inside the systray effort, the rest of the system will depend on a UI-driven abstraction, which is the wrong direction.

## Suggested Package Direction

One possible location:

- `internal/resources`

Possible files:

- `resource.go`
- `registry.go`
- `resolver_static.go`
- `resolver_runtime.go`
- `resolver_observability.go`
- `resolver_openapi.go`

If later we want generated apps or external tools to consume this concept directly, the model can be moved or mirrored more publicly.

## Possible API Shapes For Consumers

For internal callers:

```go
resources, err := resources.RegistryForProject(cfg).List(ctx)
```

For runtime session-aware callers:

```go
resources, err := resources.RegistryForSession(cfg, sessionState).List(ctx)
```

For rendering shortcuts:

```go
links := resources.Filter(resources, resources.Category("observability"))
```

## Milestones

### Milestone 1: Canonical Model

Objective:

- define the core `Resource` type and ordering/filtering rules

Tasks:

- define resource model
- define category and stable ID conventions
- define sorting and filtering helpers
- add unit tests for ordering and filtering

Exit criteria:

- one canonical model exists
- tests prove deterministic ordering and filtering

### Milestone 2: Base Registry

Objective:

- provide a registry that resolves config-derived and component-derived resources

Tasks:

- add `internal/resources`
- implement static resolvers for:
  - app
  - API
  - Mailpit
  - Grafana
  - VictoriaMetrics
  - Lighthouse
  - Swagger or OpenAPI
- add tests for enabled and disabled combinations

Exit criteria:

- `forj render` and `forj dev` can both call the same base registry

### Milestone 3: Runtime Session Resources

Objective:

- merge runtime-discovered resources into the registry

Tasks:

- define a runtime resource input shape
- allow live port or URL overrides
- support session-scoped dynamic links
- support resource updates over time

Exit criteria:

- one session can enrich or override the base registry cleanly

### Milestone 4: Surface Integration

Objective:

- replace hardcoded link lists with registry-backed output

Tasks:

- wire registry into `forj render`
- wire registry into `forj dev`
- wire registry into Lighthouse
- wire registry into systray

Exit criteria:

- all major surfaces present links from the same source of truth

### Milestone 5: Extensibility

Objective:

- make future resource additions cheap and uniform

Tasks:

- define how extensions or plugins contribute resources
- add resolver registration hooks if needed
- document how new components should register resources

Exit criteria:

- adding a new resource does not require touching every surface manually

## Recommended First Resources

The first registry-backed resources should be:

- App
- API
- Lighthouse
- Swagger or OpenAPI
- Mailpit
- Grafana
- VictoriaMetrics

That covers the exact high-value cases already emerging across the current product surface.

## Recommendation

This should become a dedicated primitive.

The best path is:

- define a canonical `Resource` model
- centralize resolution in one registry package
- make every presentation surface consume it
- let runtime state enrich it without redefining it

That gives GoForj one consistent way to discover and present local utilities over time instead of building one-off link lists at every ingress point.
