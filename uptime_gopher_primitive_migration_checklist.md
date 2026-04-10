# Uptime Gopher Primitive Migration Checklist

## Goal

Move the demo app onto goforj's first-class infrastructural primitives so Uptime Gopher uses:

- generated managers/registries
- generated named accessors
- env-scoped swappable drivers
- uniform readiness/introspection surfaces

and avoids ad hoc primitive construction inside the demo layer.

## Pattern To Preserve

For each primitive family:

- inject the generated manager/registry, not raw env readers
- use `Default()` for the root instance
- use generated named accessors for well-known children
- use `Named(name)` only for dynamic lookups
- keep driver selection/config inside generated primitive packages
- surface readiness through manager-provided checks

## Status

- [x] Database mostly follows the pattern via `*database.Connections`
- [x] Events already use generated `events.NewBus(...)`
- [ ] Queues still leak raw `*queue.Queue` into demo services/jobs
- [ ] Cache usage in demo app should go through `*caches.Manager` when present
- [ ] Storage usage in demo app should go through `*storages.Manager` when present
- [ ] Demo app wire sets should prefer manager injection over instance injection

## Queue Refactor First

- [x] Inventory all demo app queue entry points and consumers
- [x] Change demo queue-using services to depend on `*queues.Manager`
- [x] Replace raw default queue injection with `manager.Default()`
- [x] Replace named queue assumptions with generated accessors where available
- [ ] Keep queue-name literals only where the underlying job payload API requires them
- [ ] Verify worker startup/shutdown still uses the generated queue manager correctly
- [ ] Run targeted tests for demo queue paths

## Demo App Targets

- [x] `templates/demo/internal/alerts/dispatch_job.go.tmpl`
- [x] `templates/demo/internal/monitoring/monitor_check_job.go.tmpl`
- [x] `templates/wire/inject_jobs_app.go.tmpl`
- [ ] Any other demo constructors still taking raw `*queue.Queue`

## Remaining Queue Work

- [x] Move built-in job consumers to `*queues.Manager`
- [x] Move generated `make:job` template consumers to `*queues.Manager`
- [ ] Review `templates/internal/jobs/worker.go.tmpl` and `templates/internal/jobs/lighthouse.go.tmpl`
- [ ] Decide whether orchestration/observability code should also drop raw fallback queue fields entirely

## Follow-On Work

- [ ] Review demo cache usage and migrate to `*caches.Manager`
- [ ] Review demo storage usage and migrate to `*storages.Manager`
- [ ] Review whether events should remain a generated single bus or grow a manager abstraction later
- [ ] Ensure app/about/readiness/dev-console surfaces still reflect the final primitive wiring

## Verification

- [x] Template/unit tests for generator output
- [x] Render an existing demo app and confirm rewiring survives re-render
- [x] Build rendered app
- [ ] Smoke test queue-driven demo workflows
