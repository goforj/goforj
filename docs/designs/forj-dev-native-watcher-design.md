# forj Dev Native Watcher Design

## Purpose

This document captures a proposed redesign for `forj dev` watchers.

The current implementation shells out to `wgo` for each configured watcher. That
has worked well enough for simple projects, but the model starts leaking when
`forj dev` needs to coordinate app builds, app runtimes, embedded SPAs, and
multi-app projects.

The proposed direction is to make `forj dev` own the watcher engine directly
while preserving the useful parts of the `wgo` model:

- recursive filesystem watching
- include and exclude matching
- debounce
- postponed first runs
- process restart behavior
- custom watcher flexibility

The important change is that `.goforj.yml` should describe GoForj development
lifecycle intent. It should not have to encode raw `wgo` CLI flags for framework
conventions.

## Problem

Generated projects currently express dev behavior as raw watcher command lines:

```yaml
dev:
  watches:
    - name: Build App
      watch: -file .go -file .env -file .env.* -xdir forj -xdir _data -xfile app/wire/wire_gen\.go$ -postpone
      exec: forj build -o ./bin/app
    - name: Run App
      watch: -file ./bin/app -file .env -file .env.*
      exec: ./bin/app run
```

This gives users power, but it also exposes too much implementation detail.

Observed issues:

- `wgo` patterns are regular expressions, so readable values like `.go` do not
  mean "files ending in `.go`" unless translated carefully.
- `-xdir .` looks like "ignore dot directories" but means "ignore any directory"
  because `.` is a regex wildcard.
- `-dir ./cmd/app/frontend/dist` controls watched directories, but file events
  still need matching `-file` includes.
- Embedded SPA dev requires a dependency chain:
  SPA source change -> SPA build -> dist marker change -> app binary rebuild ->
  app restart.
- Hardcoding `./cmd/app/frontend/dist/index.html` into the default app build
  watcher does not scale to multiple apps or multiple SPAs per app.
- `run` and `watch` are related lifecycle concepts, but the current config keeps
  them apart as raw watcher definitions and a separate `dev.run` map.
- Listing every nonparticipating App as `false` makes generated config describe
  what development does not do instead of the lifecycle it owns.
- Requiring `run: run` repeats an App executable's intrinsic default launch
  behavior in watcher configuration.

The result is a configuration surface that is powerful but not clean enough for
the framework-owned default path.

## Design Goals

- Keep `forj dev` flexible enough for custom watchers.
- Make the generated config short and understandable.
- Treat `dev.apps` as a sparse participation set: omitted Apps have no inferred
  development lifecycle.
- Make app lifecycle behavior first-class:
  - build this app
  - run this app
  - rebuild when its embedded SPA output changes
  - restart when its binary changes
- Support multiple apps.
- Support multiple SPAs per app.
- Hide regex and `wgo` flag details from ordinary generated config.
- Preserve raw low-level watcher escape hatches for advanced users.
- Integrate watcher events directly with existing dev output, TUI, and
  Lighthouse devwatch streaming.
- Avoid inventing a second app model. Watchers should attach to the existing
  GoForj App concept.
- Use the App's existing runtime capability to decide whether conventional
  participation includes a runtime process.
- Launch conventional runtime Apps through their bare executable instead of
  restating `run` in watcher config.

## Non-Goals

- Replacing `fsnotify` with a novel filesystem watcher.
- Removing custom watchers.
- Making `forj dev` a task runner for unrelated production operations.
- Encoding every possible build system as a first-class framework feature.
- Requiring all Apps to participate during development. Omitted Apps remain
  completely outside the inferred lifecycle graph.
- Removing build-only participation. A listed App can explicitly disable its
  runtime while retaining build and watch behavior.

## Proposed Config Shape

The generated config should be small and app-centered.

For Ship, a realistic generated shape could be:

```yaml
dev:
  apps:
    app:
      spas:
        portal: ./cmd/app/frontend
```

This means:

- `app` participates in the dev loop.
- because `app` is runtime-capable, it builds and runs with `./bin/app`.
- `app` owns one SPA named `portal`.
- the `portal` SPA lives at `./cmd/app/frontend`.
- `ship`, `ship-agent`, and `ship-sentinel` are absent, so GoForj creates no
  inferred build, SPA, or runtime nodes for them.

GoForj expands this into the conventional watcher graph internally.

### Sparse App Participation

