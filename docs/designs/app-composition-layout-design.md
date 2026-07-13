# App Composition Layout Design

This note explores whether generated GoForj Apps should move App registration and dependency assembly into app-scoped composition roots.

The goal is not to introduce a second App model. The goal is to make the common single-App case obvious while giving larger projects a clean path to fan out into multiple deployable apps that share the same internal application code.

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
- one default App for the majority case
- optional additional Apps when the project fans out
- thin executable entrypoints under `cmd/<app>/`
- domain implementation under `internal/`
- App registration and dependency assembly under `app/` for the default app and `app/<app>/` for additional apps
- generated registries remain app-level composition, not domain-owned behavior

## Core Model

Use **App** as the public term.

An App is a named composition root that wires application code into one deployable GoForj runtime surface.

The default app is named `app`, but its files live directly under `app/` instead of `app/app/`.

When no conventional named apps exist, GoForj should assume the normal single-app App:

- default app: `app`
- binary: `app`
- composition root: `app/`
- entrypoint: `cmd/app/`

The mental model becomes:

- `app/` answers "what does the default app expose?"
- `app/wire/` answers "how is the default app assembled?"
- `app/<app>/` answers "what does this additional app expose?"
- `app/<app>/wire/` answers "how is this additional app assembled?"
- `cmd/<app>/` answers "which executable starts this app?"
- `internal/` answers "how does the application behavior work?"
- `migrations/` answers "how does database state evolve?"

For most projects, there is only one app:

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

When the project needs to fan out, it adds more apps:

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

> `internal/` owns behavior. `app/` and `app/<app>/` own exposure.

Controllers, services, job handlers, subscribers, and domain-owned scheduled methods live under `internal/`. Routes, command exposure, provider constructor selection, and Wire assembly live under the app. Dedicated app-level job, event, and schedule registry files should be added only when they own meaningful registration state; the current implementation keeps those registrations in generated internal runtime registries plus `app/wire/*` assembly.

Provider constructors should not require a separate app-level `providers.go` file by default. The default generated shape keeps command exposure in `app/commands.go`, framework command assembly in `app/wire/inject_cmd.go`, and app-owned command provider constructors in `app/wire/inject_cmd_app.go`. Other provider constructors live in the relevant `app/wire/inject_*.go` file, such as `app/wire/inject_http_controllers_app.go`, `app/wire/inject_jobs_app.go`, or `app/wire/inject_repositories_app.go`.

An app-level `providers.go` can still be created by users when they want a visible custom composition file, but it is not part of the default generator contract.

Entrypoints under `cmd/<app>/` should stay thin. They should start the generated command surface for the app, not own routes, jobs, schedules, provider sets, or business workflows.

An app is not the same thing as a runtime type. A `billing` app may expose HTTP routes, queue workers, scheduler entries, commands, and events from the same composition root. Split apps should represent a meaningful deployment or ownership boundary, not merely "API versus worker" files.

## Go `internal` Constraint

This model works naturally for one repository or Go module that builds multiple deployable apps.

Apps under the same module can share `internal/...` packages:

```text
platform/app/billing
platform/app/reporting
platform/internal/billing
```

If a team later splits apps into separate repositories or separate modules outside the parent tree, those modules cannot import the original project's `internal/...` packages. Shared code must move to a normal package or sibling module at that point.

Docs should be explicit about this. App fan-out is a clean monorepo or single-module scale-out path. It is not a cross-repo shared-library mechanism.

## Runtime Support Versus App Composition

Not every generated runtime file should move into `app/<app>/`.

Runtime support can remain in generated `internal/...` packages when it is reusable machinery:

- `internal/runtime` for lifecycle, timeout policy, discovery, and root runtime support
- `internal/runtime/apps.go` for framework-owned compiled app metadata
- `internal/http` for HTTP runtime implementation
- `internal/jobs` for worker runtime implementation
- `internal/schedules` for scheduler runtime implementation
- `internal/cmd` for reusable command plumbing when needed

App composition should live in the App composition layer:

- `app/commands.go` for the default app
- `app/root_cmd.go` for the default app
- `app/routes.go` for the default app
- `app/jobs.go` for the default app when a separate app-level job registry becomes useful
- `app/events.go` for the default app when a separate app-level event registry becomes useful
- `app/schedules.go` for the default app when a separate app-level schedule registry becomes useful
- `app/wire/...` for the default app
- `app/<app>/commands.go` for additional apps
- `app/<app>/root_cmd.go` for additional apps
- `app/<app>/routes.go` for additional apps
- `app/<app>/jobs.go` for additional apps when a separate app-level job registry becomes useful
- `app/<app>/events.go` for additional apps when a separate app-level event registry becomes useful
- `app/<app>/schedules.go` for additional apps when a separate app-level schedule registry becomes useful
- `app/<app>/wire/...` for additional apps

This keeps the App composition layer from becoming a dumping ground for business logic or low-level runtime implementation.

For commands specifically:

- `app/commands.go` defines the app's Kong command exposure.
- `app/wire/inject_cmd.go` provides framework command assembly and the app root command.
- `app/wire/inject_cmd_app.go` provides app-owned command constructors.
- generated `make:command` updates both files for the active app.
- legacy Apps can still fall back to `internal/cmd/app_commands.go` and `wire/inject_cmd.go`.

App metadata belongs in runtime support, not app composition. GoForj should generate a framework-owned file such as:

```text
internal/runtime/apps.go
```

That file should be regenerated on render and should not be app-owner edited. It exists so production binaries can resolve app defaults without reading `.goforj.yml` or scanning source directories.

Example shape:

```go
// Code generated by GoForj CLI. DO NOT EDIT.

package runtime

type AppInfo struct {
	Name        string
	Index       int
	EnvPrefix   string
	HTTPPort    int
	RuntimeBase int
}

var Apps = []AppInfo{
	{Name: "app", Index: 0, EnvPrefix: "", HTTPPort: 3000, RuntimeBase: 10000},
	{Name: "billing", Index: 1, EnvPrefix: "BILLING", HTTPPort: 3001, RuntimeBase: 10010},
	{Name: "customer-portal", Index: 2, EnvPrefix: "CUSTOMER_PORTAL", HTTPPort: 3002, RuntimeBase: 10020},
}
```

The renderer should discover apps by convention, sort named apps deterministically, and compile the resulting manifest into every app binary. App owners should customize local/runtime ports through env overrides, not by editing this generated file.

## Entrypoints

Use `cmd/<app>/` for Go executable entrypoints.

