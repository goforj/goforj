# Forj Plugin Design

## Status
- Proposed
- Target: next minor release
- Scope: rendered application architecture (not `forj` runtime plugin loading)

## Problem Statement
Forj-generated apps currently support strong compile-time wiring (`wire`) and package-local registries, but they do not provide a first-class, consistent model for publishing reusable packages that can contribute:
- CLI commands
- HTTP routes/controllers
- jobs/consumers
- schedules
- lifecycle hooks

We want external package authors to publish reusable “plugins” that app owners can opt into cleanly, while preserving:
- thin `main`
- compile-time type safety
- flat package layout
- explicit dependency wiring (no service locator magic)

## Goals
1. Enable ecosystem packages to contribute commands/routes/jobs/schedules/hooks through a uniform contract.
2. Keep dependency injection compile-time via `wire`.
3. Keep application entrypoints and registries explicit and debuggable.
4. Minimize generated-app churn and preserve current runtime behavior by default.
5. Provide straightforward authoring docs and tests.

## Non-Goals
1. Runtime plugin discovery/loading (no dynamic loading, no reflection-based registration).
2. Generic IoC/service locator container.
3. Backport compatibility for old generated layouts in this design doc.

## Design Principles
1. Explicit over implicit.
2. Type-safe contracts over string lookups.
3. Keep plugin points narrow and stable.
4. App owner remains in control of enabled plugins.
5. Composition in registries, construction in `wire`.

## High-Level Architecture
Each plugin provides two lanes:
1. Construction lane: `ProviderSet` for `wire`.
2. Contribution lane: `RegisterX(...)` functions that mutate app registries.

Rendered app adds one central composition point:
- `internal/plugins/registry.go`

Existing package registries delegate to this composition point:
- `internal/cmd/...`
- `internal/router/routes_registry.go`
- `internal/jobs/...`
- `internal/scheduler/scheduler_registry.go`
- `internal/lifecycle/lifecycle_registry.go`

## Contracts
Define stable, small interfaces in app packages.

### Command Contribution
```go
package cmd

type CommandRegistry interface {
	Add(any) // concrete command type accepted by current command wiring
}
```

### Route Contribution
```go
package http

type RouteRegistry interface {
	AddRoutes(...Route)
	AddControllers(...Controller)
}
```

### Job Contribution
```go
package jobs

type JobRegistry interface {
	AddConsumers(...Consumer)
}
```

### Schedule Contribution
```go
package scheduler

type ScheduleRegistry interface {
	Register(...Entry)
}
```

### Lifecycle Contribution
```go
package lifecycle

type HookRegistry interface {
	OnStartup(name string, fn HookFn)
	OnShutdown(name string, fn HookFn)
}
```

## Plugin Shape (Published Package)
```go
package foo

var ProviderSet = wire.NewSet(
	NewService,
	NewController,
	NewSyncCommand,
	NewRecomputeJob,
)

type Deps struct {
	Service    *Service
	Controller *Controller
	SyncCmd    *SyncCommand
	Recompute  *RecomputeJob
}

func RegisterCommands(r cmd.CommandRegistry, d Deps) {
	r.Add(d.SyncCmd)
}

func RegisterRoutes(r http.RouteRegistry, d Deps) {
	r.AddControllers(d.Controller)
}

func RegisterJobs(r jobs.JobRegistry, d Deps) {
	r.AddConsumers(d.Recompute)
}

func RegisterSchedules(r scheduler.ScheduleRegistry, d Deps) {
	r.Register(
		scheduler.Entry{
			Command:  "foo:sync",
			Cron:     "*/5 * * * *",
			Timezone: "UTC",
		},
	)
}

func RegisterLifecycleHooks(r lifecycle.HookRegistry, d Deps) {
	r.OnStartup("foo.warmup", d.Service.Warmup)
	r.OnShutdown("foo.flush", d.Service.Flush)
}
```

## Example Plugin Repository Layout
```text
forj-plugin-monitoring/
  go.mod
  README.md
  plugin/
    plugin.go
    providers.go
    deps.go
  commands/
    monitor_sync_cmd.go
  http/
    controller.go
  jobs/
    recompute_job.go
  scheduler/
    entries.go
  lifecycle/
    hooks.go
  internal/
    service/
      service.go
    repo/
      repo.go
  examples/
    app-integration/
      internal/plugins/registry.go
      internal/plugins/providers.go
```

Minimal plugin package shape:

`plugin/providers.go`
```go
package plugin

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	service.NewService,
	repo.NewRepo,
	commands.NewMonitorSyncCmd,
	http.NewController,
	jobs.NewRecomputeJob,
)
```

`plugin/deps.go`
```go
package plugin

type Deps struct {
	Service    *service.Service
	Controller *http.Controller
	SyncCmd    *commands.MonitorSyncCmd
	Recompute  *jobs.RecomputeJob
}
```

`plugin/plugin.go`
```go
package plugin

func RegisterCommands(r cmd.CommandRegistry, d Deps) {
	r.Add(d.SyncCmd)
}

func RegisterRoutes(r http.RouteRegistry, d Deps) {
	r.AddControllers(d.Controller)
}

func RegisterJobs(r jobs.JobRegistry, d Deps) {
	r.AddConsumers(d.Recompute)
}

func RegisterSchedules(r scheduler.ScheduleRegistry, d Deps) {
	r.Register(scheduler.Entry{
		Command:  "monitor:sync",
		Cron:     "*/5 * * * *",
		Timezone: "UTC",
	})
}

func RegisterLifecycleHooks(r lifecycle.HookRegistry, d Deps) {
	r.OnStartup("monitoring.warmup", d.Service.Warmup)
	r.OnShutdown("monitoring.flush", d.Service.Flush)
}
```

