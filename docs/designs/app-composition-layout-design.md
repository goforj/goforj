# App Composition Layout Design

This note explores whether generated GoForj Apps should move App registration and dependency assembly into target-scoped composition roots.

The goal is not to introduce a second App model. The goal is to make the common single-App case obvious while giving larger projects a clean path to fan out into multiple deployable targets that share the same internal application code.

## Problem

Today, a generated App has domain implementation files under `internal/`, but several App-level registration and composition points also live under `internal/` or `wire/`.

Examples:

- `internal/cmd/app_commands.go`
- `internal/router/routes_registry.go`
- `internal/jobs/worker.go`
- `wire/inject_cmd.go`
- `wire/inject_http_controllers.go`
- `wire/inject_jobs_app.go`
- `wire/inject_app_services.go`

That works, but it makes the App composition surface feel scattered. For larger projects, users may reach for "services" or "modules" to create a clearer shape. That risks inventing a second model.

The simpler model should be:

- one GoForj project
- one default App target for the majority case
- optional additional App targets when the project fans out
- thin executable entrypoints under `cmd/<target>/`
- domain implementation under `internal/`
- App registration and dependency assembly under `app/` for the default target and `app/<target>/` for additional targets
- generated registries remain target-level composition, not domain-owned behavior

## Core Model

Use **App target** as the public term.

An App target is a named composition root that wires application code into one deployable GoForj runtime surface.

The default target is named `app`, but its files live directly under `app/` instead of `app/app/`.

If `.goforj.yml` does not define targets, GoForj should assume the normal single-target App:

- default target: `app`
- binary: `app`
- composition root: `app/`
- entrypoint: `cmd/app/`

The mental model becomes:

- `app/` answers "what does the default target expose?"
- `app/wire/` answers "how is the default target assembled?"
- `app/<target>/` answers "what does this additional target expose?"
- `app/<target>/wire/` answers "how is this additional target assembled?"
- `cmd/<target>/` answers "which executable starts this target?"
- `internal/` answers "how does the application behavior work?"
- `migrations/` answers "how does database state evolve?"

For most projects, there is only one target:

```text
platform/
  .goforj.yml
  go.mod

  cmd/
    app/
      main.go

  app/
    commands.go
    routes.go
    jobs.go
    events.go
    schedules.go
    providers.go

    wire/
      wire.go
      wire_gen.go
      inject_cmd.go
      inject_http.go
      inject_http_controllers.go
      inject_jobs.go
      inject_jobs_app.go
      inject_repositories.go
      inject_app_services.go

  internal/
    billing/
      reports/
        controller.go
        generate_job.go
        sync_cmd.go
        service.go
    identity/
      users/
        controller.go
        service.go

  migrations/
```

When the project needs to fan out, it adds more targets:

```text
platform/
  cmd/
    app/
      main.go
    billing/
      main.go
    reporting/
      main.go

  app/
    routes.go
    jobs.go
    schedules.go
    providers.go
    wire/

    billing/
      routes.go
      jobs.go
      schedules.go
      providers.go
      wire/

    reporting/
      routes.go
      jobs.go
      providers.go
      wire/

  internal/
    billing/
      invoices/
        controller.go
        generate_job.go
        service.go
    identity/
      users/
        controller.go
        service.go
    reports/
      daily_schedule.go
      service.go
```

The fan-out rule is:

> `internal/` owns behavior. `app/` and `app/<target>/` own exposure.

Controllers, services, job handlers, subscribers, and domain-owned scheduled methods live under `internal/`. Routes, command registration, job registration, event subscription, schedule registration, provider selection, and Wire assembly live under the target.

Entrypoints under `cmd/<target>/` should stay thin. They should start the generated command surface for the target, not own routes, jobs, schedules, provider sets, or business workflows.

A target is not the same thing as a runtime type. A `billing` target may expose HTTP routes, queue workers, scheduler entries, commands, and events from the same composition root. Split targets should represent a meaningful deployment or ownership boundary, not merely "API versus worker" files.

