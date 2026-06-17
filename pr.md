## Summary

This branch keeps the simple single-app path as the default GoForj experience while expanding the project model so larger teams can grow into multiple runnable apps inside one project.

Most projects will still render and work like a normal single app. When a project needs to scale into a marketplace, backstage app, admin console, API, worker, scheduler, or dozens of other service boundaries, GoForj now has a first-class structure for that instead of forcing each service into a separate generated project.

## Why

GoForj projects need to stay easy for small apps, but they should also fit larger systems where one codebase may own many closely related services.

The nicest part is that multi-app usage reads like normal GoForj usage. You do not need to switch directories, memorize app flags, or treat each service like a separate generated project.

```bash
forj route:list
forj migrate
forj dev
```

still means "use the default app."

When a project grows, the app name becomes a natural command prefix:

```bash
forj marketplace route:list # or ./bin/marketplace route:list
forj marketplace migrate    # or ./bin/marketplace migrate
forj marketplace make:controller checkout
forj marketplace make:job sync-catalog
forj marketplace build

forj backstage scheduler # or ./bin/backstage scheduler
forj backstage make:schedule nightly-cleanup
forj backstage make:command rebuild-search
forj backstage dev
```

That same shape works whether GoForj is dispatching through source conventions like `cmd/marketplace/main.go` or through a built binary like `./bin/marketplace`.

The model is now:

```text
GoForj Project
  App: app (default)
    Runtime: http
    Runtime: jobs
    Runtime: scheduler

  App: marketplace
    Runtime: http
    Runtime: jobs

  App: backstage
    Runtime: http
    Runtime: scheduler
```

A single-app project still feels like one app. Multi-app projects get clear registration points, app-specific binaries, app-scoped runtime identity, and app-aware observability.

Generated artifacts that used to assume one app are now app-aware where they need to be: routes, commands, schedules, migrations, OpenAPI/API index output, runtime ports, metrics, dashboards, and Lighthouse identity.

Each runnable app also gets a matching binary entrypoint under `cmd/`.

```text
cmd/app/main.go          # default app binary
cmd/marketplace/main.go  # marketplace app binary
cmd/backstage/main.go    # backstage app binary
```

## What This Feels Like

<img width="1042" height="568" alt="GoForj multi-app command output" src="https://github.com/user-attachments/assets/ab8a99dc-94db-416f-951f-9f375bdcc766" />

Create an app:

```bash
forj make:app marketplace --components web-api,jobs --starter-kit vue
```

```text
Created app: marketplace
Entrypoint: cmd/marketplace/main.go
Composition: app/marketplace
```

Run commands in that app:

```bash
forj marketplace route:list # or ./bin/marketplace route:list
forj marketplace run        # or ./bin/marketplace run
forj marketplace build
```

Generate into that app:

```bash
forj marketplace make:controller checkout
forj marketplace make:job sync-catalog
forj marketplace make:model order
```

```text
creates internal/checkout/controller.go
updates app/marketplace/routes.go
updates app/marketplace/wire/inject_http_controllers_app.go
updates app/marketplace/wire/inject_jobs_app.go
updates app/marketplace/wire/inject_repositories_app.go
```

The same routing applies to jobs, models, commands, schedules, and other generated resources: behavior is created under `internal/...`, and the selected app receives the matching registration under `app/<app>/...`.

The prefix chooses the registration point. `forj marketplace make:*` creates the resource under `internal/...` and writes exposure to the marketplace app, while unprefixed `forj make:*` keeps writing exposure to the default app.

Run another app without leaving the project:

```bash
forj backstage scheduler # or ./bin/backstage scheduler
forj backstage migrate    # or ./bin/backstage migrate
forj backstage make:schedule nightly-cleanup
```

Build, then run the matching app command:

```bash
forj marketplace build
forj marketplace run # or ./bin/marketplace run

forj backstage build
forj backstage scheduler # or ./bin/backstage scheduler
```

The default app remains just as simple:

```bash
forj dev
forj route:list
forj migrate
```

<img width="1608" height="1050" alt="GoForj multi-app runtime output" src="https://github.com/user-attachments/assets/3e9330c1-de9b-4165-a326-c4d6ebee601b" />

## App-Scoped Output

The same app name shows up everywhere GoForj generates or reports app-specific state.

OpenAPI/API index output stays simple for the default app and separates named apps:

```text
build/openapi.json
build/api_index.json

build/marketplace/openapi.json
build/marketplace/api_index.json

build/backstage/openapi.json
build/backstage/api_index.json
```

Migrations stay simple until the project becomes multi-app, then expand into explicit app ownership:

```text
migrations/
  2026_06_05_120000_create_users.up.sql

migrations/app/default/
  2026_06_05_120000_create_users.up.sql

migrations/marketplace/default/
  2026_06_05_122000_create_orders.up.sql

migrations/backstage/default/
  2026_06_05_123000_create_workflows.up.sql
```

Frontend source and embedded assets follow the binary entrypoint:

```text
cmd/app/frontend/
cmd/marketplace/frontend/
cmd/backstage/frontend/
```

Metrics and dashboards can filter by the app that produced the signal:

```text
http_requests_total{app="marketplace",source="http"}
http_requests_by_route_total{app="backstage",source="http",route="/admin/workflows"}
```

Lighthouse can present the same shape users already understand:

```text
marketplace
  http
  jobs

backstage
  http
  scheduler
```

## App Registration Before and After

Before this branch, a generated project was shaped around one runnable app. The binary entrypoint lived at the project root, and registration lived in shared internal/framework-oriented places.

```text
.
├── main.go
├── internal/
│   ├── app/
│   │   └── lifecycle_registry.go
│   ├── cmd/
│   │   └── root_cmd.go
│   ├── http/
│   │   └── routes_registry.go
│   ├── jobs/
│   ├── reports/
│   ├── schedules/
│   │   └── scheduler_registry.go
│   └── users/
├── wire/
│   ├── app.go
│   └── wire.go
└── migrations/
```

That was fine when a project meant exactly one runnable app. The awkward part was that app registration was mixed into `internal/...` and root `wire/...`, so a second runnable app had nowhere obvious to own its commands, routes, lifecycle hooks, schedules, or Wire graph.

After this branch, a project has a clear split:

- `cmd/<app>/` is the binary entrypoint for one runnable app.
- `app/` and `app/<name>/` are app composition: commands, routes, lifecycle hooks, schedules, and app-local Wire registration.
- `internal/` stays shared application behavior and generated runtime support. Controllers, services, repositories, jobs, subscribers, domain packages, and reusable framework runtime code still live there.

A single-app project now looks like this:

```text
.
├── cmd/
│   └── app/
│       └── main.go
├── app/
│   ├── commands.go
│   ├── lifecycle.go
│   ├── root_cmd.go
│   ├── routes.go
│   ├── schedules.go
│   └── wire/
│       ├── inject_cmd_app.go
│       ├── inject_http_controllers_app.go
│       ├── inject_jobs_app.go
│       ├── inject_repositories_app.go
│       ├── inject_schedules_app.go
│       ├── inject_services_app.go
│       ├── wire.go
│       └── wire_gen.go
├── internal/
│   ├── checkout/
│   ├── jobs/
│   ├── reports/
│   ├── runtime/
│   │   └── apps.go
│   └── users/
└── migrations/
```

The default path is still simple: one binary, one app composition directory, shared business logic under `internal/`.

When the project grows, named apps add parallel app boundaries without cloning the whole project:

```text
.
├── cmd/
│   ├── app/
│   │   └── main.go
│   ├── marketplace/
│   │   ├── main.go
│   │   └── frontend/
│   └── backstage/
│       └── main.go
├── app/
│   ├── commands.go
│   ├── lifecycle.go
│   ├── routes.go
│   ├── schedules.go
│   ├── wire/
│   ├── marketplace/
│   │   ├── commands.go
│   │   ├── lifecycle.go
│   │   ├── routes.go
│   │   ├── schedules.go
│   │   └── wire/
│   └── backstage/
│       ├── commands.go
│       ├── lifecycle.go
│       ├── routes.go
│       ├── schedules.go
│       └── wire/
├── internal/
│   ├── checkout/
│   ├── catalog/
│   ├── jobs/
│   ├── reports/
│   ├── runtime/
│   │   └── apps.go
│   └── users/
└── migrations/
    ├── app/
    │   └── default/
    ├── marketplace/
    │   └── default/
    └── backstage/
        └── default/
```

The important part is that `internal/` does not become `internal/marketplace` by default. It remains the shared implementation layer for the project. Apps decide which pieces of that implementation they expose.

For example:

```text
internal/checkout/controller.go
internal/catalog/sync_job.go
internal/reports/daily_schedule.go
```

can be exposed by:

```text
app/marketplace/routes.go
app/marketplace/wire/inject_http_controllers_app.go
app/marketplace/wire/inject_jobs_app.go
app/backstage/schedules.go
app/backstage/wire/inject_schedules_app.go
```

That gives small projects the same simple shape as before, while giving larger projects a path to dozens of runnable apps without dozens of repos or duplicated scaffolds.

## What Changed

**Project and app model**

- Added `forj make:app` for creating named apps.
- Added default and named app rendering.
- Added app-aware `cmd/<app>/main.go` binary entrypoints.
- Moved app composition into `app/` and `app/<name>/`.
- Renamed the domain language from app target to app.
- Updated project config and component discovery for multiple apps.
- Added app-aware runtime identity and deterministic runtime ports for named apps.

**Binary entrypoints**

- Kept `cmd/app/main.go` as the default app entrypoint.
- Added `cmd/<name>/main.go` for each named app.
- Updated generated binaries so each app boots with its own composition files from `app/<name>/`.

**Build, run, and dev workflows**

- Updated build and run flows to understand app selection.
- Updated route listing, OpenAPI/API index output, and build/profile output to show the active app.
- Updated `forj dev` process management for multi-app sessions.
- Added compact multi-app CLI help output.
- Avoided unnecessary `go get` work during render dependency sync to keep renders faster.

<img width="860" height="1024" alt="GoForj multi-app build and dev output" src="https://github.com/user-attachments/assets/ca69ac74-c6f7-4256-b51b-fc1333719274" />

**Generated app registration**

- Updated generated command registration for app-specific commands.
- Updated HTTP route registration for app-specific routes.
- Updated lifecycle registration for app-specific startup/shutdown hooks.
- Updated schedule registration for app-specific schedules.
- Updated `make:*` commands so app prefixes route generated code into the selected app's registration and Wire files.
- Updated generated migrations and migration commands for app-scoped execution.
- Updated Wire templates so services, repositories, controllers, jobs, schedules, and subscribers can be wired per app.
- Added generated preboot and environment-default handling for app-specific startup.
- Updated generated runtime metadata and app discovery.

**Observability and Lighthouse**

- Updated metrics labels to use app-oriented identity.
- Updated Grafana dashboards to filter and group by app.
- Updated Lighthouse metadata, grouping, UI labels, and app/runtime display.

**Generated framework templates**

- Updated auth, database, queues, schedules, migrations, make commands, HTTP, logger, metrics, observability, and Lighthouse templates for the new app model.
- Updated generated app shell commands, health commands, readiness checks, Swagger wiring, and route list commands for app-aware runtime behavior.
- Updated Vue starter kit rendering and local post-render instructions.
- Updated frontend placeholder rendering and demo/Vue env files so app-specific frontend scaffolds work cleanly.
- Updated local dev container templates for rootless Podman-friendly volumes.

**Tests and documentation**

- Added multi-app render, runtime, and integration coverage.
- Added a dedicated multi-app GitHub Actions smoke workflow.
- Updated internal docs and design notes for the project/app/runtime model.