## Rendered App Integration

### 1. Central plugin composition
`internal/plugins/registry.go`
- App owner imports desired plugins.
- App owner calls plugin registration functions.
- This is the only place app owners edit to enable/disable plugins.

### 2. Wire aggregation point
Add plugin provider set include point in `wire`:
- `internal/plugins/providers.go` in app
- `var PluginProviderSet = wire.NewSet(/* plugin.ProviderSet... */)`
- Included from app injector set.

### 3. Registry delegation
Current generated registries call plugin composition:
- commands registry -> `plugins.RegisterCommands(...)`
- routes registry -> `plugins.RegisterRoutes(...)`
- jobs registry -> `plugins.RegisterJobs(...)`
- scheduler registry -> `plugins.RegisterSchedules(...)`
- lifecycle registry -> `plugins.RegisterLifecycleHooks(...)`

## Initialization and Ordering
1. `wire` constructs all dependencies (including plugin deps).
2. App initializes registries.
3. App local registrations run first.
4. Plugin registrations run second.
5. Duplicate name/path checks execute during registry build and fail fast with clear errors.

Default policy:
- command name collisions: error
- route method+path collisions: error
- schedule ID/command collisions: error unless explicitly allowed
- lifecycle hook name collisions: error

## Error Handling
1. Registration-time errors should be returned (or panic in tests-only helpers), not logged-and-continue.
2. Lifecycle hook execution errors already aggregate in shutdown/startup manager; keep this behavior.
3. Plugin wiring failures must fail app start deterministically.

## Security and Trust Model
1. Plugins are regular Go modules; trust is source-level (same as dependencies).
2. No runtime downloaded code.
3. Secrets/config remain app-owned; plugins read only injected config values.

## Package Layout in Generated App (Flat)
```text
internal/
  plugins/
    registry.go
    providers.go
  cmd/
  http/
  jobs/
  scheduler/
  lifecycle/
wire/
```

No nested plugin framework package tree required.

## Testing Strategy

### Unit Tests
1. Registry collision behavior per domain.
2. Plugin composition delegates to correct domain registries.
3. Lifecycle ordering with plugin + app hooks.

### Integration Tests (Generated App)
1. Render app with a fixture plugin module.
2. Verify command appears in help and executes.
3. Verify route is present in route list and serves.
4. Verify job consumer is registered and invokable.
5. Verify schedule entry appears in scheduler registry output.
6. Verify startup/shutdown hooks execute in expected order.

### Wire/Compile Safety
1. `wire generate` succeeds with plugin provider set.
2. Missing plugin providers produce compile-time failures.

## Rollout Plan

### Phase 1: Contracts + Internal Composition
1. Add `internal/plugins` templates.
2. Add plugin delegation calls in all domain registries.
3. Add empty default plugin implementation (no-op).

### Phase 2: Wire Integration
1. Add plugin provider set include point.
2. Ensure generated app compiles with no plugins.
3. Add fixture plugin integration test.

### Phase 3: Developer Experience
1. Add docs: “How to publish a Forj plugin package”.
2. Add `forj make:plugin` scaffolder (optional).
3. Add diagnostics command (optional): `forj plugin:list`.

## Migration Impact
1. Existing apps continue to work if they do not use plugins.
2. Generated registries gain one extra call into `internal/plugins`.
3. No runtime behavior change for apps with no configured plugins.

## Open Questions
1. Should schedule registration use command strings only, or allow typed job references?
2. Should plugin registration accept context for conditional registration?
3. Do we expose plugin metadata (name/version/capabilities) to lighthouse?

## Ecosystem Opportunities and Gaps

### Gaps to Address
1. Standard plugin config contract:
   A consistent way to load/validate plugin configuration.
2. Plugin migrations/assets contract:
   A first-class mechanism for plugin-owned DB migrations, seed data, and static assets.
3. Observability contract:
   Standard hooks for plugin metrics, tracing, and structured logs.
4. Capability metadata:
   Plugin-declared capabilities (commands/routes/jobs/schedules/hooks) for introspection and tooling.
5. Compatibility policy:
   Declared supported forj/app API versions to reduce upgrade breakage.

### Likely Plugin Categories
1. Auth and identity:
   OAuth/SSO providers, RBAC, API key systems, multitenancy guards.
2. Notification and integration packs:
   Slack, Teams, PagerDuty, webhook, SMS, and email providers.
3. Monitoring and operations:
   Check providers, synthetic probes, incident automation, on-call workflows.
4. Data connectors:
   CRM/warehouse/external API syncs, ingestion pipelines, webhook receivers.
5. Domain feature packs:
   Billing, audit trails, feature flags, workflow engines, approval flows.
6. Developer productivity:
   Additional generators, lint/test/build commands, project scaffolding helpers.
7. API platform features:
   Pagination/filtering conventions, policy middleware, rate limiting, error contracts.
8. Admin/lighthouse modules:
   Dashboards, diagnostics panels, queue and scheduler inspectors.

## Acceptance Criteria
1. A plugin package can add at least one command, route, job consumer, schedule, and lifecycle hook.
2. All additions are visible in generated app behavior and tests.
3. `wire generate` remains the only dependency construction mechanism.
4. `main` remains thin and unchanged except normal app boot wiring.