## Go `internal` Constraint

This model works naturally for one repository or Go module that builds multiple deployable targets.

Targets under the same module can share `internal/...` packages:

```text
platform/app/billing
platform/app/reporting
platform/internal/billing
```

If a team later splits targets into separate repositories or separate modules outside the parent tree, those modules cannot import the original project's `internal/...` packages. Shared code must move to a normal package or sibling module at that point.

Docs should be explicit about this. Target fan-out is a clean monorepo or single-module scale-out path. It is not a cross-repo shared-library mechanism.

## Runtime Support Versus Target Composition

Not every generated runtime file should move into `app/<target>/`.

Runtime support can remain in generated `internal/...` packages when it is reusable machinery:

- `internal/app` for lifecycle, timeout policy, discovery, and root runtime support
- `internal/http` for HTTP runtime implementation
- `internal/jobs` for worker runtime implementation
- `internal/schedules` for scheduler runtime implementation
- `internal/cmd` for reusable command plumbing when needed

Target composition should live in the App composition layer:

- `app/commands.go` for the default target
- `app/routes.go` for the default target
- `app/jobs.go` for the default target
- `app/events.go` for the default target
- `app/schedules.go` for the default target
- `app/providers.go` for the default target
- `app/wire/...` for the default target
- `app/<target>/commands.go` for additional targets
- `app/<target>/routes.go` for additional targets
- `app/<target>/jobs.go` for additional targets
- `app/<target>/events.go` for additional targets
- `app/<target>/schedules.go` for additional targets
- `app/<target>/providers.go` for additional targets
- `app/<target>/wire/...` for additional targets

This keeps the App composition layer from becoming a dumping ground for business logic or low-level runtime implementation.

## Entrypoints

Use `cmd/<target>/` for Go executable entrypoints.

This follows the common Go convention that `cmd/<name>` maps to an executable named `<name>` while keeping substantial application composition outside `cmd/`.

Single-target project:

```text
cmd/
  app/
    main.go

app/
  routes.go
  jobs.go
  schedules.go
  providers.go
  wire/
```

Multi-target project:

```text
cmd/
  app/
    main.go
  billing/
    main.go
  reporting/
    main.go

app/
  routes.go
  providers.go
  wire/

  billing/
    routes.go
    jobs.go
    schedules.go
    providers.go
    wire/

  reporting/
    routes.go
    jobs.go
    providers.go
    wire/
```

`cmd/<target>/main.go` should generally do little more than invoke that target's generated root command or runtime launcher.

Do not move the composition layer into `cmd/<target>/`. `cmd/` is the binary entrypoint layer. `app/` is the GoForj composition layer.

## Package Naming

Use simple package names that follow Go naming rules.

Suggested rules:

- `app/` uses package `app`.
- `app/<target>/` uses a Go-safe package name derived from the target name.
- `app/wire/` and `app/<target>/wire/` use package `wire`.
- `cmd/<target>/main.go` uses package `main`.

Target names may be user-facing slugs for CLI and binary names, but generated Go package names must be valid identifiers.

Example:

```text
target: customer-portal
entrypoint: cmd/customer-portal/main.go
composition path: app/customer-portal/
binary: bin/customer-portal
package name: customerportal
```

This keeps the CLI and filesystem readable without forcing hyphenated target names into invalid Go package names.

## CLI Model

Avoid spreading `--target` across normal workflows.

The command shape should use target prefixes:

```bash
forj <target> <command>
```

If the first argument matches a configured App target, `forj` enters that target context and resolves the rest of the command against that target.

Examples:

```bash
forj app --help
forj app route:list
forj billing --help
forj billing route:list
forj billing worker --queue invoices
forj reporting scheduler
```

When no target is specified, `forj` uses the default target, which is normally `app`:

```bash
forj route:list
forj worker
forj scheduler
```

These are equivalent to:

```bash
forj app route:list
forj app worker
forj app scheduler
```

