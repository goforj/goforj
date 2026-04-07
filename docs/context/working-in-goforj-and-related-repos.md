# Working In GoForj And Related Repos

This document is durable guidance for agents working in the `goforj`, `web`, and `queue` repos.

It is not a dated status snapshot. It is the operating model.

## Repo Roles

### `goforj`

`goforj` is the framework, generator, and local developer workflow repo.

It owns:

- project rendering
- generated app templates
- `forj build`, `forj run`, `forj render`, `forj dev`
- generated app runtime conventions
- app-level env policy
- app-level lifecycle and bootstrap wiring
- demo app templates and Lighthouse UI

Recent important areas inside `goforj`:

- Lighthouse is now a substantial operator console, not just a diagnostics page
- generated storage manager behavior is part of app/runtime policy
- `forj dev` correctness depends on supervisor/watcher env behavior, not just file watching

It should not absorb reusable web or queue implementation details when those can live in sibling repos.

### `web`

`web` is the web abstraction repo.

It owns:

- `web.Context`, `web.Response`, `web.Router`
- route registration and route-list concepts
- middleware abstractions
- framework adapter logic in `adapter/echoweb`
- web support packages like:
  - `webmiddleware`
  - `webindex`
  - `webprometheus`

Use `web` when the generated app should not need to care that Echo exists.

Recent note:

- the Echo v5 migration stayed behind the `web` boundary successfully
- if a change can be contained in `web`, prefer that over pushing framework details into rendered apps

### `storage` / `filesystem`

`storage` is now a meaningful sibling repo in the same category as `web` and `queue`.

It owns:

- storage abstraction shape
- driver implementations
- cross-driver contract behavior
- integration coverage for real backends
- docs/examples for storage capabilities

Recent changes that pushed work into `storage` rather than `goforj`:

- paged listing
- directory creation
- directory rename/move behavior
- file counting helpers
- slash-path normalization vs local-filesystem `filepath` boundaries

If the question is "should Lighthouse/storage explorer be able to do this generically across drivers?", the answer often belongs in `storage` first.

### `queue`

`queue` owns queue abstractions and driver implementations.

It owns:

- queue interfaces
- driver behavior
- Redis/Asynq pass-through behavior
- worker shutdown semantics inside the queue layer

If a bug is really about Redis/Asynq behavior or queue-driver consistency, it probably belongs in `queue`, not `goforj`.

## Where Changes Belong

### Put the change in `goforj` when:

- it is app policy
- it affects generated app structure
- it is CLI/build/dev workflow behavior
- it is about templates or generators
- it is project-level env policy

Examples:

- `.env` conventions
- generated app lifecycle wiring
- `forj run` UX
- `forj dev` TUI behavior
- render-time local module replaces
- Lighthouse UX/state handling
- generated storage manager policy
- optional disk degradation/reporting in generated apps

### Put the change in `web` when:

- it is generic web runtime behavior
- it should be hidden behind the web abstraction
- it is route/middleware/telemetry functionality
- it is framework-adapter behavior

Examples:

- response abstraction
- route listing/sorting
- Prometheus support
- Echo version migrations

### Put the change in `queue` when:

- the queue driver itself is inconsistent
- shutdown semantics are wrong inside the driver
- backend-specific timeout/config behavior is incomplete

Examples:

- Asynq shutdown timeout propagation
- Redis worker shutdown honoring context

## Core Rendering Model

### Source of truth

The generated app is not the source of truth.

If a change should survive rerender, the real fix usually belongs in:

- `templates/...`
- or framework/generator code under `internal/...`

The rendered app is mainly a smoke target and local integration target.

### Render flow

At a high level, `forj render` does:

1. load `.goforj.yml`
2. render templates and generator outputs
3. apply `render.module_replaces`
4. sync core libraries
5. run wire generation
6. leave the app in a buildable/generated state

If `module_replaces` is wrong, sibling repos will not be picked up.

Do not assume `forj dev` rerendering is the right reaction to every env change.
At this point:

- `forj render` is for template/project-shape updates
- `forj build` is the codegen/build step for generated apps
- `.env` changes in `forj dev` should rebuild/restart watchers, not force rerender

## `render.module_replaces`

This is the local-dev bridge for unpublished sibling repos.

Example:

```yaml
render:
  module_replaces:
    github.com/goforj/web: /Users/cmiles/code/web
```

That becomes:

```bash
go mod edit -replace github.com/goforj/web=/Users/cmiles/code/web
```

Important rules:

- use absolute paths
- do not use `~`
- do not assume relative paths are stable

If a rendered app needs to consume local `web` or `queue` work before release, this is the preferred path.

This same pattern is now normal for `storage` too while validating new storage capabilities before release.

## Working With The Rendered App

The common smoke target during this work has been:

- `/host-tmp/test`

Use the rendered app to validate that:

- templates actually generate correctly
- runtime wiring still holds together
- sibling repo changes really work in a real app