This follows the common Go convention that `cmd/<name>` maps to an executable named `<name>` while keeping substantial application composition outside `cmd/`.

Single-app project:

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

Multi-app project:

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

`cmd/<app>/main.go` should generally do little more than invoke that app's generated root command or runtime launcher.

Do not move the composition layer into `cmd/<app>/`. `cmd/` is the binary entrypoint layer. `app/` is the GoForj composition layer.

## Package Naming

Use simple package names that follow Go naming rules.

Suggested rules:

- `app/` uses package `app`.
- `app/<app>/` uses a Go-safe package name derived from the app name.
- `app/wire/` and `app/<app>/wire/` use package `wire`.
- `cmd/<app>/main.go` uses package `main`.

App names may be user-facing slugs for CLI and binary names, but generated Go package names must be valid identifiers.

Example:

```text
app: customer-portal
entrypoint: cmd/customer-portal/main.go
composition path: app/customer-portal/
binary: bin/customer-portal
package name: customerportal
```

This keeps the CLI and filesystem readable without forcing hyphenated app names into invalid Go package names.

## CLI Model

Avoid spreading `--app` across normal workflows.

The command shape should use app prefixes:

```bash
forj <app> <command>
```

If the first argument matches a conventional app, `forj` enters that app context and resolves the rest of the command against that app.

The source of truth for normal app dispatch is convention, not configuration:

- default app: `cmd/app/main.go`, `app/`, `app/wire/`
- named app: `cmd/<app>/main.go`, `app/<app>/`, `app/<app>/wire/`

`.goforj.yml` should not describe which apps exist. App discovery, app dispatch, runtime metadata, and all-app dev fanout should use project layout convention rather than a configured app list.

The implementation also supports binary dispatch outside the source-tree path: if only `./bin/<app>` exists, `forj <app> ...` delegates to that binary with the remaining arguments. During dispatch, GoForj sets app identity environment values such as `FORJ_COMMAND_PREFIX=forj <app>` and `FORJ_APP=<app>`.

Examples:

```bash
forj app --help
forj app route:list
forj billing --help
forj billing route:list
forj billing worker --queue invoices
forj reporting scheduler
```

When no app is specified, `forj` uses the default app, which is normally `app`:

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

The built binary shape should match app names:

```bash
./bin/app
./bin/app run
./bin/billing api
./bin/billing worker
./bin/reporting scheduler
```

Those binaries are built from `cmd/<app>/main.go`. Bare execution enters `run`
when that specific App is runtime-capable; CLI-only Apps print help.

`forj <app> --help` should behave like asking that app for help. In a built context, this is conceptually equivalent to:

```bash
./bin/<app> --help
```

In source-aware development, `forj` delegates through the generated app command surface when `cmd/<app>/main.go` exists, even if `./bin/<app>` also exists. That keeps source-mutating commands such as `make:*` from running through a stale built app binary.

### Command Resolution

Suggested resolution order:

1. If the first argument is a native Framework command, run the native command.
2. Else if the first argument matches `cmd/<app>/main.go` in a generated project, set the active app and resolve the remaining command normally in that app context.
3. Else if the first argument matches a built app binary at `./bin/<app>`, delegate the remaining arguments to that binary.
4. Else resolve the command against the default app.

App names should not be allowed to collide with native Framework commands such as `build`, `dev`, `render`, or `version`.

If no app is specified, command resolution behaves like the normal single-App path and uses the implicit default app.

After an app prefix, normal command resolution should still apply. Native Framework commands operate against the active app, and generated App commands delegate to that app's command surface.

Inside an app context, app-aware native commands may use the active app:

```bash
forj billing build
forj reporting dev
```

Those should be equivalent to app-scoped native operations, without requiring:

```bash
forj build --app billing
forj dev --app reporting
```

If a generated App command collides with a native app-aware command, native commands should keep precedence. A collision escape hatch can remain explicit if needed.

Current implementation note: source-mode app dispatch is convention-first. `forj <app> build` and `forj <app> run` use `cmd/<app>` and `app/<app>/wire` when those paths exist.

## Build And Dev

Single-app projects should keep the current ergonomic path:

```bash
forj build
forj dev
```

Those operate on the default app.

Multi-app projects should support app prefixes:

```bash
forj billing build
forj reporting build
forj billing dev
forj reporting dev
```

Most unqualified commands should operate on the default app only:

- `forj build` builds the default app.
- `forj test:render` validates the default app unless the command explicitly opts into all-app validation.
- `forj build:all` or another explicit command can build every app.

`forj dev` uses the sparse `dev.apps` lifecycle configuration. Unqualified
`forj dev` orchestrates only the Apps listed there; filesystem or App discovery
must not silently enroll omitted Apps. A listed runtime-capable App builds and
launches its bare binary by default, while a listed CLI-only App builds without
launching. This keeps participation explicit without turning `.goforj.yml` into
the source of truth for which Apps exist.

Shutdown should behave like orchestration, not a serial script. When `forj dev` exits, restarts, or rerenders, all running watcher subprocesses should receive the shutdown signal in parallel and then be awaited as a group. A slow worker or scheduler in one app should not delay another app from receiving its interrupt. The shutdown budget should be bounded by the slowest subprocess, not by the sum of every subprocess timeout.

Generated local infrastructure should also avoid holding up dev shutdown. Long-running Docker Compose services can keep normal graceful shutdown behavior, but one-shot helper containers should not inherit a long default stop grace period. For example, `grafana-seed` is an idempotent dashboard preference helper and should use a short Compose `stop_grace_period` so Ctrl+C does not appear stuck on that helper container.

App-prefixed dev remains scoped:

```bash
forj billing dev
```

That command should watch, build, and run only the selected app.

## Generator Feel

Generator commands should still feel app-local and familiar.

In a single-app project:

```bash
forj make:controller billing:reports
forj make:command reports:sync -d ./internal/billing/reports
forj make:job billing:reports:generate --queue reports
forj make:event billing:invoice-paid -d ./internal/billing/events
```

Those commands create implementation under `internal/...` and update the default app under `app/...`.

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

`app/commands.go` exposes the command on the app root command. `app/wire/inject_cmd_app.go` registers the app-owned command constructor with Wire. The framework-owned `app/wire/inject_cmd.go` should remain stable and include `appCommandSet`. The generator should not create or require `app/providers.go` for command registration.

In a multi-app project, the app prefix should scope registration:

```bash
forj billing make:controller billing:invoices
forj billing make:job billing:invoice-reminders --queue invoices
forj billing make:schedule billing:daily-settlement --every 24h
forj reporting make:job reports:generate --queue reports
```

These commands should create domain-owned files under `internal/...` and update only the selected app's registration and Wire files.

The generator should also support a way to create implementation without registration when needed, but that should be an advanced workflow. The golden path should create and register into the active app.

## App Convention Model

Project config should stay focused on project-wide settings:

- module name
- render settings
- component availability
- supported drivers
- dependency versions
- shared local development defaults

App metadata is derived from layout convention:

- app name
- entrypoint path
- composition directory
- Wire directory

When no named app layout exists, GoForj should synthesize the single-app default rather than requiring config boilerplate.

Do not keep `app.apps` or `app.default_app` in `.goforj.yml` unless a future feature proves it needs explicit configuration and cannot be expressed by layout or environment overrides.

Per-app component participation is allowed, but it should not become app discovery. A named app is discovered from `cmd/<app>/main.go` and `app/<app>/`; `.goforj.yml` may persist only the app's selected component slice and starter-kit choice:

```yaml
apps:
  billing:
    components:
      web_api: true
      jobs: true
    starter_kit: none
  customer-portal:
    components:
      web_api: true
      web_ui: true
    starter_kit: vue
```

The project-level component set is the currently rendered support surface, not a hard ceiling for future apps. `make:app` may promote newly selected app-safe capabilities into the project render set. For example, a project that starts with MySQL can create a `reporting` app that uses Postgres; the app keeps an exclusive database driver selection while the project records both drivers as supported.

`make:app` should reuse the same component catalog and starter-kit catalog as `forj new`, filtered to app-safe choices. The interactive app wizard should show the app-affecting choices that can be compiled into the project, not only the choices used by the default app. This keeps the additional app creation flow from feeling artificially constrained by the first app's choices.

The interactive app wizard should stay focused on choices that change the generated app surface: Web API, Web UI, Auth, OAuth, database driver, scheduler, and jobs. Auth and OAuth are not separate deployable runtimes, but they do affect route exposure, Wire inputs, generated environment, and supporting dependencies, so hiding them would make app creation less transparent. Synthetic stress tooling should live in a separate harness instead of becoming part of app composition.

Project-only capabilities such as Docker, Observability, Grafana, and Demo App stay project-level. They can be available because the project has them, but they should not become per-app toggles unless a later feature gives them app-specific runtime meaning.

`forj make:app <app>` should support both the no-friction default and explicit app shape:

```bash
forj make:app billing
forj make:app billing --components web-api,jobs
forj make:app customer-portal --components web-api,web-ui --starter-kit vue
forj make:app worker --without web-ui
```

When no component flags are provided from an interactive terminal, `make:app` opens the app wizard by default. Non-interactive runs mirror the app-surface components already enabled for the project. Explicit `--components` starts from the provided app-safe component set and skips the wizard. `--without` removes choices from the default app slice and also skips the wizard. The wizard is just an ergonomic frontend over the same component and starter-kit rules.

Database choices are app-exclusive but project-supported. Selecting `Database (Postgres)` for one app should clear other database choices for that app, add Postgres to the project-supported driver set, add app-scoped env defaults such as `REPORTING_DB_DRIVER=postgres`, extend `DB_SUPPORTED_DRIVERS` in the base `.env`, add host-safe app overrides in `.env.host`, and leave the default App's root `DB_DRIVER` alone.

App names should be validated as path-safe slugs and should not collide with generated files directly under `app/`.

App-specific env should be conservative. It is useful for process ports, worker counts, runtime toggles, and observability identity. It should not force every shared resource to be configured separately unless the app intentionally uses a different resource.

## Runtime Topology

App split changes deployment topology, not business logic ownership.

An app may support one or more logical runtimes:

- HTTP/API runtime
- worker runtime
- scheduler runtime
- CLI command runtime
- Lighthouse/runtime visibility surfaces

The default app can run combined local development:

```bash
forj app run
./bin/app run
```

Production apps can run leaf runtimes:

```bash
./bin/billing api
./bin/billing worker
./bin/billing scheduler
./bin/reporting worker
```

Docs should keep standalone and distributed behavior distinct. Process-local drivers remain process-local. Shared behavior across distributed apps requires shared infrastructure.

## App Runtime Ports

Multi-app local development should not require manual port editing just because several apps or runtimes start together.

Port defaults should be deterministic by convention:

- app index: `app = 0`; named apps sorted alphabetically start at `1`.
- HTTP/API port: `3000 + appIndex`.
- runtime port block: `10000 + (appIndex * 10)`.

The runtime port block is intentionally separate from the HTTP/API range because each app can run multiple listener-owning runtimes at the same time.

Bundled local development tools should avoid the app HTTP range. For example, Grafana defaults to `13001` instead of `3001` so the first named app can use the natural `3000 + appIndex` default.

Within each app runtime block:

- HTTP metrics/runtime listener: `runtimeBase + 0`
- scheduler metrics/runtime listener: `runtimeBase + 1`
- worker metrics/runtime listener: `runtimeBase + 2`
- future runtime listeners can use the remaining block slots before the next app starts.

Example:

```text
app
  PORT=3000
  METRICS_PORT=10000
  SCHEDULER_METRICS_PORT=10001
  WORKER_METRICS_PORT=10002

billing
  BILLING_PORT=3001
  BILLING_METRICS_PORT=10010
  BILLING_SCHEDULER_METRICS_PORT=10011
  BILLING_WORKER_METRICS_PORT=10012

customer-portal
  CUSTOMER_PORTAL_PORT=3002
  CUSTOMER_PORTAL_METRICS_PORT=10020
  CUSTOMER_PORTAL_SCHEDULER_METRICS_PORT=10021
  CUSTOMER_PORTAL_WORKER_METRICS_PORT=10022
```

These metric env keys are override examples, not required rendered `.env` output. Generated app metadata owns the deterministic defaults; app owners only add app-scoped metric env vars when they want to override a port.

Runtime port resolution should prefer the most specific env value and then fall back to deterministic defaults:

```text
HTTP:
  <APP>_PORT
  <APP>_API_HTTP_PORT
  PORT or API_HTTP_PORT for the default app only
  3000 + appIndex

HTTP metrics:
  <APP>_METRICS_PORT
  <APP>_API_METRICS_PORT or <APP>_METRICS_API_PORT
  METRICS_PORT, API_METRICS_PORT, or METRICS_API_PORT for the default app only
  10000 + (appIndex * 10)

Scheduler metrics:
  <APP>_SCHEDULER_METRICS_PORT
  <APP>_METRICS_SCHEDULER_PORT
  <APP>_METRICS_PORT
  SCHEDULER_METRICS_PORT, METRICS_SCHEDULER_PORT, or METRICS_PORT for the default app only
  10000 + (appIndex * 10) + 1

Worker metrics:
  <APP>_WORKER_METRICS_PORT
  <APP>_JOBS_METRICS_PORT or <APP>_METRICS_JOBS_PORT
  <APP>_METRICS_PORT
  WORKER_METRICS_PORT, JOBS_METRICS_PORT, METRICS_JOBS_PORT, or METRICS_PORT for the default app only
  10000 + (appIndex * 10) + 2
```

Single-app projects keep the existing user experience. Multi-app projects should be able to run `forj dev` and have every app start with non-conflicting default HTTP and runtime ports unless the user explicitly overrides them.

The compiled app manifest should provide the deterministic fallback values. Environment variables remain the override mechanism:

- `<APP>_PORT` overrides one app's HTTP port.
- `<APP>_METRICS_PORT` overrides one app's default metrics/runtime port.
- `<APP>_<RUNTIME>_METRICS_PORT` overrides one runtime listener for one app.
- global variables such as `PORT`, `API_HTTP_PORT`, `METRICS_PORT`, `METRICS_SCHEDULER_PORT`, and `METRICS_JOBS_PORT` remain useful for single-app projects and the default app, but named apps do not consume those globals by default because existing `.env` files would otherwise create listener collisions.

## Observability And Lighthouse

Every app needs stable runtime identity.

Logs, metrics, inspects, health, readiness, and Lighthouse payloads should preserve:

- Project name
- App name
- runtime name
- process or instance identity when relevant

Local observability app generation should use the conventional app layout, not `.goforj.yml`, as the source of app existence:

- `local-single` writes one scrape target per app. When the HTTP source exists, each app is scraped through its HTTP port (`3000`, `3001`, `3002`, ...). If an app has no HTTP source, generation falls back to that app's shared metrics port.
- `local-multi` writes one scrape target per app and runtime source. Ports come from the deterministic runtime block (`10000`, `10001`, `10002` for `app`; `10010`, `10011`, `10012` for the first named app; etc.).
- every generated scrape entry includes `app`, `process`, `service`, and `environment` labels.
- app component config may filter source roles for an app, but app existence is still discovered from `cmd/<app>` and `app/<app>` conventions.

Use **app** in docs and UI. A GoForj project can contain multiple apps, and each app can run multiple runtimes. For machine-readable fields, use `project`, `app`, `runtime`, and `instance`.

Examples:

```text
project="platform", app="billing", runtime="http"
app_name="platform", app="billing", runtime="worker"
app_name="platform", app="billing", runtime="scheduler"
app_name="platform", app="reporting", runtime="worker"
```

Lighthouse should consume app-aware runtime metadata. It should not infer app boundaries from filenames.

Lighthouse agent identity should include:

- `app_name`: the generated GoForj App or project identity
- `app`: the composition app inside the App, such as `app`, `billing`, or `reporting`
- `runtime`: the logical runtime, such as `http`, `worker`, `scheduler`, or `cli`
- `instance_id`: a process, host, PID, generated run ID, or other stable-enough process instance identity
- `environment`: local, test, staging, production, or another configured environment label when available

For a single-app local App, the identity is still simple:

```text
app_name="platform", app="app", runtime="http"
```

For multi-app local development, each running app/runtime process should connect as its own agent:

```text
app_name="platform", app="billing", runtime="http"
app_name="platform", app="billing", runtime="worker"
app_name="platform", app="reporting", runtime="worker"
```

Lighthouse should keep separate keys for the logical runtime group and the concrete runtime instance:

```text
group_key="billing/jobs"
key="billing/jobs/worker-01"
key="billing/jobs/worker-02"
```

The default single-app group key remains `http`, `jobs`, or `scheduler` for simple local ergonomics. The concrete agent key can still include host or instance identity when multiple replicas connect.

The UI should group runtime data as:

```text
Project -> App -> Runtime -> Instance
```

When `app="app"` is the only app, Lighthouse may collapse the app level so the default single-app experience does not feel more complex.

App-aware Lighthouse behavior should make it clear:

- which routes belong to which app
- which jobs belong to which app
- which schedules belong to which app
- which resources are shared
- which resources are unavailable or degraded for a specific app

## API Index And OpenAPI

API indexing is app-aware. It follows the active App's returned route
composition, so routes exposed by `billing` are not mixed with routes exposed
only by `reporting`.

Single-app projects use the unqualified build command. Multi-app projects use
the normal App prefix:

```bash
forj build:api-index
forj billing build:api-index
forj reporting build:api-index
```

The default App writes `build/api_index.json`,
`build/api_index.diagnostics.json`, and `build/openapi.json`. A named App writes
the same files below `build/<app>/`. `forj build` and `forj run` also prepare the
active App's artifacts, but publish them only after the final compile or process
start succeeds. `build:api-index --strict` and the pipeline
`--api-index-strict` flag reject warnings as well as errors.

An App explicitly configured without WebAPI produces no API index and stale
artifacts are cleaned. An App configured with WebAPI but missing its
`app[/<app>]/routes.go` composition fails instead of widening to a whole-project
index.

See [Forj API Index Design](forj-api-index-design.md) for route-provider
identity, Manifest v2, schema inference, OpenAPI projection, and publication
semantics.

## Migrations And Database

Migration files should stay project-level, but their hierarchy changes once a project has more than one App.

Single-app projects keep the existing simple layout:

```text
migrations/
  2026_06_05_120000_create_users.up.sql
  2026_06_05_120000_create_users.down.sql

  reporting/
    2026_06_05_121000_create_reports.up.sql
    2026_06_05_121000_create_reports.down.sql
```

In that shape:

- `migrations/` is the default connection.
- `migrations/<connection>/` is a named connection.

When a second App is created, migrations should expand into an explicit app-and-connection hierarchy:

```text
migrations/
  app/
    default/
      2026_06_05_120000_create_users.up.sql
      2026_06_05_120000_create_users.down.sql
    reporting/
      2026_06_05_121000_create_reports.up.sql
      2026_06_05_121000_create_reports.down.sql

  billing/
    default/
      2026_06_05_122000_create_invoices.up.sql
      2026_06_05_122000_create_invoices.down.sql
    ledger/
      2026_06_05_123000_create_ledger_entries.up.sql
      2026_06_05_123000_create_ledger_entries.down.sql
    archive/
      2026_06_05_124000_create_invoice_archive.up.sql
      2026_06_05_124000_create_invoice_archive.down.sql
```

