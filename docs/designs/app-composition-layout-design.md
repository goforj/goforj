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
- `wire/inject_http_controllers_app.go`
- `wire/inject_jobs_app.go`
- `wire/inject_services_app.go`

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
    root_cmd.go
    routes.go

    wire/
      wire.go
      wire_gen.go
      inject_cmd.go
      inject_cmd_app.go
      inject_http.go
      inject_http_controllers_app.go
      inject_jobs.go
      inject_jobs_app.go
      inject_repositories_app.go
      inject_services_app.go

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
    commands.go
    root_cmd.go
    routes.go
    wire/

    billing/
      commands.go
      root_cmd.go
      routes.go
      wire/

    reporting/
      commands.go
      root_cmd.go
      routes.go
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

Controllers, services, job handlers, subscribers, and domain-owned scheduled methods live under `internal/`. Routes, command exposure, provider constructor selection, and Wire assembly live under the target. Dedicated target-level job, event, and schedule registry files should be added only when they own meaningful registration state; the current implementation keeps those registrations in generated internal runtime registries plus `app/wire/*` assembly.

Provider constructors should not require a separate target-level `providers.go` file by default. The default generated shape keeps command exposure in `app/commands.go`, framework command assembly in `app/wire/inject_cmd.go`, and app-owned command provider constructors in `app/wire/inject_cmd_app.go`. Other provider constructors live in the relevant `app/wire/inject_*.go` file, such as `app/wire/inject_http_controllers_app.go`, `app/wire/inject_jobs_app.go`, or `app/wire/inject_repositories_app.go`.

A target-level `providers.go` can still be created by users when they want a visible custom composition file, but it is not part of the default generator contract.

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

- `internal/runtime` for lifecycle, timeout policy, discovery, and root runtime support
- `internal/http` for HTTP runtime implementation
- `internal/jobs` for worker runtime implementation
- `internal/schedules` for scheduler runtime implementation
- `internal/cmd` for reusable command plumbing when needed

Target composition should live in the App composition layer:

- `app/commands.go` for the default target
- `app/root_cmd.go` for the default target
- `app/routes.go` for the default target
- `app/jobs.go` for the default target when a separate target-level job registry becomes useful
- `app/events.go` for the default target when a separate target-level event registry becomes useful
- `app/schedules.go` for the default target when a separate target-level schedule registry becomes useful
- `app/wire/...` for the default target
- `app/<target>/commands.go` for additional targets
- `app/<target>/root_cmd.go` for additional targets
- `app/<target>/routes.go` for additional targets
- `app/<target>/jobs.go` for additional targets when a separate target-level job registry becomes useful
- `app/<target>/events.go` for additional targets when a separate target-level event registry becomes useful
- `app/<target>/schedules.go` for additional targets when a separate target-level schedule registry becomes useful
- `app/<target>/wire/...` for additional targets

This keeps the App composition layer from becoming a dumping ground for business logic or low-level runtime implementation.

For commands specifically:

- `app/commands.go` defines the target's Kong command exposure.
- `app/wire/inject_cmd.go` provides framework command assembly and the target root command.
- `app/wire/inject_cmd_app.go` provides app-owned command constructors.
- generated `make:command` updates both files for the active target.
- legacy Apps can still fall back to `internal/cmd/app_commands.go` and `wire/inject_cmd.go`.

## Entrypoints

Use `cmd/<target>/` for Go executable entrypoints.

This follows the common Go convention that `cmd/<name>` maps to an executable named `<name>` while keeping substantial application composition outside `cmd/`.

Single-target project:

```text
cmd/
  app/
    main.go

app/
  commands.go
  root_cmd.go
  routes.go
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
  commands.go
  root_cmd.go
  routes.go
  wire/

  billing/
    commands.go
    root_cmd.go
    routes.go
    wire/

  reporting/
    commands.go
    root_cmd.go
    routes.go
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

If the first argument matches a conventional App target, `forj` enters that target context and resolves the rest of the command against that target.

The source of truth for normal CLI targeting is convention, not configuration:

- default target: `cmd/app/main.go`, `app/`, `app/wire/`
- named target: `cmd/<target>/main.go`, `app/<target>/`, `app/<target>/wire/`

`.goforj.yml` may describe targets as metadata for UI, validation, render orchestration, and future overrides, but basic target dispatch should not require a configured target list.

The implementation also supports binary dispatch: if `./bin/<target>` exists, `forj <target> ...` delegates to that binary with the remaining arguments. During dispatch, GoForj sets target identity environment values such as `FORJ_COMMAND_PREFIX=forj <target>`, `FORJ_APP_TARGET=<target>`, and `APP_TARGET=<target>`.

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

In source-aware development, `forj` delegates through the generated target command surface when `cmd/<target>/main.go` exists, without requiring the binary to already exist.

### Command Resolution

Suggested resolution order:

1. If the first argument is a native Framework command, run the native command.
2. Else if the first argument matches a built target binary at `./bin/<target>`, delegate the remaining arguments to that binary.
3. Else if the first argument matches `cmd/<target>/main.go`, set the active target and resolve the remaining command normally in that target context.
4. Else resolve the command against the default target.

Target names should not be allowed to collide with native Framework commands such as `build`, `dev`, `render`, or `version`.

If no target is specified, command resolution behaves like the normal single-App path and uses the implicit `app` target.

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

Current implementation note: source-mode target dispatch is convention-first. `forj <target> build` and `forj <target> run` use `cmd/<target>` and `app/<target>/wire` when those paths exist.

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

Most unqualified commands should operate on the default target only:

- `forj build` builds the default target.
- `forj test:render` validates the default target unless the command explicitly opts into all-target validation.
- `forj build:all` or another explicit command can build every target.

`forj dev` is the deliberate exception. In a multi-target project, unqualified `forj dev` should orchestrate every discovered target at once: watch all target entrypoints, build all target binaries, and run all configured target runtimes together. This keeps local development closer to the deployed shape when an App has fanned out into multiple executables.

Target-prefixed dev remains scoped:

```bash
forj billing dev
```

That command should watch, build, and run only the selected target.

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
app/wire/inject_http_controllers_app.go
```

For commands:

```bash
forj make:command hello:test
```

Creates:

```text
internal/hello/test_cmd.go
```

Updates:

```text
app/commands.go
app/wire/inject_cmd_app.go
```

`app/commands.go` exposes the command on the target root command. `app/wire/inject_cmd_app.go` registers the app-owned command constructor with Wire. The framework-owned `app/wire/inject_cmd.go` should remain stable and include `appCommandSet`. The generator should not create or require `app/providers.go` for command registration.

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

Implemented target settings include:

- target name
- entrypoint path
- composition directory
- Wire directory

Future target-specific settings may include:

- binary name when it differs from the target name
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
    - name: app
      entrypoint: cmd/app/main.go
      app_dir: app
      wire_dir: app/wire
    - name: billing
      entrypoint: cmd/billing/main.go
      app_dir: app/billing
      wire_dir: app/billing/wire
    - name: reporting
      entrypoint: cmd/reporting/main.go
      app_dir: app/reporting
      wire_dir: app/reporting/wire
```

When the `app.targets` section is omitted, GoForj should synthesize the single-target default rather than requiring config boilerplate.

The current implementation synthesizes:

```yaml
app:
  default_target: app
  targets:
    - name: app
      entrypoint: cmd/app/main.go
      app_dir: app
      wire_dir: app/wire