The built binary shape should match target names:

```bash
./bin/app run
./bin/billing api
./bin/billing worker
./bin/reporting scheduler
```

Those binaries are built from `cmd/<target>/main.go`.

`forj <target> --help` should behave like asking that target for help. In a built context, this is conceptually equivalent to:

```bash
./bin/<target> --help
```

In source-aware development, `forj` may delegate through the generated target command surface rather than requiring the binary to already exist.

### Command Resolution

Suggested resolution order:

1. If the first argument is a native Framework command, run the native command.
2. Else if the first argument is a configured target, set the active target and resolve the remaining command normally in that target context.
3. Else resolve the command against the default target.

Target names should not be allowed to collide with native Framework commands such as `build`, `dev`, `render`, or `version`.

If no target is configured or specified, command resolution should behave like the normal single-App path and use the implicit `app` target.

After a target prefix, normal command resolution should still apply. Native Framework commands operate against the active target, and generated App commands delegate to that target's command surface.

Inside a target context, target-aware native commands may use the active target:

```bash
forj billing build
forj reporting dev
```

Those should be equivalent to target-scoped native operations, without requiring:

```bash
forj build --target billing
forj dev --target reporting
```

If a generated App command collides with a native target-aware command, native commands should keep precedence. A collision escape hatch can remain explicit if needed.

## Build And Dev

Single-target projects should keep the current ergonomic path:

```bash
forj build
forj dev
```

Those operate on the default `app` target.

Multi-target projects should support target prefixes:

```bash
forj billing build
forj reporting build
forj billing dev
forj reporting dev
```

Unqualified commands should operate on the default target only:

- `forj build` builds the default target.
- `forj dev` runs the default target.
- `forj test:render` validates the default target unless the command explicitly opts into all-target validation.
- `forj build:all` or another explicit command can build every target.
- multi-target dev orchestration could be explicit.

The default should avoid surprising teams by starting every target just because it exists.

## Generator Feel

Generator commands should still feel app-local and familiar.

In a single-target project:

```bash
forj make:controller billing:reports
forj make:command reports:sync -d ./internal/billing/reports
forj make:job billing:reports:generate --queue reports
forj make:event billing:invoice-paid -d ./internal/billing/events
```

Those commands create implementation under `internal/...` and update the default target under `app/...`.

For example:

```bash
forj make:controller billing:reports
```

Creates:

```text
internal/billing/reports/controller.go
```

Updates:

```text
app/routes.go
app/wire/inject_http_controllers.go
```

In a multi-target project, the target prefix should scope registration:

```bash
forj billing make:controller billing:invoices
forj billing make:job billing:invoice-reminders --queue invoices
forj billing make:schedule billing:daily-settlement --every 24h
forj reporting make:job reports:generate --queue reports
```

These commands should create domain-owned files under `internal/...` and update only the selected target's registration and Wire files.

The generator should also support a way to create implementation without registration when needed, but that should be an advanced workflow. The golden path should create and register into the active target.

## Configuration Model

The project config should distinguish project-wide settings from target-specific settings.

Project-wide settings include:

- module name
- render settings
- component availability
- supported drivers
- dependency versions
- shared local development defaults

Target-specific settings include:

- target name
- binary name
- supported runtimes
- default launch behavior
- process identity
- optional target-specific env prefix
- target-specific included providers or resources

Possible shape:

```yaml
app:
  default_target: app
  targets:
    app:
      binary: app
      runtimes: [api, worker, scheduler]
    billing:
      binary: billing
      runtimes: [api, worker, scheduler]
    reporting:
      binary: reporting
      runtimes: [api, worker]
```

When the `app.targets` section is omitted, GoForj should synthesize the single-target default rather than requiring config boilerplate.

Target-specific env should be conservative. It is useful for process ports, worker counts, runtime toggles, and observability identity. It should not force every shared resource to be configured separately unless the target intentionally uses a different resource.

## Runtime Topology

