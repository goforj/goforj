# App Composition Layout Design

This note explores whether generated GoForj Apps should move App registration and dependency assembly into a top-level `app/` composition layer.

The goal is not to introduce a second App model. The goal is to make large Apps easier to reason about while preserving the single-App mental model that already exists.

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

- one GoForj App
- domain implementation under `internal/`
- App composition in one obvious place
- generated registries remain App-level, not domain-owned

## Possible Shape

Move App composition out of `internal/` and into a top-level `app/` directory:

```text
platform/
  .goforj.yml
  go.mod

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

The mental model becomes:

- `app/` answers "what does this App expose?"
- `app/wire/` answers "how is this App assembled?"
- `internal/` answers "how does the domain behavior work?"
- `migrations/` answers "how does database state evolve?"

## Generator Feel

The generator commands should still feel the same:

```bash
forj make:controller billing:reports
forj make:command reports:sync -d ./internal/billing/reports
forj make:job billing:reports:generate --queue reports
forj make:event billing:invoice-paid -d ./internal/billing/events
```

But the files they update would become more obvious.

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

Another example:

```bash
forj make:command reports:sync -d ./internal/billing/reports
```

Creates:

```text
internal/billing/reports/sync_cmd.go
```

Updates:

```text
app/commands.go
app/wire/inject_cmd.go
```

This keeps package layout flexible without changing the App model.

## What Should Not Happen

Do not introduce a new "service" abstraction just to organize files.

Avoid:

```text
services/billing
services/identity
services/notifications
```

unless each entry is truly a separate GoForj App with separate deployment, release, runtime, and configuration ownership.

Also avoid a parallel module registry abstraction unless there is a concrete problem the `app/` composition layer cannot solve.

## Benefits

- Gives users one obvious place to inspect App composition.
- Keeps domain packages focused on implementation.
- Avoids pretending large internal packages are independent services.
- Makes generated registration files feel intentional rather than hidden.
- Keeps `forj` commands app-local and familiar.
- Makes docs easier: "App composition lives in `app/`; implementation lives in `internal/`."

## Risks

- `app/` could become a dumping ground if it accepts business logic.
- Moving Wire into `app/wire/` may require import path and generated template churn.
- Existing docs and tests reference `wire/` and `internal/*` registration paths heavily.
- Users may still expect `app/` to contain runtime code, not just composition.
- The migration could be disruptive for existing generated Apps.

## Migration Direction

This should not be a sudden breaking layout change.

Possible path:

1. Add new templates for `app/` and `app/wire/` in new Apps only.
2. Keep compatibility with current `internal/` and `wire/` registration files for existing Apps.
3. Teach generators to detect which layout exists.
4. Update docs to describe the new layout as the preferred generated shape.
5. Consider a migration command only after the new layout has proven itself.

Generator detection could be simple:

- if `app/commands.go` exists, update the new layout
- else update `internal/cmd/app_commands.go`
- if `app/wire/` exists, update `app/wire/*`
- else update `wire/*`

## Open Questions

- Should `app/` be a Go package named `app`, or should it use a less collision-prone package name?
- Should `app/wire/` be package `wire`, `appwire`, or something else?
- Should `app/routes.go` replace `internal/router/routes_registry.go`, or should HTTP routing keep a package-specific home?
- Should generated App root construction move from `wire/` to `app/wire/` entirely?
- Where should `about` and other framework-provided App commands live?
- Should `internal/jobs/worker.go` remain runtime implementation while job registration moves to `app/jobs.go`?
- Can we keep imports clean if `app/` imports many `internal/<domain>` packages?
- How much of this can be introduced without breaking existing Apps?

## Current Lean

This direction feels cleaner than adding service folders, module registries, or workspace semantics.

The constraint should remain:

> GoForj has one App model. Large Apps use package organization plus a visible App composition layer.

If that remains true, `app/` may be a simplification rather than a new abstraction.