Edit the rendered app directly only when:

- testing a local hypothesis quickly
- fixing a temporary local-only path/config issue
- patching the smoke target on purpose

Do not treat those edits as the durable fix unless they are intentionally local-only.

Recent practical lesson:

- if a rendered app behaves differently from source expectations, inspect generated files directly
- several storage/discovery bugs were easiest to confirm in:
  - `internal/storages/manager_gen.go`
  - `internal/app/discovery.go`

## Generated App Runtime Model

### `internal/app`

This is the root runtime package in generated apps.

Key concepts:

- `Lifecycle`
  - startup/shutdown phase coordination
- `Timeouts`
  - app timeout policy resolved once
- `LifecycleRegistry`
  - extension point for user lifecycle hooks

This replaced the earlier `internal/lifecycle` package direction.

### Runtime ownership

The generated app is shaped so:

- app/root runtime policy lives in `internal/app`
- HTTP server wiring lives in `internal/http`
- jobs wiring lives in `internal/jobs`
- scheduler wiring lives in `internal/scheduler`

Process bootstrap concerns should live at the process boundary, not be hidden in low-level helpers.

That is why Lighthouse runtime boot moved out of free functions and into process entrypoints.

Another important runtime rule:

- default storage disk is required
- named disks should degrade independently where possible

One unavailable optional disk should not wipe out every healthy disk in Lighthouse or app readiness.

## Logging / Observability Model

### Process logs vs primitive chatter

Keep a distinction between:

- top-level process lifecycle logs
  - visible by default
- managed primitive chatter
  - debug-level or explicit, not noisy by default

This split matters for:

- HTTP
- scheduler
- jobs
- DB/event lifecycle logs

Recent storage-specific lesson:

- warnings about unavailable optional disks should go through the normal app logger
- avoid raw `stderr` prints from generated managers
- avoid emitting the same warning once per bootstrap process when `forj run` starts subprocesses

### Route visibility

Do not dump the full route table at normal boot.

Instead:

- boot logs a short route-count summary
- `route:list` is the explicit full route inspection command

This avoids ugly output collisions during multi-process startup.

### `APP_LOG_TIME`

Console timestamps are gated by:

- `APP_LOG_TIME`

If timestamps appear but only show `.000`, the issue is usually emitted timestamp precision, not the console formatter.

## `forj run` / `forj dev`

### `forj run`

Intended mental model:

- `forj run <app-command>`

Examples:

- `forj run run`
- `forj run route:list`
- `forj run queue:work`

It should feel like an orchestrated app command runner, not raw `go run main.go ...`.

Important recent behavior:

- `forj run run` maps cleanly to `go run . run`
- `--timings` exists
- no-op `generate` no longer calls `go mod tidy`
- launch output is cleaner than before

### `forj dev`

`forj dev` is watcher/orchestration UX.

It should not own process naming policy or app log prefix semantics beyond watcher concerns.

The actual child app/process topology belongs lower in `run`/runtime launch logic.

## Common Workflows

### Changing `goforj`

Typical loop:

1. change template/generator/framework code
2. run focused tests
3. rerender `/host-tmp/test`
4. build/run/smoke the rendered app

Common checks:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/forj -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/generate -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/build -count=1
```

### Changing `web`

Typical loop:

1. change `web`
2. run tests in `/workspace/code/web`
3. point the rendered app at local `web` using `render.module_replaces`
4. rerender and smoke the app

Common check:

```bash
cd /workspace/code/web
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

### Changing `queue`

Typical loop:

1. change `queue`
2. run focused driver tests
3. tag/push queue modules if GoForj needs published versions
4. bump GoForj dependency
5. rerender/build/test the app

## Important Architectural Conclusions

### The `web` boundary is paying off

The strongest proof so far:

- Echo v5 was a major upstream break
- most of the migration stayed inside `web/adapter/echoweb`
- generated app shapes mostly did not need to change

That is exactly what the abstraction is for.

### `web` should own real primitives

The repo is no longer just a thin facade.

It now legitimately owns:

- request/response abstractions
- middleware surface
- telemetry packages
- route/indexing behavior

### Root runtime policy belongs near the root

App timeout and lifecycle policy should not be rediscovered by each primitive from env.

Resolve once, pass down as dependency, and keep runtime policy centralized.

## Frequent Pitfalls

- Do not use `~` in `module_replaces`.
- Do not assume relative replace paths will work.
- Do not fix persistent generated-app issues only in the rendered app.
- Do not put driver/backend-specific fixes in `goforj` if they belong in `queue`.
- Do not put reusable web concerns in GoForj just because the template currently holds them.
- Do not reintroduce duplicated env parsing in leaf components.

## Current Live Work To Remember

There is currently uncommitted Echo v5 migration work in `/workspace/code/web`.

That migration is green under:

```bash
cd /workspace/code/web
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

It should be committed before treating local `web` as settled.