In the multi-app shape:

- `migrations/<app>/` is the owning App.
- `migrations/<app>/<connection>/` is the connection stream for that app.
- `default` is an explicit connection directory.
- If two apps share the same physical database, only one app should own that database's migration source.

This keeps single-app projects ergonomic while making monorepo service fan-out obvious. It also avoids overloading `migrations/<name>/` with both connection names and app names once multiple apps exist.

App-scoped migration connections still need to map onto the generated flat database connection registry:

- `migrations/app/default/` uses database connection `default`.
- `migrations/app/reporting/` uses database connection `reporting`.
- `migrations/billing/default/` uses database connection `billing`.
- `migrations/billing/ledger/` uses database connection `billing_ledger`.

That means app-specific default databases naturally use env scopes such as `DB_BILLING_DRIVER`, while app-specific named connections use scopes such as `DB_BILLING_LEDGER_DRIVER`.

Migration command exposure is app-level, and migration execution is orchestration-aware:

```bash
forj migrate
forj billing migrate
forj billing migrate --connection ledger
```

For a single-app project, `forj migrate` runs the default app's migrations.

For a multi-app project, unqualified `forj migrate` should infer the all-app workflow. It should discover every app migration stream, build an app/connection migration plan, and run each owned stream once. A separate `migrate:all` command is not necessary for the primary workflow unless an explicit alias proves useful later.

App-prefixed migration commands should scope execution to the selected app. For example, `forj billing migrate` should run every connection under `migrations/billing/*`, while `forj billing migrate --connection ledger` should run only `migrations/billing/ledger`.

For example, the default app may expose migration commands for local development, while production apps may omit them unless explicitly configured.

This avoids every generated binary casually exposing schema-changing commands by accident.

Migration tracking and locking should carry enough identity to explain what ran:

- App name
- connection name
- migration source path
- generated database connection name

The physical database still owns its migration history table and lock. The app, logical connection, source path, and generated database connection name make route planning, logging, and Lighthouse/runtime visibility understandable without treating apps as database schemas.

The runner does not need to fingerprint DSNs or dedupe streams by physical database identity right now. If two apps point at the same physical database, the project should give that database one owning migration stream.

## Auth, Frontend, And Starter Kits

Generated product surfaces must separate implementation from exposure.

Auth implementation may live under `internal/auth`, while auth routes are exposed by one or more app HTTP runtimes.

Background auth behavior, such as cleanup schedules or mail jobs, may be exposed by an app's worker or scheduler runtimes.

Frontend source and build output should live next to the command package that embeds it: `cmd/<app>/frontend`. That keeps `npm run build` writing directly to the embedded `frontend/dist` path for the active app and avoids a separate publish/copy step for the common case.

Frontend starter kits should point at a specific app HTTP runtime. A project that later moves background work into another app should not need to rewrite frontend-owned code just because worker or scheduler composition moved. Generated starter kits may use `FRONTEND_BACKEND_URL` or `<APP>_FRONTEND_BACKEND_URL` to override the local proxy app when the frontend needs to talk to a non-default HTTP app.

Root `.env` remains the source for generated frontend configuration. Browser-visible values must opt in through frontend-specific prefixes and are transposed by Vite into `import.meta.env.VITE_*`:

- `FRONTEND_*` applies to every frontend in every app.
- `<APP>_FRONTEND_*` applies to one app, such as `CUSTOMER_PORTAL_FRONTEND_BACKEND_URL`.

This keeps server-only variables private by default while avoiding per-key manual mapping in Vite config. The default frontend remains `cmd/<app>/frontend`. Multiple SPAs are intentionally left as a future design pass.

## What Should Not Happen

Do not introduce a new "service" abstraction just to organize files.

Avoid:

```text
services/billing
services/identity
services/notifications
```

unless each entry is truly a separate GoForj project with separate repository, release, runtime, and configuration ownership.

Also avoid a parallel module registry abstraction unless there is a concrete problem Apps cannot solve.

Do not let `app/` or `app/<app>/` accumulate business logic. The composition layer should compose and expose application behavior; it should not own domain workflows.

## Benefits

- Gives users one obvious place to inspect app composition.
- Keeps the single-App case as the default.
- Keeps the single-App layout clean: `cmd/app`, `app`, `internal`.
- Gives larger Apps a clear fan-out path.
- Aligns executable entrypoints with the common Go `cmd/<name>` convention.
- Keeps domain packages focused on implementation.
- Avoids pretending large internal packages are independent services.
- Makes generated registration files feel intentional rather than hidden.
- Keeps `forj` commands app-local and familiar.
- Avoids `--app` flags on every command.
- Makes docs clearer: "The default app lives in `app/`; additional apps live in `app/<app>/`; implementation lives in `internal/`."

## Risks

- The App composition layer could become a dumping ground if it accepts business logic.
- Mixing default app files and named app directories under `app/` creates reserved-name concerns.
- Moving Wire into `app/wire/` and `app/<app>/wire/` requires import path and generated template churn.
- Existing docs and tests reference `wire/` and `internal/*` registration paths heavily.
- Command resolution becomes more complex once app names enter the top-level `forj` grammar.
- App names can collide with native Framework commands unless validation prevents it.
- Users may expect apps to imply separate repositories or microservices.
- The migration could be disruptive for existing generated Apps.

## Migration Direction

This should not be a sudden breaking layout change.

Possible path:

1. Define App conventions, with `app` as the default app. Done for the default app and source-mode app dispatch.
2. Add new templates for `cmd/app/`, `app/`, and `app/wire/` in new Apps. Done for the default app.
3. Keep compatibility with current `internal/` and `wire/` registration files for existing Apps. Done through generator path fallback and legacy cleanup.
4. Teach generators to detect which layout exists. Done for the default app generators.
5. Add app detection and command resolution to `forj`. Done for source-mode convention dispatch and binary fallback dispatch.
6. Add app-aware generator registration. Done for default and named app-owned generator registration.
7. Add app-aware build, dev, API index, OpenAPI, metrics identity, and Lighthouse metadata. Partially done for default-app build/run/wire paths, source-mode app build/run/wire paths, all-app unqualified dev orchestration, standalone and pipeline API-index/OpenAPI generation with App-scoped artifacts, and Lighthouse agent identity.
8. Add rendered smoke scenarios for single-app and multi-app Apps. Done for default and named-App render coverage, including App-scoped API-index artifacts and serving behavior.
9. Update docs to describe Apps as the preferred generated shape. In progress.
10. Consider a migration command only after the new layout has proven itself.