`dev.apps` is an allowlist, not an inventory of every App in the project.

The target semantics are:

| Configuration | Inferred lifecycle |
| --- | --- |
| App key absent | no build, SPA, or runtime nodes |
| `app: true` | conventional build; conventional runtime when runtime-capable |
| App mapping | conventional lifecycle plus the listed overrides |
| App mapping with `run: false` | build and watch only |
| CLI-only App mapping with no `run` | build and watch only |
| App mapping with scalar `run` | build and run that App command |
| App mapping with a `run:` object | build and run the explicit runtime override |

App-level `false` is not part of the target schema. Absence is the only
App-level exclusion mechanism, so generated configuration should never contain:

```yaml
dev:
  apps:
    ship: false
```

`app: true` is only a concise inclusion shorthand for an App with no overrides.
A mapping is used when the App owns SPAs or changes conventional behavior.

Runtime capability comes from the existing App component model. An App is
runtime-capable when it enables Web API, Web UI, Scheduler, or Jobs. The watcher
must not infer capability from the presence of a binary, filesystem layout, or
an App name.

A listed CLI-only App builds but does not launch by default because its bare
executable intentionally prints help. CLI-only Apps such as `ship`,
`ship-agent`, and `ship-sentinel` are normally omitted unless a developer wants
their binaries rebuilt during `forj dev`.

### App Defaults

For a listed App, GoForj can infer:

```yaml
build:
  exec: forj <app> build -o ./bin/<app>
  watch: [.go, .env, .env.*]
  ignore: [forj, _data, wire_gen.go]

run: # runtime-capable Apps only, unless explicitly overridden
  exec: ./bin/<app>
  watch: [./bin/<app>]
```

The build node is conventional for every listed App. The runtime node is
conventional only when the App is runtime-capable and `run` is not `false`.

The bare executable is sufficient because the
[`App Default Launch Design`](completed/app-default-launch-design.md) makes zero-argument
execution intrinsic: runtime-capable Apps enter `run`, while CLI-only Apps print
help.

For the default app named `app`, the build command remains:

```text
forj build -o ./bin/app
```

For a named app such as `billing`, the build command becomes:

```text
forj billing build -o ./bin/billing
```

### Default Development Runtime Topology

The completed
[`app run` single-process host design](completed/app-run-single-process-design.md)
left `forj dev` topology unchanged unless a separate design chose otherwise.
This watcher design is that separate decision for the structured `dev.apps`
path.

A listed runtime-capable App with no `run` override launches its bare binary,
which enters the standalone host:

```text
forj dev -> ./bin/<app> -> run -> standalone runtime host
```

An explicit leaf-runtime development topology remains available through a
scalar `run` command or a `run:` mapping. Legacy raw watchers also retain their
configured commands during migration. The native watcher must not silently
rewrite an explicit leaf command into the standalone host.

### SPA Defaults

For each SPA path, GoForj can infer:

```yaml
build: npm run build -s -- --logLevel silent
watch: [.ts, .js, .vue, .css, .html, package.json, package-lock.json]
ignore: [_data, node_modules, dist]
dist: <spa path>/dist/index.html
```

The SPA dist marker is automatically added as an app build dependency. Users
should not need to list it in the app build watcher.

### Explicit App Overrides

When defaults are not enough:

```yaml
dev:
  apps:
    app:
      build:
        exec: forj build -o ./bin/app
        watch: [.go, .env, .env.*, ./cmd/app/frontend/dist/index.html]
        ignore: [forj, _data, app/wire/wire_gen.go]
      run:
        exec: ./bin/app http:serve
        watch: [./bin/app]
      spas:
        portal:
          path: ./cmd/app/frontend
          build: npm run build -s -- --logLevel silent
          watch: [.ts, .js, .vue, .css, .html, package.json, package-lock.json]
          ignore: [_data, node_modules, dist]
```

This is intentionally more verbose. It exists for projects that need control,
not as the generated default.

The `run:` mapping is an advanced process override. It implies that a runtime
node should be created and its `exec` value is used exactly as configured. This
keeps explicit leaf-runtime development available without making it the
framework default.

The scalar form remains a concise override for a command executed through the
App binary:

```yaml
dev:
  apps:
    app:
      run: http:serve
```

This expands to `./bin/app http:serve`. The redundant scalar `run: run` should
not be generated; omitting `run` already represents the conventional standalone
command. It may remain accepted while existing structured config migrates.

For the uncommon build-only case:

```yaml
dev:
  apps:
    app:
      run: false
```

This creates the App build node and its SPA dependencies, but no runtime node.
The negative value is attached to the one exceptional behavior being disabled
instead of requiring `false` entries for every unrelated App.

### Multiple SPAs

Apps can own more than one SPA:

```yaml
dev:
  apps:
    app:
      spas:
        portal: ./cmd/app/frontend
        admin:
          path: ./cmd/app/admin
          build: npm run build -s -- --logLevel silent
          watch: [.ts, .js, .vue, .css, .html]
          ignore: [node_modules, dist]
```

GoForj adds both SPA dist markers to the owning app build dependencies:

```text
./cmd/app/frontend/dist/index.html
./cmd/app/admin/dist/index.html
```

### Custom Watchers

Custom watchers remain available for work that is not app lifecycle behavior:

```yaml
dev:
  watches:
    - name: docs
      exec: make docs
      watch: [.md]
      ignore: [_data]
```

For full escape-hatch control, custom watchers can allow raw regexes:

```yaml
dev:
  watches:
    - name: api index
      exec: forj api-index
      watch: [re:^internal/.+\.go$]
      ignore: [re:^internal/.+_test\.go$]
```

The exact raw marker is open for design. The key requirement is that ordinary
generated config should not require raw regexes.

## Matcher Semantics

The config should expose simple matchers and compile them into native matcher
rules internally.

Suggested simple matcher behavior:

| Config value | Meaning |
| --- | --- |
| `.go` | files ending in `.go` |
| `.env` | a file named `.env` |
| `.env.*` | files beginning with `.env.` |
| `package.json` | a file named `package.json` |
| `wire_gen.go` | a file named `wire_gen.go` |
| `forj` | a directory or file path segment named `forj` |
| `./cmd/app/frontend/dist/index.html` | exact project-relative path |
| `re:<expr>` | explicit regular expression |

This keeps the common case readable while preserving advanced matching.

Internally, these can compile into typed matchers:

```text
Suffix(".go")
ExactBasename(".env")
BasenamePrefix(".env.")
ExactPath("cmd/app/frontend/dist/index.html")
Regex(...)
```

The native watcher does not need to compile everything back into `wgo` flags.
Once `forj dev` owns the watcher engine, matcher semantics can be typed and
tested directly.

## Native Watcher Engine

`forj dev` should embed the watcher engine instead of starting `wgo` processes.

The engine should provide:

- recursive watch roots
- include matchers
- exclude matchers
- directory pruning
- event debounce
- optional polling fallback
- postponed first execution
- process lifecycle management
- restart-on-change behavior

The public config should compile into internal watcher specs:

```go
type DevWatcherSpec struct {
	Name      string
	Root      string
	Includes  []Matcher
	Excludes  []Matcher
	Postpone  bool
	Command   DevCommandSpec
	Restart   bool
	App       string
	Kind      DevWatcherKind
}
```

Potential watcher kinds:

```text
app_build
app_run
spa_build
custom
```

The kind gives the dev output layer enough context to render clean lifecycle
messages without parsing watcher names.

## Lifecycle Graph

The important difference from raw watcher orchestration is that `forj dev`
should know dependencies between watchers.

These inferred graphs are created only for Apps present in `dev.apps`.

For an app with one SPA:

```text
SPA source files
  -> spa_build
  -> SPA dist marker
  -> app_build
  -> app binary
  -> app_run restart
```

For an app with no SPA:

```text
Go/env files
  -> app_build
  -> app binary
  -> app_run restart
```

For a listed CLI-only App or an App with `run: false`, the graph ends at the
binary:

```text
Go/env files or SPA output
  -> app_build
  -> app binary
```

No `app_run` node exists in that graph.

For an app with multiple SPAs:

```text
portal source -> portal build -> portal dist
admin source  -> admin build  -> admin dist
portal/admin dist or Go/env files -> app_build -> app_run restart
```

This graph should be explicit enough to avoid duplicate restarts and noisy
build loops.

## Process Behavior

The current `forj dev` behavior already has useful process semantics:

- watchers start together
- subprocess output streams into the transcript
- app restarts are visible
- shutdown stops children in parallel
- build progress markers feed the dev TUI

The native watcher engine should preserve those behaviors, but without each
watcher being a separate `wgo` process.

Suggested process behavior:

- Build commands are one-shot subprocesses.
- Runtime commands are long-running subprocesses.
- The conventional runtime command is the bare `./bin/<app>` executable.
- An explicit `run.exec` command is preserved exactly.
- A pending build cancels or coalesces duplicate build requests during debounce.
- A runtime restarts only after its corresponding build succeeds.
- Failed builds do not restart the runtime.
- Runtime exits are surfaced as watcher failures unless shutdown is intentional.
- App shutdown still happens in parallel.

## Dev Output

Native events should improve output because `forj dev` will see file events,
build requests, skipped restarts, and dependency edges directly.

Useful event types:

```text
watcher.started
watcher.stopped
file.changed
build.queued
build.started
build.succeeded
build.failed
runtime.started
runtime.restarting
runtime.stopped
runtime.failed
```

The dev TUI and Lighthouse devwatch stream can consume these structured events
instead of inferring lifecycle from child process output.

## Migration Strategy

This should not require a flag day.

### Backwards Compatibility

Existing projects should continue to run under `forj dev` after the native
watcher engine lands.

Compatibility rules:

- Existing `dev.watches` entries remain valid.
- Existing raw `watch:` strings continue to mean "use the legacy `wgo` flag
  grammar."
- Custom watcher names, commands, env values, and ordering should be preserved.
- If GoForj cannot confidently translate a legacy raw watcher into structured
  config, it should execute it through the compatibility path instead of
  rewriting it.
- Generated legacy watchers can be recognized by name and command shape, but
  hand-written watchers should not be rewritten unless the user asks for it.

The native implementation can parse the subset of `wgo` flags GoForj generated
historically:

```text
-cd
-file
-dir
-xfile
-xdir
-postpone
```

Unknown legacy flags should produce a clear compatibility warning and either:

- continue through a raw `wgo` subprocess compatibility mode during the
  transition, or
- fail with a focused message if the raw compatibility mode has been removed in
  a later major version.

### Auto-Migration

GoForj can offer an automatic migration path for generated watcher config.

The migration should be conservative:

- Detect only known generated watcher shapes.
- Convert a known App build/run watcher pair into a listed `dev.apps` entry with
  no `run` field.
- Convert a known build-only App watcher into a listed entry with `run: false`.
- Convert generated NPM frontend watchers into `spas`.
- Omit known nonparticipating Apps instead of writing App-level `false` values.
- Preserve a nonstandard App command as an explicit scalar `run` value. Preserve
  full process overrides as a `run:` mapping.
- Preserve unknown `dev.watches` entries as custom watchers.
- Preserve comments and formatting where practical, but correctness matters more
  than perfect YAML layout.
- Show a diff or dry-run mode before writing changes.

Possible command:

```text
forj config migrate-dev-watchers
```

Possible render behavior:

- `forj render` can normalize legacy generated watcher bugs, such as bad
  frontend excludes.
- `forj render` should not silently replace a user's raw custom watcher with a
  structured watcher.
- If a project still uses generated legacy watcher config, `forj render` can
  print a one-time suggestion pointing to the migration command.

The goal is that old projects keep working immediately, while projects can move
to the cleaner `dev.apps` model when they are ready.

Phase 1:

- Add native watcher config structs.
- Keep existing `dev.watches[].watch` raw `wgo` strings working.
- Add parsing for `dev.apps`.
- Compile `dev.apps` into the same internal watcher list used by raw watches.

Phase 2:

- Replace `wgo` subprocess startup with native watcher execution.
- Keep raw `watch:` strings as compatibility input by parsing the subset of
  existing flags GoForj generates today.
- Prefer structured `watch: [...]` and `ignore: [...]` for new configs.

Phase 3:

- Update generated `.goforj.yml` to use `dev.apps`.
- Generate only participating App keys.
- Omit `run` for the conventional standalone runtime.
- Render old `dev.watches` only for custom watches.
- Normalize or migrate generated old watcher strings during render.

Phase 4:

- Decide whether raw `wgo` syntax remains permanently supported or becomes an
  explicit legacy escape hatch.

## Implementation Notes

The existing `internal/forj/dev_cmd.go` already owns app expansion and process
orchestration. The native watcher work should keep those responsibilities but
replace the shell-out boundary.

Likely implementation areas:

- `project.Config`
  - add structured dev app/watch config
  - preserve existing raw watcher config during migration
- `internal/forj/dev_cmd.go`
  - compile config into watcher specs
  - build lifecycle graph
  - run native watcher engine
