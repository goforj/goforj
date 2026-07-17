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
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/apiindex -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/build -count=1
```

For integration-heavy regressions, also remember:

```bash
PATH="/tmp:$PATH" GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go run ./cmd/forj --dev test:integration framework -v
```

The framework command discovers tests contributed by integration-tagged files across `internal/forj/...`. It does not rerun ordinary unit tests. `wire` must be on `PATH` because those tests render and build temp apps.

Current default behavior matters:

- `forj test:integration` with no args should run everything
- rendered app integration should run all DB variants by default:
  - `sqlite`
  - `mysql`
  - `postgres`
- do not hide a `current DB only` shortcut behind the default command path
- if a run is intentionally narrowed, that should happen via explicit args

## CI Parity And Service Ownership

Each integration shard owns its test services. Do not start a host Redis instance in those jobs: framework tests that need Redis start and tear down a shared package-level container, and a process-level `REDIS_PORT` can override the rendered `.env` value used by Docker Compose. That can make the observability Compose test try to bind `6379` even after it selected a free port.

When a rendered Compose test allocates host ports, pass the same values through both the project `.env` and `t.Setenv`. Docker Compose interpolation gives process environment variables precedence over `.env`, which is important when the test runs under CI with inherited service variables.

Generated environment-sensitive tests must clear unrelated root and named overrides before asserting defaults. For example, a named SQLite default-path test should clear `DB_DATABASE`, `DB_SQLITE_DATABASE`, `DB_ANALYTICS_DATABASE`, and `DB_ANALYTICS_SQLITE_DATABASE`; otherwise a rendered project default can turn a unit assertion into an environment-dependent test.

The CI-equivalent checks for broad changes are:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./... -v
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
go run ./cmd/forj --dev test:integration all --variant all -v
cd integration
GOFORJ_BACKUP_INTEGRATION=1 GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
  go test -tags=integration_backup ./... -count=1
GOFORJ_BACKUP_INTEGRATION=1 GOFORJ_BACKUP_NATIVE_POSTGRES=1 \
  GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
  go test -tags=integration_backup ./... -run '^TestNativePostgresBackupRestore$' -count=1
GOWORK=off GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
  go test -tags=integration_generator ./... -run 'TestGenerate(Cache|Storage)FilesIntegrationSmoke' -count=1
cd ..
go run ./cmd/forj --dev test:renders --profile=pr --run-tests
```

Run renders in `/tmp`, keep `wire` on `PATH`, and inspect `git status` before trusting a result. A failing generated test should be fixed in its template or generator, not patched in the temporary render.

## Keep The Repo State Clean Before Render Smoke

Render smoke is sensitive to the actual current checkout state because embedded templates and source assets come from the working tree you build `forj` from.

Practical rule:

- if you are validating a render/smoke issue, do not assume a dirty repo is harmless
- clean up or explicitly account for uncommitted template/source changes first
- otherwise you can end up rendering output that does not match the commit or branch you think you are testing

This matters especially when:

- a hidden `test:render` or temp-app smoke path builds a fresh `forj` binary first
- generated output depends on embedded assets
- you are comparing temp-render behavior against CI or another machine

If a smoke result seems inconsistent, check `git status` before assuming the renderer is nondeterministic.

## Rendered Dependency Model In Tests

Rendered app integration now treats the rendered `docker-compose.yml` as the source of truth for dependency shape.

Important distinction:

- tests do not run `docker compose up`
- tests do parse the rendered compose file
- tests then translate rendered services into testcontainers requests
- testcontainers is still the mechanism that actually boots the containers

That means:

- if the rendered compose says `mysql` exists, the rendered test harness should boot the equivalent `mysql` service
- if the rendered compose says `redis` exists, the rendered test harness should boot the equivalent `redis` service
- do not keep a separate hardcoded “maybe we need mysql/redis/postgres” dependency list in test code for rendered-app coverage

## Env And Port Rules For Rendered Integration

Recent harness direction:

- use `.env` as the test source of truth
- remove `.env.host` for rendered integration tests to avoid layered host/container ambiguity
- allocate free host ports from a dedicated test range
- write those chosen ports into `.env`
- parse the rendered compose file with those env values applied
- bind testcontainers to the exact host ports the rendered compose resolves to

Why this matters:

- it avoids collisions with local development services already using `3306`, `5432`, or `6379`
- it proves env-driven port configuration still works in a real rendered app
- it keeps compose as the contract while still letting tests choose safe host ports

Related template rule:

- rendered compose ports must be parameterized with env values such as `DB_PORT` and `REDIS_PORT`
- if compose hardcodes host ports, the test harness cannot validate env-driven port behavior correctly
- that kind of hardcoding should be treated as a regression

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

## Preferred Temp Render Smoke

When you want a real rendered-app check, prefer a disposable temp render over patching a long-lived smoke app by hand.

The intended shape is:

1. render into `/tmp`
2. build the emitted app
3. run `go test ./...` inside the emitted app

Current maintainer-friendly shortcut:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go run ./cmd/forj/main.go test:render -s
```

That should be treated as a smoke path for:

- template regressions
- broken generated imports
- emitted app compile failures
- missing generated dependencies

If `test:render` disagrees with package-level tests in `goforj`, trust the rendered app failure and inspect the generated output path.

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
./bin/app
```

For runtime-capable Apps, bare execution is equivalent to `./bin/app run`.
CLI-only binaries print help when launched without arguments.

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

For rendered-app compile confidence after metrics/auth/template work, also use:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./... -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test -tags=integration ./internal/forj -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go run ./cmd/forj/main.go test:render -s
```

## Common Failure Modes

- `module_replaces` points at the wrong path
- `~` used in replace path
- local sibling repo change was made but the rendered app is still on published dependency versions
- a rendered app fix was made without changing the template/generator source
- a sibling repo release tag exists locally but was not actually pushed
- a multi-module sibling repo release updated one module but left submodules on older versions
- a generated app is still using an older installed `forj` binary instead of the checkout you just changed
- the repo was dirty when the `forj` binary was built, so the embedded render assets did not match the expected commit state
- watcher processes are still running with stale inherited env even though `.env` changed
- the temp app integration harness is launching child processes with duplicate env keys, causing stale values to win unexpectedly
- rendered integration still writes `.env.host` after the harness has intentionally removed it
- rendered compose ports are hardcoded so test-selected env ports never actually affect the dependency mapping
- rendered dependency boot logic diverges from the rendered compose file and silently stops testing the real generated contract

## Working Rule

If the bug reappears after rerender, the real fix was not made in the right place.