This project has not launched publicly yet, so migration urgency is lower than choosing the right default shape. Legacy detection still matters for local rendered Apps and existing internal smoke apps.

Generator detection could be:

- if the active app is the default app and `app/commands.go` exists, update the default app layout
- else if `app/<app>/commands.go` exists, update the named app layout
- else update the legacy `internal/cmd/app_commands.go`
- if the active app is the default app and `app/wire/` exists, update `app/wire/*`
- else if `app/<app>/wire/` exists, update `app/<app>/wire/*`
- else update `wire/*`

Command generator detection now follows that shape for the default app:

- command implementation is generated under `internal/...`
- command exposure is added to `app/commands.go`
- command provider construction is added to `app/wire/inject_cmd_app.go`
- if the new layout is absent, the generator falls back to legacy command files

## Reserved Names

Keep reserved names under `app/` minimal.

The required reserved name is:

- `wire`

That prevents an additional app from colliding with the default app's `app/wire/` support directory.

App names should also be validated as path-safe slugs and should not collide with generated files directly under `app/`.

## Implementation Tasks

Track implementation as concrete work items:

- [x] Add the default App model to project layout helpers.
  - [x] Synthesize the implicit single-app default when no named app layout exists.
  - [x] Derive app metadata for `name`, `entrypoint`, `app_dir`, and `wire_dir` from convention.
  - [x] Remove `app.apps` and `app.default_app` from project configuration logic.
  - [x] Validate discovered app names for stable project-layout constraints.
  - [x] Validate app names as path-safe slugs.
  - [x] Reject app names that collide with native Framework commands.
  - [x] Reject app names that collide with reserved `app/` names such as `wire`.
  - [x] Reject app names that collide with generated files directly under `app/`, such as `commands.go`, `root_cmd.go`, and `routes.go`.

- [x] Render the new single-app layout for new Apps.
  - [x] Generate `cmd/app/main.go`.
  - [x] Generate default app composition under `app/`.
  - [x] Generate default app Wire files under `app/wire/`.
  - [x] Render the default app through the same app renderer used by named apps.
  - [x] Keep generated runtime support packages under `internal/...` where they are reusable machinery.
  - [x] Do not generate `app/providers.go` by default.
  - [x] Co-locate frontend source and embedded build output under `cmd/app/frontend`.
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

- [x] Add named app rendering.
  - [x] Discover named apps from existing conventional `cmd/<app>/main.go` / `app/<app>/` layouts instead of `.goforj.yml`.
  - [x] Generate `cmd/<app>/main.go`.
  - [x] Generate named app composition under `app/<app>/`.
  - [x] Generate `app/<app>/commands.go`.
  - [x] Generate `app/<app>/root_cmd.go`.
  - [x] Generate `app/<app>/routes.go` when the app has HTTP enabled.
  - [x] Generate `app/<app>/schedules.go` when the scheduler is enabled.
  - [x] Generate named app Wire files under `app/<app>/wire/`.
  - [x] Derive Go-safe package names from app slugs.
  - [x] Ensure hyphenated app slugs map to legal package names.
  - [x] Ensure named app entrypoints import the correct app composition package.
  - [x] Keep named app `wire.go` editable-but-overwrite-rendered like the default app.
  - [x] Keep named app-owned injectors render-once.
  - [x] Generate app-local frontend source/build paths under `cmd/<app>/frontend` when Web UI is enabled.
  - [x] Keep named app cleanup non-destructive by only writing known generated files and preserving app-owned files after first render.
  - [x] Add `forj make:app <app>` to create a named app from convention.
  - [x] Keep `make:app` narrow by rendering only the new app scaffold, runtime app metadata, migration layout updates, and Wire generation.
  - [x] Support per-app component selection from the app-safe component catalog.
  - [x] Promote newly selected app-safe capabilities into the project render set when a named app needs them.
  - [x] Keep database driver choices exclusive per app while allowing multiple project-supported database drivers.
  - [x] Persist per-app component and starter-kit choices without using config as app discovery.
  - [x] Scope the `make:app` command and interactive app wizard under `internal/forj/makeapp`.
  - [x] Add `forj make:app <app> --remove` for conservative removal of conventional app files, persisted app choices, built app binaries, and regenerated runtime app metadata.
  - [x] Keep `make:app --remove` from deleting app migrations or unknown command-package files.

- [x] Update command resolution.
  - [x] Detect `forj <app> ...` when `./bin/<app>` exists.
  - [x] Delegate remaining arguments to the built app binary when source layout is not present.
  - [x] Set app identity env during binary dispatch.
  - [x] Keep unqualified commands on the default app.
  - [x] Set the active app from `cmd/<app>/main.go` before binary dispatch in source projects.
  - [x] Route `forj <app> --help` through the source app in source mode.
  - [x] Preserve native Framework command precedence for convention source-mode app dispatch.
  - [x] Make `forj <app> <native-command>` pass active app context into native commands.
  - [x] Make `forj <app> <app-command>` execute the selected app command surface.
  - [ ] Keep an explicit collision escape hatch for generated App commands if needed.
  - [x] Add tests for convention app dispatch without a prebuilt binary.
  - [x] Add tests for native-command precedence over app and App command names.