```

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

1. Define App target conventions, with `app` as the default target. Done for the default target and source-mode target dispatch.
2. Add new templates for `cmd/app/`, `app/`, and `app/wire/` in new Apps. Done for the default target.
3. Keep compatibility with current `internal/` and `wire/` registration files for existing Apps. Done through generator path fallback and legacy cleanup.
4. Teach generators to detect which layout exists. Done for the default target generators.
5. Add target detection and command resolution to `forj`. Done for binary dispatch and source-mode convention dispatch.
6. Add target-aware generator registration. Done for default and named target app-owned generator registration.
7. Add target-aware build, dev, API index, OpenAPI, metrics identity, and Lighthouse metadata. Partially done for default-target build/run/wire paths, source-mode target build/run/wire paths, all-target unqualified dev orchestration, and Lighthouse agent identity.
8. Add rendered smoke scenarios for single-target and multi-target Apps. Done for default single-target render coverage; named-target coverage remains.
9. Update docs to describe App targets as the preferred generated shape. In progress.
10. Consider a migration command only after the new layout has proven itself.

This project has not launched publicly yet, so migration urgency is lower than choosing the right default shape. Legacy detection still matters for local rendered Apps and existing internal smoke targets.

Generator detection could be:

- if the active target is the default target and `app/commands.go` exists, update the default target layout
- else if `app/<target>/commands.go` exists, update the named target layout
- else update the legacy `internal/cmd/app_commands.go`
- if the active target is the default target and `app/wire/` exists, update `app/wire/*`
- else if `app/<target>/wire/` exists, update `app/<target>/wire/*`
- else update `wire/*`

Command generator detection now follows that shape for the default target:

- command implementation is generated under `internal/...`
- command exposure is added to `app/commands.go`
- command provider construction is added to `app/wire/inject_cmd_app.go`
- if the new layout is absent, the generator falls back to legacy command files

## Reserved Names

Keep reserved names under `app/` minimal.

The required reserved name is:

- `wire`

That prevents an additional target from colliding with the default target's `app/wire/` support directory.

Target names should also be validated as path-safe slugs and should not collide with generated files directly under `app/`.

## Implementation Tasks

Track implementation as concrete work items:

- [x] Add the default App target model to project configuration.
  - [x] Synthesize the implicit single-target default when `app.targets` is omitted.
  - [x] Store default target metadata for `name`, `entrypoint`, `app_dir`, and `wire_dir`.
  - [x] Default named target metadata by convention when targets are present.
  - [x] Add configured target validation for stable project-layout constraints.
  - [x] Validate `app.default_target` exists in configured targets.
  - [x] Validate target names as path-safe slugs.
  - [x] Reject target names that collide with native Framework commands.
  - [x] Reject target names that collide with reserved `app/` names such as `wire`.
  - [x] Reject target names that collide with generated files directly under `app/`, such as `commands.go`, `root_cmd.go`, and `routes.go`.

- [x] Render the new single-target layout for new Apps.
  - [x] Generate `cmd/app/main.go`.
  - [x] Generate default target composition under `app/`.
  - [x] Generate default target Wire files under `app/wire/`.
  - [x] Keep generated runtime support packages under `internal/...` where they are reusable machinery.
  - [x] Do not generate `app/providers.go` by default.
  - [x] Co-locate the embedded frontend bundle under `cmd/app/frontend/dist`.
  - [x] Remove legacy generated `main.go`, `wire/`, `internal/cmd/app_commands.go`, `internal/cmd/root_cmd.go`, and `internal/router/routes_registry.go` during full render cleanup.
  - [x] Use consistent app-owned injector filenames:
    - [x] `inject_cmd_app.go`
    - [x] `inject_http_controllers_app.go`
    - [x] `inject_jobs_app.go`
    - [x] `inject_repositories_app.go`
    - [x] `inject_schedules_app.go`
    - [x] `inject_services_app.go`
    - [x] `inject_subscribers_app.go`
  - [x] Keep app-owned injectors render-once.
  - [x] Keep framework-owned injectors overwrite-rendered.
  - [x] Mark framework-owned injectors as `DO NOT EDIT`.
  - [x] Mark app-owned injectors as `EDIT THIS FILE`.
  - [x] Mark `wire.go` as an editable Wire harness that may be overwritten by rerender.
  - [x] Keep `repositorySet` limited to repository providers.
  - [x] Move app services such as monitoring retention and incident transition services to `inject_services_app.go`.

- [x] Add named target rendering.
  - [x] Discover named targets from optional config metadata and existing conventional `cmd/<target>/main.go` / `app/<target>/` layouts.
  - [x] Generate `cmd/<target>/main.go`.
  - [x] Generate named target composition under `app/<target>/`.
  - [x] Generate `app/<target>/commands.go`.
  - [x] Generate `app/<target>/root_cmd.go`.
  - [x] Generate `app/<target>/routes.go` when the target has HTTP enabled.
  - [x] Generate `app/<target>/schedules.go` when the scheduler is enabled.
  - [x] Generate named target Wire files under `app/<target>/wire/`.
  - [x] Derive Go-safe package names from target slugs.
  - [x] Ensure hyphenated target slugs map to legal package names.
  - [x] Ensure named target entrypoints import the correct target composition package.
  - [x] Keep named target `wire.go` editable-but-overwrite-rendered like the default target.
  - [x] Keep named target app-owned injectors render-once.
  - [x] Generate target-local frontend dist placeholders under `cmd/<target>/frontend/dist` when Web UI is enabled.
  - [x] Keep named target cleanup non-destructive by only writing known generated files and preserving app-owned files after first render.

- [x] Update command resolution.
  - [x] Detect `forj <target> ...` when `./bin/<target>` exists.
  - [x] Delegate remaining arguments to the built target binary.
  - [x] Set target identity env during binary dispatch.
  - [x] Keep unqualified commands on the default target.
  - [x] Set the active target from `cmd/<target>/main.go` when no binary exists.
  - [x] Route `forj <target> --help` through the source target in source mode.
  - [x] Preserve native Framework command precedence for convention source-mode target dispatch.
  - [x] Preserve native Framework command precedence for convention source-mode target dispatch.
  - [x] Make `forj <target> <native-command>` pass active target context into native commands.
  - [x] Make `forj <target> <app-command>` execute the selected target command surface.
  - [ ] Keep an explicit collision escape hatch for generated App commands if needed.
  - [x] Add tests for convention target dispatch without a prebuilt binary.
  - [x] Add tests for native-command precedence over target and App command names.

- [ ] Partial: update build and dev workflows.
  - [x] Make `forj build` and `forj run` operate on `cmd/app` by default.
  - [x] Prefer `app/wire` as the default Wire path with legacy `wire` fallback.
  - [x] Build the default target binary from `cmd/app/main.go`.
  - [x] Make `forj build` target-aware in source mode.
  - [x] Make `forj run` target-aware in source mode.
  - [x] Make `forj dev` fully active-target aware for named targets.
  - [x] Make unqualified `forj dev` expand default Build/Run watchers across every discovered target.
  - [x] Keep `forj <target> dev` scoped to the selected target.
  - [ ] Add explicit all-target build or render validation when needed.
  - [x] Ensure source-mode `forj <target> dev` operates on the active conventional target.
  - [ ] Add `build:all` or equivalent only if the workflow proves useful.
  - [x] Ensure Wire generation runs in the selected conventional target's `wire_dir` when it exists.
  - [x] Ensure full render runs Wire generation for every discovered target Wire directory.
  - [x] Ensure dev build/watch uses the selected target entrypoint and binary when a named target owns HTTP.

- [x] Partial: update generators.
  - [x] Register generated controllers, commands, jobs, events, schedules, repositories, and Wire entries into the default target layout.
  - [x] Keep generated implementation under `internal/...`.
  - [x] Preserve legacy layout detection for existing internal smoke targets.
  - [x] Move `make:command` provider constructor injection to `app/wire/inject_cmd_app.go`.
  - [x] Keep command exposure injection in `app/commands.go`.
  - [x] Add tests and rendered smoke coverage for default-target generation.
  - [x] Make `make:controller` update `app/routes.go` and `app/wire/inject_http_controllers_app.go` for the default target.
  - [x] Make `make:job` update `app/wire/inject_jobs_app.go` for the default target.
  - [x] Make `make:schedule` update `app/schedules.go` and `app/wire/inject_schedules_app.go` for the default target.
  - [x] Make `make:subscriber` update `app/wire/inject_subscribers_app.go` for the default target.
  - [x] Make `make:model` update `app/wire/inject_repositories_app.go` for the default target.
  - [x] Add active-target context plumbing shared by all generators.
  - [x] Make `make:controller` update `app/<target>/routes.go` and `app/<target>/wire/inject_http_controllers_app.go`.
  - [x] Make `make:command` update `app/<target>/commands.go` and `app/<target>/wire/inject_cmd_app.go`.
  - [x] Make `make:job` update `app/<target>/wire/inject_jobs_app.go`.
  - [x] Make `make:schedule` update `app/<target>/schedules.go` and `app/<target>/wire/inject_schedules_app.go`.
  - [x] Make `make:subscriber` update `app/<target>/wire/inject_subscribers_app.go`.
  - [x] Make `make:model` update `app/<target>/wire/inject_repositories_app.go` for the active target.
  - [x] Make `make:model` default to active target only.
  - [ ] Add an advanced generate-without-registration mode for implementation-only files.
  - [x] Add named-target generator tests for controller, command, job, schedule, subscriber, and model flows.
  - [ ] Preserve legacy fallback behavior for old `wire/` and old app injector filenames.

- [ ] Partial: move target-level registration out of runtime support packages.
  - [x] Move route registration to `app/routes.go` for the default target.
  - [x] Move command exposure to `app/commands.go` for the default target.
  - [x] Keep framework command assembly in `app/wire/inject_cmd.go` for the default target.
  - [x] Move app-owned command provider construction to `app/wire/inject_cmd_app.go` for the default target.
  - [x] Move controller, job, schedule, event subscriber, repository, and app service Wire files to `app/wire/` for the default target.
  - [x] Move lifecycle hook registration from `internal/app/lifecycle_registry.go` to `app/lifecycle.go`.
  - [x] Move schedule registration from `internal/schedules/scheduler_registry.go` to `app/schedules.go`.
  - [x] Keep scheduler observer and runtime registration plumbing in `internal/schedules/registration.go`.
  - [x] Keep generated cache, queue, storage, event, mail, and database managers under `internal/...` as shared runtime support.
  - [ ] Move job registration to `app/jobs.go` and `app/<target>/jobs.go` if a separate job registry file becomes useful.
  - [ ] Move event registration to `app/events.go` and `app/<target>/events.go` if a separate event registry file becomes useful.
  - [x] Move schedule registration to `app/schedules.go` for the default target.
  - [x] Move schedule registration to `app/<target>/schedules.go` for named targets.
  - [ ] Decide whether current Wire-only registration for jobs, events, and schedules is sufficient long term.
  - [x] Keep HTTP, worker, scheduler, lifecycle, and command runtime machinery in generated `internal/...` packages for now.

- [ ] Partial: update target-aware product surfaces.
  - [x] Ensure auth routes and auth command wiring register through default target composition.
  - [x] Ensure starter kits can serve from the default target entrypoint.
  - [ ] Scope API index and OpenAPI generation to the active named target.
  - [x] Scope route lists to the active named target.
  - [ ] Decide which named targets expose migration commands.
  - [ ] Make migration command exposure configurable per target if needed.
  - [ ] Ensure auth routes, auth jobs, and auth schedules register through named target composition.
  - [ ] Ensure starter kits can point frontend code at a named target HTTP runtime.
  - [ ] Ensure generated frontend assets are embedded under the correct `cmd/<target>/frontend/dist` when a named target owns Web UI.
  - [ ] Ensure route/openapi commands show the active target in human-readable output.

- [ ] Partial: add target-aware observability and Lighthouse identity.
  - [x] Add `app_target` to Lighthouse agent identity.
  - [x] Read target identity from `APP_TARGET` or `FORJ_APP_TARGET`, defaulting to `app`.
  - [x] Include `app_target` in hub agent metadata.
  - [ ] Add or verify `app_target` across machine-readable metric and inspect metadata.
  - [ ] Add or verify `app_target` in health/readiness/runtime inspect payloads where relevant.
  - [ ] Group Lighthouse data by `App -> App Target -> Runtime -> Instance`.
  - [ ] Collapse the App Target level in Lighthouse when the only target is `app`.
  - [ ] Verify local multi-target Lighthouse connections use distinct target identity.
  - [ ] Update Lighthouse UI copy and grouping tests for target-aware data.

- [ ] Partial: add rendered smoke coverage.
  - [x] Render and build default single-target Apps across the render matrix.
  - [x] Verify unqualified commands use the default target.
  - [x] Verify `make:command` updates `app/commands.go` and `app/wire/inject_cmd_app.go`.
  - [x] Verify generated Wire rebuilds after `make:command`.
  - [x] Verify app-owned injector files are preserved across rerender.
  - [x] Add unit coverage for convention-discovered named targets and named target template rendering.
  - [x] Manually verify a `/tmp` named-target render builds both default and named target binaries.
  - [x] Verify framework-owned injector files are overwrite-rendered.
  - [x] Verify `repositorySet` does not contain service providers.
  - [x] Render and build a multi-target App with targets such as `customer-portal`.
  - [ ] Verify target-specific jobs and schedules in rendered smoke coverage.
  - [x] Verify target-specific route lists and binaries in rendered smoke coverage.
  - [x] Verify convention source-mode `forj <target> ...` routes commands to the active target in rendered smoke coverage.
  - [ ] Verify named target generators update only the selected target.
  - [ ] Verify default target generators continue to work when named targets are configured.
  - [x] Verify target name validation rejects reserved names and command collisions.
  - [ ] Verify all-target build/render validation when that workflow is added.

- [ ] Partial: update docs and generated READMEs.
  - [x] Document `cmd/<target>`, `app/`, `app/<target>`, and `internal/` ownership in this design.
  - [x] Document App targets as composition roots, not runtime types.
  - [x] Document the default single-target path before multi-target fan-out.
  - [x] Document app-owned versus framework-owned injector ownership in generated headers.
  - [x] Update generated component READMEs where lifecycle registration paths moved.
  - [ ] Update `internal/events/README.md` to mention `app/wire/inject_subscribers_app.go`.
  - [x] Update scheduler docs to mention `app/schedules.go`.
  - [ ] Update model/repository docs to mention `app/wire/inject_repositories_app.go`.
  - [ ] Update controller docs/scenarios to mention `app/wire/inject_http_controllers_app.go`.
  - [ ] Update service/resource workflow scenarios to mention `app/wire/inject_services_app.go`.
  - [ ] Document that `wire.go` is editable but overwrite-rendered.
  - [ ] Document that app-owned injectors are render-once and framework-owned injectors are regenerated.
  - [ ] Decide whether to remove or update unused legacy demo Wire templates.

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