Target split changes deployment topology, not business logic ownership.

A target may support one or more logical runtimes:

- HTTP/API runtime
- worker runtime
- scheduler runtime
- CLI command runtime
- Lighthouse/runtime visibility surfaces

The default `app` target can run combined local development:

```bash
forj app run
./bin/app run
```

Production targets can run leaf runtimes:

```bash
./bin/billing api
./bin/billing worker
./bin/billing scheduler
./bin/reporting worker
```

Docs should keep standalone and distributed behavior distinct. Process-local drivers remain process-local. Shared behavior across distributed targets requires shared infrastructure.

## Observability And Lighthouse

Every target needs stable runtime identity.

Logs, metrics, inspects, health, readiness, and Lighthouse payloads should preserve:

- App name
- App target name
- runtime name
- process or instance identity when relevant

Use **App target** in docs and UI. For machine-readable fields, prefer `app_target` over `app` because `App` already means the whole generated GoForj App.

Examples:

```text
app_name="platform", app_target="billing", runtime="http"
app_name="platform", app_target="billing", runtime="worker"
app_name="platform", app_target="billing", runtime="scheduler"
app_name="platform", app_target="reporting", runtime="worker"
```

Lighthouse should consume target-aware runtime metadata. It should not infer target boundaries from filenames.

Lighthouse agent identity should include:

- `app_name`: the generated GoForj App or project identity
- `app_target`: the composition target inside the App, such as `app`, `billing`, or `reporting`
- `runtime`: the logical runtime, such as `http`, `worker`, `scheduler`, or `cli`
- `instance_id`: a process, host, PID, generated run ID, or other stable-enough process instance identity
- `environment`: local, test, staging, production, or another configured environment label when available

For a single-target local App, the identity is still simple:

```text
app_name="platform", app_target="app", runtime="http"
```

For multi-target local development, each running target/runtime process should connect as its own agent:

```text
app_name="platform", app_target="billing", runtime="http"
app_name="platform", app_target="billing", runtime="worker"
app_name="platform", app_target="reporting", runtime="worker"
```

The UI should group runtime data as:

```text
App -> App Target -> Runtime -> Instance
```

When `app_target="app"` is the only target, Lighthouse may collapse the App Target level so the default single-target experience does not feel more complex.

Target-aware Lighthouse behavior should make it clear:

- which routes belong to which target
- which jobs belong to which target
- which schedules belong to which target
- which resources are shared
- which resources are unavailable or degraded for a specific target

## API Index And OpenAPI

API indexing must become target-aware.

Routes exposed by `billing` should not be mixed with routes exposed only by `reporting`.

Single-target projects can keep unqualified commands. Multi-target projects should use target prefixes:

```bash
forj api-index
forj billing api-index
forj reporting api-index
```

The exact command names may change, but the behavior should be target-scoped.

## Migrations And Database

Migrations should remain project-level by default:

```text
migrations/
```

Command exposure is target-level.

For example, the default `app` target may expose migration commands for local development, while production targets may omit them unless explicitly configured.

This avoids every generated binary casually exposing schema-changing commands by accident.

## Auth, Frontend, And Starter Kits

Generated product surfaces must separate implementation from exposure.

Auth implementation may live under `internal/auth`, while auth routes are exposed by one or more target HTTP runtimes.

Background auth behavior, such as cleanup schedules or mail jobs, may be exposed by a target's worker or scheduler runtimes.

Frontend starter kits should point at a specific target HTTP runtime. A project that later moves background work into another target should not need to rewrite frontend-owned code just because worker or scheduler composition moved.

## What Should Not Happen

Do not introduce a new "service" abstraction just to organize files.

Avoid:

```text
services/billing
services/identity
services/notifications
```

unless each entry is truly a separate GoForj project with separate repository, release, runtime, and configuration ownership.

Also avoid a parallel module registry abstraction unless there is a concrete problem App targets cannot solve.