- [ ] Partial: update build and dev workflows.
  - [x] Make `forj build` and `forj run` operate on `cmd/app` by default.
  - [x] Prefer `app/wire` as the default Wire path with legacy `wire` fallback.
  - [x] Build the default app binary from `cmd/app/main.go`.
  - [x] Make `forj build` app-aware in source mode.
  - [x] Make `forj run` app-aware in source mode.
  - [x] Make `forj dev` fully active-app aware for named apps.
  - [x] Make unqualified `forj dev` expand default Build/Run watchers across every discovered app.
  - [x] Keep `forj <app> dev` scoped to the selected app.
  - [ ] Add explicit all-app build or render validation when needed.
  - [x] Ensure source-mode `forj <app> dev` operates on the active conventional app.
  - [ ] Add `build:all` or equivalent only if the workflow proves useful.
  - [x] Ensure Wire generation runs in the selected conventional app's `wire_dir` when it exists.
  - [x] Ensure full render runs Wire generation for every discovered app Wire directory.
  - [x] Ensure dev build/watch uses the selected app entrypoint and binary when a named app owns HTTP.
  - [x] Shut down all `forj dev` watcher subprocesses in parallel on Ctrl+C, restart, and render-triggered restarts.
  - [x] Keep one-shot Compose helper containers from delaying dev shutdown, starting with a short `grafana-seed` stop grace period.
  - [x] Generate framework-owned compiled app metadata in `internal/runtime/apps.go`.
  - [x] Keep `internal/runtime/apps.go` regenerated on render and document that app owners should not edit it.
  - [x] Add deterministic app indexes for runtime defaults: `app = 0`, named apps sorted alphabetically from `1`.
  - [x] Add app-aware HTTP port defaults: `3000 + appIndex`.
  - [x] Move bundled Grafana's default host port out of the app HTTP range to `13001`.
  - [x] Add app-aware runtime port blocks: `10000 + (appIndex * 10)`.
  - [x] Resolve HTTP ports from app-scoped env, default-app env, then deterministic app default.
  - [x] Resolve HTTP metrics ports from app-scoped env, default-app env, then deterministic app runtime default.
  - [x] Resolve scheduler metrics ports from app-scoped env, default-app env, then deterministic app runtime default plus `1`.
  - [x] Resolve worker metrics ports from app-scoped env, default-app env, then deterministic app runtime default plus `2`.
  - [x] Ensure production binaries use compiled app metadata instead of `.goforj.yml` or source directory scanning.
  - [x] Ensure all-app `forj dev` can start HTTP, scheduler, and worker runtimes without listener collisions by default.
  - [x] Add integration coverage that renders three apps, starts each app's HTTP, scheduler, and worker runtimes, and verifies HTTP plus metrics ports respond without collisions.

- [x] Partial: update generators.
  - [x] Register generated controllers, commands, jobs, events, schedules, repositories, and Wire entries into the default app layout.
  - [x] Keep generated implementation under `internal/...`.
  - [x] Preserve legacy layout detection for existing internal smoke apps.
  - [x] Move `make:command` provider constructor injection to `app/wire/inject_cmd_app.go`.
  - [x] Keep command exposure injection in `app/commands.go`.
  - [x] Add tests and rendered smoke coverage for default-app generation.
  - [x] Make `make:controller` update `app/routes.go` and `app/wire/inject_http_controllers_app.go` for the default app.
  - [x] Make `make:job` update `app/wire/inject_jobs_app.go` for the default app.
  - [x] Make `make:schedule` update `app/schedules.go` and `app/wire/inject_schedules_app.go` for the default app.
  - [x] Make `make:subscriber` update `app/wire/inject_subscribers_app.go` for the default app.
  - [x] Make `make:model` update `app/wire/inject_repositories_app.go` for the default app.
  - [x] Add active-app context plumbing shared by all generators.
  - [x] Make `make:controller` update `app/<app>/routes.go` and `app/<app>/wire/inject_http_controllers_app.go`.
  - [x] Make `make:command` update `app/<app>/commands.go` and `app/<app>/wire/inject_cmd_app.go`.
  - [x] Make `make:job` update `app/<app>/wire/inject_jobs_app.go`.
  - [x] Make `make:schedule` update `app/<app>/schedules.go` and `app/<app>/wire/inject_schedules_app.go`.
  - [x] Make `make:subscriber` update `app/<app>/wire/inject_subscribers_app.go`.
  - [x] Make `make:model` update `app/<app>/wire/inject_repositories_app.go` for the active app.
  - [x] Make `make:model` default to active app only.
  - [ ] Add an advanced generate-without-registration mode for implementation-only files.
  - [x] Add named-app generator tests for controller, command, job, schedule, subscriber, and model flows.
  - [ ] Preserve legacy fallback behavior for old `wire/` and old app injector filenames.

- [ ] Partial: move app-level registration out of runtime support packages.
  - [x] Move route registration to `app/routes.go` for the default app.
  - [x] Move command exposure to `app/commands.go` for the default app.
  - [x] Keep framework command assembly in `app/wire/inject_cmd.go` for the default app.
  - [x] Move app-owned command provider construction to `app/wire/inject_cmd_app.go` for the default app.
  - [x] Move controller, job, schedule, event subscriber, repository, and app service Wire files to `app/wire/` for the default app.
  - [x] Move lifecycle hook registration from `internal/app/lifecycle_registry.go` to `app/lifecycle.go`.
  - [x] Move schedule registration from `internal/schedules/scheduler_registry.go` to `app/schedules.go`.
  - [x] Keep scheduler observer and runtime registration plumbing in `internal/schedules/registration.go`.
  - [x] Keep generated cache, queue, storage, event, mail, and database managers under `internal/...` as shared runtime support.
  - [ ] Move job registration to `app/jobs.go` and `app/<app>/jobs.go` if a separate job registry file becomes useful.
  - [ ] Move event registration to `app/events.go` and `app/<app>/events.go` if a separate event registry file becomes useful.
  - [x] Move schedule registration to `app/schedules.go` for the default app.
  - [x] Move schedule registration to `app/<app>/schedules.go` for named apps.
  - [ ] Decide whether current Wire-only registration for jobs, events, and schedules is sufficient long term.
  - [x] Keep HTTP, worker, scheduler, lifecycle, and command runtime machinery in generated `internal/...` packages for now.

