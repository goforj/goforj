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

Use rerender intentionally.

- template/generator/source shape changes: rerender
- generated app codegen/build issues after env/config changes: rebuild
- Lighthouse UI changes: rebuild the UI bundle and then rerender only if generated output needs to move

Common checks:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/forj -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/generate -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/build -count=1
```

For integration-heavy regressions, also remember:

```bash
PATH="/tmp:$PATH" GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go test -tags=integration ./internal/forj -count=1
```

`wire` must be on `PATH` for those tests because they render and build temp apps.

Then:

```bash
cd /host-tmp/test
/tmp/forj render
```

For dev-loop validation, also keep this distinction clear:

```bash
/tmp/forj build
/tmp/forj dev
```

`forj build` is the step that regenerates code, runs wire, and rebuilds the rendered app.
Do not assume every change that affects a running dev session requires `forj render`.

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
- if a sibling repo is multi-module, make sure all relevant submodules are replaced or released consistently

Recent lesson from `storage`:

- bumping only the root module is not always enough
- generated apps can keep older driver submodules unless GoForj pins them explicitly or local replaces cover them

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

For sibling-repo release validation, the practical sequence has been:

```bash
cd /host-tmp/test
/tmp/forj render
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go mod tidy
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go build ./...
```

If `forj build` or `wire` complains that `go.mod` needs updates, verify whether the rendered app is on the intended released versions first before assuming a generator bug.

Recent release-sensitive test lesson:

- fresh sibling repo tags may be resolvable from GitHub before the public Go proxy catches up
- if a test is intentionally verifying a just-published multi-module release, `GOPROXY=direct` can be appropriate for that narrow test path
- do not turn that into a blanket repo-wide default

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
- a sibling repo release tag exists locally but was not actually pushed
- a multi-module sibling repo release updated one module but left submodules on older versions
- a generated app is still using an older installed `forj` binary instead of the checkout you just changed
- watcher processes are still running with stale inherited env even though `.env` changed
- the temp app integration harness is launching child processes with duplicate env keys, causing stale values to win unexpectedly

## Working Rule

If the bug reappears after rerender, the real fix was not made in the right place.