Do not let `app/` or `app/<target>/` accumulate business logic. The composition layer should compose and expose application behavior; it should not own domain workflows.

## Benefits

- Gives users one obvious place to inspect target composition.
- Keeps the single-App case as the default.
- Keeps the single-App layout clean: `cmd/app`, `app`, `internal`.
- Gives larger Apps a clear fan-out path.
- Aligns executable entrypoints with the common Go `cmd/<name>` convention.
- Keeps domain packages focused on implementation.
- Avoids pretending large internal packages are independent services.
- Makes generated registration files feel intentional rather than hidden.
- Keeps `forj` commands app-local and familiar.
- Avoids `--target` flags on every command.
- Makes docs clearer: "The default target lives in `app/`; additional targets live in `app/<target>/`; implementation lives in `internal/`."

## Risks

- The App composition layer could become a dumping ground if it accepts business logic.
- Mixing default target files and named target directories under `app/` creates reserved-name concerns.
- Moving Wire into `app/wire/` and `app/<target>/wire/` requires import path and generated template churn.
- Existing docs and tests reference `wire/` and `internal/*` registration paths heavily.
- Command resolution becomes more complex once target names enter the top-level `forj` grammar.
- Target names can collide with native Framework commands unless validation prevents it.
- Users may expect targets to imply separate repositories or microservices.
- The migration could be disruptive for existing generated Apps.

## Migration Direction

This should not be a sudden breaking layout change.

Possible path:

1. Define App targets in `.goforj.yml`, with `app` as the default target.
2. Add new templates for `cmd/app/`, `app/`, and `app/wire/` in new Apps only.
3. Keep compatibility with current `internal/` and `wire/` registration files for existing Apps.
4. Teach generators to detect which layout exists.
5. Add target detection and command resolution to `forj`.
6. Add target-aware generator registration.
7. Add target-aware build, dev, API index, OpenAPI, metrics identity, and Lighthouse metadata.
8. Add rendered smoke scenarios for single-target and multi-target Apps.
9. Update docs to describe App targets as the preferred generated shape.
10. Consider a migration command only after the new layout has proven itself.

This project has not launched publicly yet, so migration urgency is lower than choosing the right default shape. Legacy detection still matters for local rendered Apps and existing internal smoke targets.

Generator detection could be:

- if the active target is the default target and `app/commands.go` exists, update the default target layout
- else if `app/<target>/commands.go` exists, update the named target layout
- else update the legacy `internal/cmd/app_commands.go`
- if the active target is the default target and `app/wire/` exists, update `app/wire/*`
- else if `app/<target>/wire/` exists, update `app/<target>/wire/*`
- else update `wire/*`

## Reserved Names

Keep reserved names under `app/` minimal.

The required reserved name is:

- `wire`

That prevents an additional target from colliding with the default target's `app/wire/` support directory.

Target names should also be validated as path-safe slugs and should not collide with generated files directly under `app/`.

## Implementation Tasks

Track implementation as concrete work items:

- [ ] Add an App target model to project configuration.
  - [ ] Synthesize the implicit single-target default when `app.targets` is omitted.
  - [ ] Validate target names as path-safe slugs.
  - [ ] Reject target names that collide with native Framework commands or reserved `app/` names.

- [ ] Render the new single-target layout for new Apps.
  - [ ] Generate `cmd/app/main.go`.
  - [ ] Generate default target composition under `app/`.
  - [ ] Generate default target Wire files under `app/wire/`.
  - [ ] Keep generated runtime support packages under `internal/...` where they are reusable machinery.

- [ ] Add named target rendering.
  - [ ] Generate `cmd/<target>/main.go`.
  - [ ] Generate named target composition under `app/<target>/`.
  - [ ] Generate named target Wire files under `app/<target>/wire/`.
  - [ ] Derive Go-safe package names from target slugs.

- [ ] Update command resolution.
  - [ ] Detect `forj <target> ...`.
  - [ ] Set the active target for the remaining command.
  - [ ] Keep unqualified commands on the default target.
  - [ ] Preserve native Framework command precedence.
  - [ ] Keep an explicit collision escape hatch for generated App commands if needed.