- [ ] Partial: update app-aware product surfaces.
  - [x] Ensure auth routes and auth command wiring register through default app composition.
  - [x] Ensure starter kits can serve from the default app entrypoint.
  - [x] Scope API index and OpenAPI generation to the active named app.
  - [x] Tighten `webindex` app scoping so included owners are derived from route groups returned by the active composition file, not every `.Routes()` call that appears in the file.
  - [x] Decide whether missing active app route composition should fail fast instead of silently falling back to whole-project API indexing.
  - [x] Add webindex fixtures for dead/local `.Routes()` calls in composition files so app-scoped OpenAPI cannot leak unrelated routes.
  - [x] Scope route lists to the active named app.
  - [x] Decide the multi-app migration hierarchy: single-app projects use `migrations/` and `migrations/<connection>/`; multi-app projects use `migrations/<app>/<connection>/`.
  - [x] Move existing single-app migrations into `migrations/app/default/` and `migrations/app/<connection>/` when the first additional app is created.
  - [x] Make `make:migration` app-aware so `forj <app> make:migration ...` writes into `migrations/<app>/default/`.
  - [x] Make `make:migration --connection <name>` write into `migrations/<app>/<connection>/` for active multi-app projects.
  - [x] Preserve the existing single-app migration paths until a second app exists.
  - [x] Make unqualified `forj migrate` run the default app in single-app projects.
  - [x] Make unqualified `forj migrate` infer all-app migration orchestration in multi-app projects.
  - [x] Make `forj <app> migrate` run every connection under `migrations/<app>/*`.
  - [x] Make `forj <app> migrate --connection <name>` run only `migrations/<app>/<connection>`.
  - [x] Map app-scoped migration streams onto flat database connection names: `app/default -> default`, `app/<connection> -> <connection>`, `<app>/default -> <app>`, and `<app>/<connection> -> <app>_<connection>`.
  - [x] Ensure the migration runner records or logs App, connection name, migration source path, and generated database connection name.
  - [ ] Decide whether app migration command exposure remains always available or becomes configurable per app.
  - [x] Ensure auth routes and auth schedules register through named app composition. Auth does not currently define app-owned job registrations.
  - [x] Ensure starter kits can point frontend code at a named app HTTP runtime through frontend-prefixed root `.env` keys.
  - [x] Move starter frontend scaffolds under `cmd/app/frontend` so frontend source and embedded assets share the app-local command package.
  - [x] Transpose root `.env` `FRONTEND_*` values into Vite `import.meta.env.VITE_*` values.
  - [x] Support app-specific frontend env overrides with `<APP>_FRONTEND_*`.
  - [ ] Design the multiple-SPA story separately, including filesystem layout, env scope, route mount, and `http.RegisterSpa` generation.
  - [x] Ensure generated frontend placeholder assets are embedded under the correct `cmd/<app>/frontend/dist` when a named app owns Web UI.
  - [x] Avoid a first-class frontend publish/copy command by making app-local `npm run build` write directly to `cmd/<app>/frontend/dist`.
  - [x] Ensure route/API index/OpenAPI output shows the active app where command output exists. Route list output shows the active app; API index/OpenAPI status and logs include the active app.
  - [x] Keep queue names logical in app code and `.env`; physicalize backend queue names from the active app so named apps use queues such as `billing_default` and `billing_reports`.
  - [x] Avoid a separate `QUEUE_NAMESPACE` setting. The app name is the queue namespace for named apps, and the default `app` keeps existing queue names unchanged.
  - [x] Add queue driver coverage for app-isolated queue names across Redis, SQL, NATS, SQS, and RabbitMQ.
  - [x] Add the dedicated `forj [<app>] build:api-index` command with standalone `--strict` diagnostics policy.

- [ ] Partial: add app-aware observability and Lighthouse identity.
  - [x] Add `app` to Lighthouse agent identity.
  - [x] Read app identity from `FORJ_APP`, defaulting to `app`.
  - [x] Include `app` in hub agent metadata.
  - [x] Add stable Lighthouse group keys so the default app keeps source groups such as `http`, while named apps use `<app>/<source>` such as `billing/http`.
  - [x] Add stable Lighthouse agent keys that include instance identity, such as `billing/jobs/worker-01`.
  - [x] Add generated hub coverage proving two apps and two replicas can keep distinct identities for the same runtime source.
  - [ ] Verify local multi-app Lighthouse websocket connections use distinct app identity.
  - [x] Add `app`, `agent_key`, `group_key`, and `instance_key` to inspect metadata shipped through Lighthouse.
  - [x] Add `app` to readiness payloads while keeping the hot health payload stable.
  - [x] Add explicit `app` labels to framework-owned Prometheus metrics and generated Grafana dashboard selectors.
  - [x] Generate vmagent scrape targets for every conventional app in local modes, including `app` labels and deterministic app runtime ports.
  - [x] Group Lighthouse-selectable agents by app through generated agent keys.
  - [x] Collapse the app level in Lighthouse when the only app is `app` by preserving default source keys.
  - [ ] Update Lighthouse UI copy and deeper grouping tests for app-aware data.

- [ ] Partial: add rendered smoke coverage.
  - [x] Render and build default single-app Apps across the render matrix.
  - [x] Verify unqualified commands use the default app.
  - [x] Verify `make:command` updates `app/commands.go` and `app/wire/inject_cmd_app.go`.
  - [x] Verify generated Wire rebuilds after `make:command`.
  - [x] Verify app-owned injector files are preserved across rerender.
  - [x] Add unit coverage for convention-discovered named apps and named app template rendering.
  - [x] Manually verify a `/tmp` named-app render builds both default and named app binaries.
  - [x] Verify framework-owned injector files are overwrite-rendered.
  - [x] Verify `repositorySet` does not contain service providers.
  - [x] Render and build a multi-app App with apps such as `customer-portal`.
  - [x] Verify app-specific jobs and schedules in rendered smoke coverage.
  - [x] Verify app-specific route lists and binaries in rendered smoke coverage.
  - [x] Verify convention source-mode `forj <app> ...` routes commands to the active app in rendered smoke coverage.
  - [x] Verify named app generators update only the selected app.
  - [x] Verify default app generators continue to work when named apps are configured.
  - [x] Verify app name validation rejects reserved names and command collisions.
  - [ ] Verify all-app build/render validation when that workflow is added.

- [ ] Partial: update docs and generated READMEs.
  - [x] Document `cmd/<app>`, `app/`, `app/<app>`, and `internal/` ownership in this design.
  - [x] Document Apps as composition roots, not runtime types.
  - [x] Document the default single-app path before multi-app fan-out.
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

- Should `cmd/<app>/main.go` import the app package directly, or should it call a generated launcher package shared by all apps?
- What is the collision escape hatch for generated commands that share names with native app-aware commands?
- How should app-specific env overrides be named without creating noisy configuration?
- Which generated runtime support packages remain in `internal/` permanently?
- How much of this can be introduced without breaking existing Apps?

## Current Lean

This direction is cleaner than adding service folders, module registries, workspace semantics, or `--app` flags across every command.

The constraint should remain:

> GoForj has one App model. Larger projects use shared internal packages plus visible Apps under `app/` and `app/<app>/`.

The default app should live directly under `app/`. That optimizes for the vast majority of single-app Apps:

```text
cmd/app/
app/
internal/
```

Additional apps should live directly under `app/<app>/`, with matching thin entrypoints under `cmd/<app>/`.

If that remains true, Apps are a scale-out path for the existing model rather than a new abstraction.
