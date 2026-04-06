# Rendering And Smoke Workflow

This document explains how to validate GoForj changes against a real rendered app.

## Core Rule

The rendered app is not the source of truth.

If a fix should survive rerender, it belongs in:

- `templates/...`
- generator/runtime code in `internal/...`

The rendered app is a smoke target and integration target.

## Main Smoke Target

During recent work, the common local target has been:

- `/host-tmp/test`

Treat it as disposable.

## Standard Workflow

1. change GoForj templates/framework code
2. run focused tests in `goforj`
3. rerender the smoke app
4. build/run/smoke the rendered app

Common checks:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/forj -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/generate -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/build -count=1
```

Then:

```bash
cd /host-tmp/test
/tmp/forj render
```

## What `forj render` Conceptually Does

At a high level:

1. loads `.goforj.yml`
2. renders templates and generator outputs
3. applies `render.module_replaces`
4. syncs core libraries
5. runs wire generation
6. leaves the app in a generated state

If sibling repos are not being picked up, inspect `render.module_replaces` first.

## `module_replaces`

Use `render.module_replaces` when the rendered app needs local sibling repos before a release is tagged.

Example:

```yaml
render:
  module_replaces:
    github.com/goforj/web: /Users/cmiles/code/web
```

Important:

- use absolute paths
- do not use `~`
- do not assume relative paths are stable

## When To Edit The Rendered App Directly

Valid reasons:

- quick hypothesis check
- local-only path/config fix
- patching the smoke target intentionally

Do not stop there if the fix should be durable.

## Typical Smoke Commands

After render:

```bash
cd /host-tmp/test
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go build ./...
./bin/app run
```

Useful command smoke:

```bash
/tmp/forj run --timings route:list
/tmp/forj dev
```

## Common Failure Modes

- `module_replaces` points at the wrong path
- `~` used in replace path
- local sibling repo change was made but the rendered app is still on published dependency versions
- a rendered app fix was made without changing the template/generator source

## Working Rule

If the bug reappears after rerender, the real fix was not made in the right place.