- [ ] Update build and dev workflows.
  - [ ] Make `forj build`, `forj dev`, and `forj test:render` operate on the default target.
  - [ ] Add explicit all-target build or render validation when needed.
  - [ ] Build target binaries from `cmd/<target>/main.go`.
  - [ ] Ensure `forj <target> build` and `forj <target> dev` operate on the active target.

- [ ] Update generators.
  - [ ] Register generated controllers, commands, jobs, events, schedules, providers, and Wire entries into the active target.
  - [ ] Keep generated implementation under `internal/...`.
  - [ ] Preserve legacy layout detection for existing internal smoke targets.
  - [ ] Add tests for default-target and named-target generation.

- [ ] Move target-level registration out of runtime support packages.
  - [ ] Move route registration to `app/routes.go` and `app/<target>/routes.go`.
  - [ ] Move command registration to `app/commands.go` and `app/<target>/commands.go`.
  - [ ] Move job registration to `app/jobs.go` and `app/<target>/jobs.go`.
  - [ ] Move event registration to `app/events.go` and `app/<target>/events.go`.
  - [ ] Move schedule registration to `app/schedules.go` and `app/<target>/schedules.go`.
  - [ ] Keep HTTP, worker, scheduler, lifecycle, and command runtime machinery in generated `internal/...` packages for now.

- [ ] Update target-aware product surfaces.
  - [ ] Scope API index and OpenAPI generation to the active target.
  - [ ] Scope route lists to the active target.
  - [ ] Decide which targets expose migration commands.
  - [ ] Ensure auth routes, auth jobs, and auth schedules register through target composition.
  - [ ] Ensure starter kits point frontend code at a specific target HTTP runtime.

- [ ] Add target-aware observability and Lighthouse identity.
  - [ ] Add `app_name`, `app_target`, `runtime`, `instance_id`, and `environment` to Lighthouse agent identity.
  - [ ] Use `app_target` in machine-readable metric, inspect, and Lighthouse metadata.
  - [ ] Group Lighthouse data by `App -> App Target -> Runtime -> Instance`.
  - [ ] Collapse the App Target level in Lighthouse when the only target is `app`.

- [ ] Add rendered smoke coverage.
  - [ ] Render and build a default single-target App.
  - [ ] Render and build a multi-target App with targets such as `billing` and `reporting`.
  - [ ] Verify target-specific route lists, jobs, schedules, and binaries.
  - [ ] Verify unqualified commands use the default target.
  - [ ] Verify `forj <target> ...` routes commands to the active target.

- [ ] Update docs and generated READMEs.
  - [ ] Document `cmd/<target>`, `app/`, `app/<target>`, and `internal/` ownership.
  - [ ] Document App targets as composition roots, not runtime types.
  - [ ] Document the default single-target path before multi-target fan-out.
  - [ ] Update generated component READMEs where registration paths move.

## Open Questions

- Should `cmd/<target>/main.go` import the target package directly, or should it call a generated launcher package shared by all targets?
- What is the collision escape hatch for generated commands that share names with native target-aware commands?
- How should target-specific env overrides be named without creating noisy configuration?
- Which generated runtime support packages remain in `internal/` permanently?
- How much of this can be introduced without breaking existing Apps?

## Current Lean

This direction is cleaner than adding service folders, module registries, workspace semantics, or `--target` flags across every command.

The constraint should remain:

> GoForj has one App model. Larger projects use shared internal packages plus visible App targets under `app/` and `app/<target>/`.

The default target should live directly under `app/`. That optimizes for the vast majority of single-target Apps:

```text
cmd/app/
app/
internal/
```

Additional targets should live directly under `app/<target>/`, with matching thin entrypoints under `cmd/<target>/`.

If that remains true, App targets are a scale-out path for the existing model rather than a new abstraction.