- `internal/forj/dev_watch_engine.go`
  - recursive fsnotify/polling engine
  - debounce
  - matcher evaluation
- `internal/forj/dev_process.go`
  - one-shot build process execution
  - runtime process restart logic
- `internal/forj/dev_tui.go`
  - consume structured lifecycle events
- `internal/forj/devwatch_streamer.go`
  - stream structured dev events to Lighthouse

The names are illustrative. The actual split should follow the existing file
boundaries once implementation starts.

Structured App expansion must follow this order:

1. Iterate only keys explicitly present in `dev.apps`.
2. Resolve each key against the project's existing App model.
3. Always create the conventional build node unless `build` explicitly changes
   that behavior.
4. If `run: false`, stop the graph at the build output.
5. If a scalar `run` command is present, append it to the conventional App
   binary and create that runtime node.
6. If a `run:` mapping is present, create the configured runtime node.
7. Otherwise, create a bare-binary runtime node only when the App is
   runtime-capable.

The compiler must not scan `cmd/`, the filesystem, or the project App registry
and silently enroll omitted Apps. Discovery can validate configured names, but
it cannot expand the participation set.

The config representation needs to distinguish four runtime states:

```text
run omitted  -> capability-derived default
run false    -> explicit build-only behavior
run scalar   -> explicit App command suffix
run mapping  -> explicit runtime override
```

It does not need an App-level `Enabled` state in the target model. Map presence
already represents inclusion.

Framework-owned config generation and migration should normalize to this model:

| Input intent | Stored `dev.apps` shape |
| --- | --- |
| Initial runtime-capable default App | listed App with no `run` field |
| Additional App not selected for dev | omitted App key |
| Additional App selected with conventional `run` | listed App with no `run` field |
| Additional App selected with a custom App command | listed App with a scalar `run` |
| Additional App selected with a custom process | listed App with a `run.exec` override |
| Legacy App explicitly built without running | listed App with `run: false` |

For an existing command-suffix input such as `queue:work`, the generator or
migration owns expanding it to a complete runtime command:

```yaml
dev:
  apps:
    worker:
      run: queue:work
```

This expands transparently to `./bin/worker queue:work`. A `run.exec` mapping is
reserved for callers that need to replace the complete process command.

## Testing Strategy

The test suite should cover:

- simple matcher translation
- raw regex matcher behavior
- directory pruning
- exact path matching for SPA dist markers
- app expansion for default and named apps
- multiple SPAs under one app
- omitted App creates no build, SPA, or runtime nodes
- listed runtime-capable App builds and launches its bare binary
- listed runtime-capable App with `run: false` creates no runtime node
- listed CLI-only App builds without implicitly launching help
- generated config omits CLI-only Apps that have no dev participation
- explicit scalar and mapped `run` overrides preserve their respective command
  semantics
- mixed runtime and CLI-only projects do not enroll Apps by discovery
- app build followed by app run restart
- SPA source change followed by SPA build, app build, and app restart
- build failure does not restart runtime
- duplicate file changes coalesce through debounce
- shutdown stops runtimes in parallel
- raw legacy watcher strings still work during migration

For integration tests that render projects, test renders must happen under
`/tmp`, not inside the GoForj repository.

## Open Questions

- Should SPA default detection be automatic when `cmd/<app>/frontend/package.json`
  exists, or should generated config list SPAs explicitly?
- Should `watch: [.go]` match by suffix everywhere, or only file basenames?
- Should directory ignores and file ignores share one `ignore` list, or should
  they be split for precision?
- What raw escape hatch is least confusing: `re:<expr>`, `regex:<expr>`, or a
  separate `raw_watch` field?
- Should polling fallback be configurable per watcher or globally?
- How much of `wgo`'s existing code can be reused directly, and what license or
  maintenance implications does that create?

## Desired End State

The generated Ship-style dev config should be readable enough to understand at
a glance:

```yaml
dev:
  apps:
    app:
      spas:
        portal: ./cmd/app/frontend
```

From that, `forj dev` should know how to:

- watch app Go/env inputs
- watch SPA source inputs
- build SPA output
- rebuild the embedding app when SPA output changes
- restart the app when the binary changes
- leave omitted Apps completely unmanaged
- keep custom watchers available for project-specific work

The framework still provides the flexibility that made `wgo` useful, but the
default experience becomes app lifecycle configuration instead of a bag of
watcher flags.
