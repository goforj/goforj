# App Structure

This document is the quick orientation for GoForj's app layout. A Project starts with one default app and can add more apps when it needs separate runnable boundaries.

## Default App

The default app uses:

- `cmd/app/` for the binary entrypoint
- `app/` for app composition and registration
- `app/wire/` for app-specific Wire assembly
- `internal/` for shared implementation, domain packages, handlers, services, jobs, repositories, and runtime internals

The simple mental model is:

```text
cmd/app/   # how the default app starts
app/       # what the default app exposes and composes
internal/  # what the app does
```

Do not move business logic into `app/`. The `app/` package should compose and expose behavior; it should not become the domain layer.

## Default App Entry Point

The default app binary lives at:

```text
cmd/app/main.go
```

Keep this entrypoint thin. Runtime behavior, commands, routes, schedules, and lifecycle hooks should be composed through `app/` and `app/wire/`.

When the generated app has Web API, Web UI, Scheduler, or Jobs capability, launching its binary without arguments starts the combined `run` host. A CLI-only app keeps no-argument help behavior. Explicit commands, including `run` and `--help`, always remain available.

## App Composition Points

Common default-app composition files:

- `app/root_cmd.go`
- `app/commands.go`
- `app/routes.go`
- `app/lifecycle.go`
- `app/schedules.go`
- `app/wire/inject_cmd_app.go`
- `app/wire/inject_http_controllers_app.go`
- `app/wire/inject_jobs_app.go`
- `app/wire/inject_repositories_app.go`
- `app/wire/inject_schedules_app.go`
- `app/wire/inject_services_app.go`
- `app/wire/inject_subscribers_app.go`

When adding generated behavior, prefer the matching `forj make:*` command so the right composition and Wire files are updated together.

## Additional Apps

An additional app uses the same shape with its app name:

```text
cmd/<app>/main.go
app/<app>/
app/<app>/wire/
```

Every Project starts with the default app, `app`. Add another app when the Project needs an additional runnable boundary, such as `admin`, `marketplace`, or `backstage`.

## Command Routing

Unprefixed commands target the default app:

```bash
forj make:controller users
forj route:list
forj build
```

Prefixed commands target an additional app:

```bash
forj marketplace make:controller checkout
forj marketplace route:list
forj marketplace build
```

The prefix should route generated code into that app's registration and Wire files.

## Migrations

Single-app Projects keep the simple migration layout until an additional app exists.

Multi-app projects use app-scoped migration ownership:

```text
migrations/<app>/<connection>/
```

Do not force a single-app migration into multi-app paths unless the project has actually added another app.

## Upgrade Notes

For beta projects created before this layout, use the root migration note:

- [`../../migration.md`](../../migration.md)

Treat that as a single-app structure migration. It is not a requirement to create additional apps.

## Deep Design

For full architectural history and implementation details, read:

- [`../designs/completed/app-composition-layout-design.md`](../designs/completed/app-composition-layout-design.md)
