# forj Dev Native Watcher Design

## Purpose

This document records the implemented contract for native `forj dev` watchers.

The previous implementation shelled out to `wgo` for each configured watcher.
That worked well enough for simple projects, but the model started leaking when
`forj dev` needs to coordinate app builds, app runtimes, embedded SPAs, and
multi-app projects.

`forj dev` now owns the watcher engine directly while preserving the useful
parts of the `wgo` model:

- recursive filesystem watching
- include and exclude matching
- debounce
- postponed first runs
- process restart behavior
- custom watcher flexibility

The important change is that `.goforj.yml` describes GoForj development
lifecycle intent instead of encoding raw `wgo` CLI flags for framework
conventions.

## Problem

Generated projects previously expressed dev behavior as raw watcher command
lines:

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
  SPA source change -> SPA build -> app binary rebuild -> app restart.
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

## Implemented Config Shape

Generated config is app-centered and explicit about the processes and file
matchers that developers are expected to edit.

For Ship, a realistic generated shape is:

```yaml
dev:
  apps:
    app:
      build:
        exec: forj build -o ./bin/app
        watch: [.go, .env, .env.*]
        ignore: [forj, _data, wire_gen.go, .git, .hg, .svn, .idea, .vscode, .settings, node_modules]
        root: .
        postpone: true
      run:
        exec: ./bin/app
      spas:
        frontend:
          path: ./cmd/app/frontend
          build: npm run build -s -- --logLevel silent
          watch: [.ts, .tsx, .js, .jsx, .vue, .css, .html, package.json, package-lock.json]
          ignore: [_data, node_modules, dist]
```

This means:

- `app` participates in the dev loop with visible build and runtime commands.
- the build and SPA matcher lists are ordinary editable configuration.
- `app` owns one SPA named `frontend` rooted at `./cmd/app/frontend`.
- `ship`, `ship-agent`, and `ship-sentinel` are absent, so GoForj creates no
  inferred build, SPA, or runtime nodes for them.

GoForj still owns the success edges between these nested lifecycle nodes. SPA
success queues the App build, and App build success queues the runtime restart;
fake dist-file and binary-file watches are not rendered for those edges.

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
| App mapping with a `run:` object | build and run the configured complete command |

App-level `false` is invalid. Absence is the only App-level exclusion mechanism,
so generated configuration never contains:

```yaml
dev:
  apps:
    ship: false
```

`app: true` remains a concise hand-authored compatibility shorthand for an App
with conventional behavior. Framework generation writes the expanded mapping
so the effective commands and matchers are visible without reading GoForj
source.

For backward compatibility, a project with no `dev.apps` field remains on the
historical discovery path used by raw legacy watchers. An explicit
`dev.apps: {}` selects the native model with no managed Apps, which lets a
project run custom watchers without an implicit App build.

Runtime capability comes from the existing App component model. An App is
runtime-capable when it enables Web API, Web UI, Scheduler, or Jobs. The watcher
must not infer capability from the presence of a binary, filesystem layout, or
an App name.

A listed CLI-only App builds but does not launch by default because its bare
executable intentionally prints help. CLI-only Apps such as `ship`,
`ship-agent`, and `ship-sentinel` are normally omitted unless a developer wants
their binaries rebuilt during `forj dev`.

### App Defaults

For a generated App, GoForj renders:

```yaml
build:
  exec: forj <app> build -o ./bin/<app>
  watch: [.go, .env, .env.*]
  ignore: [forj, _data, wire_gen.go, .git, .hg, .svn, .idea, .vscode, .settings, node_modules]
  root: .
  postpone: true

run: # runtime-capable Apps only, unless explicitly overridden
  exec: ./bin/<app>
```

Concise hand-authored Apps that omit these fields still infer the same defaults
for compatibility. Generated configuration snapshots them so future default
changes do not silently alter an existing project's visible watcher contract.

The build node is conventional for every listed App. The runtime node is
conventional only when the App is runtime-capable and `run` is not `false`.
The [App Default Launch Design](completed/app-default-launch-design.md) makes the bare
executable enter `run` intrinsically for runtime-capable Apps, while CLI-only
Apps retain their no-argument help behavior.

The build-to-runtime edge is internal. Additional `run.watch` matchers remain
available for external restart triggers, and a conventional binary matcher is
removed during compilation to avoid duplicate restarts.

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

For each SPA path, GoForj infers:

```yaml
build: npm run build -s -- --logLevel silent
watch: [.ts, .tsx, .js, .jsx, .vue, .css, .html, package.json, package-lock.json]
ignore: [_data, node_modules, dist]
```

SPA build success directly queues the owning app build. App build success then
queues the runtime restart. This explicit success graph avoids relying on a
generated dist-marker event and prevents a failed SPA build from rebuilding or
restarting the app.

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
        exec: env MODE=dev ./tools/app-server
        watch: [.runtime-restart]
      spas:
        portal:
          path: ./cmd/app/frontend
          build: npm run build -s -- --logLevel silent
          watch: [.ts, .tsx, .js, .jsx, .vue, .css, .html, package.json, package-lock.json]
          ignore: [_data, node_modules, dist]
```

This is the same expanded shape used by generated projects. Developers can edit
the command, matchers, root, workdir, environment, or timing fields directly.
App build ignores extend the invariant VCS, editor, and `node_modules` safety
exclusions; compilation de-duplicates defaults already present in the rendered
list.

The generated `run.exec: ./bin/<app>` mapping retains the conventional managed
binary behavior, including safe executable snapshots during rebuilds. Any
other `run:` mapping is a full process override whose `exec` value is preserved
exactly and is not prefixed with the App binary. The scalar form is a concise
App-command override:

```yaml
dev:
  apps:
    app:
      run: http:serve
```

This expands to `./bin/app http:serve`. Existing scalar `run: run` values remain
accepted for backward compatibility, but framework generation writes the
complete bare-binary `run.exec` command instead. `run: false` keeps the build
and SPA graph while suppressing the runtime node.

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
          watch: [.ts, .tsx, .js, .jsx, .vue, .css, .html]
          ignore: [node_modules, dist]
```

Both SPA build nodes feed the same owning app build:

```text
portal build --+
admin build  ----+--> app build
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
      exec: forj build:api-index
      watch: [re:^internal/.+\.go$]
      ignore: [re:^internal/.+_test\.go$]
```

`build:api-index` is the actual standalone command and publishes the active
App's artifacts immediately. A named-App watcher uses
`forj <app> build:api-index`; CI-like watcher policy can add `--strict` so any
warning rejects the candidate without replacing the active artifacts. Ordinary
`forj build` and `forj run` already include API indexing and delay publication
until their final compile or process-start boundary succeeds, so a separate
index watcher is only useful when the project wants contract refreshes without
running either pipeline.

`re:` is the implemented raw marker. Ordinary generated config does not require
regular expressions.

### Structured Custom Controls

List-shaped `watch` values opt into native matching. Structured custom watchers
also support:

```yaml
dev:
  watches:
    - name: schema compiler
      roots: [schemas, internal/contracts]
      workdir: ./tools/schema
      watch: [.json, .yaml]
      ignore: [generated]
      files:
        include: [re:^v[0-9]+/.+\.json$]
        exclude: [snapshot.json]
      dirs:
        include: [schemas]
        exclude: [vendor]
      exec: make generate
      env:
        GENERATOR_MODE: dev
      debounce: 150ms
      poll: 1s
      postpone: true
      restart: false
      exit: false
      stdin: false
```

`root` is accepted as a single-root YAML convenience; `roots` is the normalized
multi-root form. `poll` selects bounded snapshot polling for that watcher. With
no `poll`, filesystem notifications are preferred and startup falls back to
polling if notification coverage cannot be established.

Outermost physical roots must exist as real directories when the watcher
starts; symbolic-link roots are rejected because notification paths cannot be
routed reliably through both lexical and resolved names. An explicit nested
root can still restore coverage inside a subtree excluded by an outer root.

## Matcher Semantics

The config exposes simple matchers and compiles them into native matcher rules.

Implemented simple matcher behavior:

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

`forj dev` embeds the watcher engine instead of starting `wgo` processes.

The engine provides:

- recursive watch roots
- include matchers
- exclude matchers
- directory pruning
- event debounce
- create, write, remove, and rename events
- filesystem notification health/error reporting
- bounded polling and automatic startup fallback
- postponed first execution
- process lifecycle management
- restart-on-change behavior

Project config compiles into `internal/devwatch.Spec` values. The engine spec is
filesystem-oriented and intentionally contains no project, app, YAML, TUI, or
Lighthouse concepts:

```go
type Spec struct {
	Name              string
	Roots             []string
	Includes          []Matcher
	Excludes          []Matcher
	DirectoryIncludes []Matcher
	DirectoryExcludes []Matcher
	Debounce          time.Duration
	DebounceSet       bool
}
```

The GoForj adapter attaches lifecycle kinds and process commands after matcher
compilation:

```text
app_build
app_run
spa_build
custom
```

The kind gives the dev output layer enough context to render clean lifecycle
messages without parsing watcher names.

### Package Boundary

`internal/devwatch` owns two reusable mechanisms:

- shared physical filesystem subscriptions, typed matchers, debounce,
  notification health, and polling
- process supervision, process-tree shutdown, restart, and exit records
- pseudo-terminal output on Linux and macOS, preserving the historical merged
  watcher stream and native color/cursor behavior

`internal/forj` owns policy:

- compiling `project.Config` into watcher specs and commands
- app/SPA lifecycle graph edges
- framework build conventions and Templ/HTMX generated-output exclusions
- transcript formatting, TUI state, sound hooks, and Lighthouse streaming
- outer-session environment reload, migration setup, and explicit rebuilds

Lifecycle wrappers consistently use the same Bash command contract as the
rest of `forj dev`, including on Windows. PTY attachment is enabled where the
platform supports it; other platforms retain separate output streams.

This boundary keeps the engine reusable without turning `.goforj.yml` or UI
concerns into low-level watcher dependencies. `internal/devwatch` is an internal
package boundary, not a promised public Go API.

## Lifecycle Graph

The important difference from raw watcher orchestration is that `forj dev`
knows dependencies between watchers.

These inferred graphs are created only for Apps present in `dev.apps`.

For an app with one SPA:

```text
SPA source files
  -> spa_build
  -> app_build
  -> app_run restart
```

For an app with no SPA:

```text
Go/env files -> app_build -> app_run restart
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
portal source -> portal build --+
admin source  -> admin build  ----+-> app_build -> app_run restart
Go/env files ---------------------+
```

The explicit graph avoids duplicate restarts and noisy build loops.

## Process Behavior

The current `forj dev` behavior already has useful process semantics:

- watchers start together
- subprocess output streams into the transcript
- app restarts are visible
- shutdown stops children in parallel
- build progress markers feed the dev TUI

The native watcher engine preserves those behaviors without making each watcher
a separate `wgo` process.

Implemented process behavior:

- Cold start builds listed apps, runs pre-dev setup, builds configured SPAs,
  rebuilds app artifacts when SPA output was published, and only then starts
  watcher runtimes.
- Build commands are one-shot subprocesses.
- Runtime commands are long-running subprocesses.
- The conventional runtime command is the bare `./bin/<app>` executable.
- An explicit `run.exec` command is preserved exactly.
- Trailing-edge debounce coalesces duplicate filesystem events.
- A change during an active build queues at most one follow-up build; it does
  not cancel the active build.
- A custom watcher with `restart: true` cancels and replaces its active command.
- A runtime restarts only after its corresponding build succeeds.
- Failed builds do not restart the runtime.
- Runtime exits are surfaced as watcher failures unless shutdown is intentional.
- App shutdown happens in parallel and uses process-group termination followed
  by forced termination after a bounded grace period.

## Dev Output

The filesystem engine exposes typed `Event` and `Health` streams to the GoForj
controller. The controller knows watcher kinds, graph edges, process exits, and
intentional stop reasons without parsing `wgo` output. It routes child output
through the existing transcript writers so the TUI and Lighthouse devwatch
stream retain their established presentation and restart separators.

There is not yet a separate public `build.started`/`runtime.restarting` event
schema. That can be added above the `internal/devwatch` boundary without
changing matcher or process supervision APIs.

## Migration and Backwards Compatibility

The implementation does not require a flag day.

### Backwards Compatibility

Existing projects should continue to run under `forj dev` after the native
watcher engine lands.

Compatibility rules:

- Existing `dev.watches` entries remain valid.
- Existing raw `watch:` strings continue to mean "use the legacy `wgo` flag
  grammar."
- Custom watcher names, commands, env values, and ordering should be preserved.
- If GoForj cannot confidently migrate a legacy raw watcher, it leaves the
  config untouched and compiles the scalar string through the native legacy
  parser.
- Generated legacy watchers can be recognized by name and command shape, but
  hand-written watchers should not be rewritten unless the user asks for it.

The native implementation supports these scalar legacy flags:

```text
-root
-cd
-file
-dir
-xfile
-xdir
-debounce
-poll
-postpone
-exit
-stdin
-verbose
-exec-log
-exec-msg
-log-prefix
```

Repeated flags, quoted values, `-flag=value`, boolean `=true`/`=false`, ordered
roots, forward-slash paths, exclude precedence, and `wgo`'s dot convenience
escaping are preserved. Unsupported flags fail with a focused message that
points users to structured fields. GoForj does not fall back to a `wgo`
subprocess.

### Auto-Migration

`forj render` performs conservative automatic migration for known generated
watcher config.

The migration is conservative:

- Detect only known generated watcher shapes.
- Convert known App build/run watcher pairs into listed `dev.apps` entries with
  explicit build and runtime snapshots.
- Convert a known build-only App watcher into a listed entry with `run: false`.
- Convert generated NPM frontend watchers into `spas`.
- Omit known nonparticipating Apps instead of writing App-level `false` values.
- Preserve legacy custom App commands as scalar `run` overrides.
- Preserve full custom process commands as `run:` mappings.
- Keep Apps outside the legacy allowlist absent.
- Preserve unknown `dev.watches` entries as custom watchers.
- Preserve semantic values and custom watcher order. Re-encoding may normalize
  YAML comments and formatting.
- Preserve the ordering of remaining custom watchers.
- Treat added env, matcher, timing, process, or path controls as evidence of
  customization and do not migrate that watcher.

The goal is that old projects keep working immediately, while projects can move
to the cleaner `dev.apps` model when they are ready.

Implemented phases:

- Add native watcher config structs.
- Keep existing `dev.watches[].watch` raw `wgo` strings working.
- Add parsing for `dev.apps`.
- Compile `dev.apps` into the same internal watcher list used by raw watches.
- Update generated `.goforj.yml` to use `dev.apps`.
- Generate only participating App keys.
- Render the conventional standalone runtime as `run.exec: ./bin/<app>`.
- Render old `dev.watches` only for custom watches.
- Normalize or migrate generated old watcher strings during render.
- Replace `wgo` subprocess startup with native watcher execution.
- Keep raw scalar `watch:` strings as a backwards-compatible input grammar.
- Preserve Lighthouse round trips for `dev.apps`, native watcher controls, and
  scalar legacy watcher strings even though the current form edits only the
  legacy scalar surface.

## Implementation Layout

- `project/config_dev.go`: YAML unions for scalar/list watches, app booleans,
  command shorthands, and SPA shorthands
- `internal/devwatch`: filesystem engine and process supervisor
- `internal/forj/dev_watcher_spec.go`: config and legacy grammar compiler
- `internal/forj/dev_watcher_runner.go`: lifecycle graph controller and output
  integration
- `internal/forj/project_dev_config.go`: generated defaults and conservative
  render migration
- generated Lighthouse config/server: lossless settings round trips

### Structured App Compilation

Structured App expansion follows this order:

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

The compiler does not scan `cmd/`, the filesystem, or the project App registry
to silently enroll omitted Apps. Discovery can validate configured names, but
it cannot expand the participation set.

The config representation distinguishes four runtime states:

```text
run omitted  -> capability-derived compatibility default
run false    -> explicit build-only behavior
run scalar   -> explicit App command suffix
run mapping  -> explicit runtime command
```

There is no App-level `Enabled` state. Map presence represents inclusion.

Framework-owned config generation and migration normalize to this model:

| Input intent | Stored `dev.apps` shape |
| --- | --- |
| Initial runtime-capable default App | listed App with explicit build and bare-binary `run.exec` |
| Additional App not selected for dev | omitted App key |
| Additional App selected with conventional `run` | listed App with explicit build and bare-binary `run.exec` |
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

This expands transparently to `./bin/worker queue:work`. Other `run.exec`
mappings let callers replace the complete process command.

## Test Coverage

The test suite covers:

- simple matcher translation
- raw regex matcher behavior
- directory pruning
- exact path matching
- app expansion for default and named apps
- multiple SPAs under one app
- omitted App creates no build, SPA, or runtime nodes
- listed runtime-capable App builds and launches its bare binary
- listed runtime-capable App with `run: false` creates no runtime node
- listed CLI-only App builds without implicitly launching help
- generated config omits CLI-only Apps that have no dev participation
- generated config exposes build, runtime, and SPA commands and matcher lists
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

## Implemented Decisions

- App map presence opts into the conventional build; omission leaves an App
  completely unmanaged. App-level `false` is rejected.
- `app: true` is the concise conventional inclusion shorthand.
- Omitted `run` launches `./bin/<app>` only when the App's components satisfy
  `WebAPI || WebUI || Scheduler || Jobs`. Listed CLI-only Apps build only.
- Scalar `run` values append an App command. The exact bare-binary
  `run.exec: ./bin/<app>` mapping is the explicit conventional form; other
  mapped `run.exec` values replace the complete process command exactly.
- `build: false` supports an existing-artifact/runtime-only app;
  `run: false` supports build-only behavior.
- SPAs are listed explicitly. GoForj generates ownership for known starter kits
  but does not infer arbitrary `package.json` files into lifecycle graphs.
- `.go` is a basename suffix matcher; named values are exact basenames; paths
  containing `/` are exact normalized relative paths.
- Shared `ignore` values apply to files and directory pruning. `files` and
  `dirs` provide scoped precision when needed.
- `re:` is the explicit regex escape hatch.
- Polling is configured per watcher by duration. Watchers with the same polling
  interval share an engine; notification-backed watchers share physical
  coverage.
- The watcher and process mechanisms were adapted from `wgo` concepts while
  correcting delete/rename handling, coverage errors, unbounded polling, build
  cancellation, and shutdown escalation. Legacy regex behavior derived from
  `wgo` remains attributed in code under its MIT license.

## Desired End State

The generated Ship-style dev config is inspectable without knowing framework
defaults:

```yaml
dev:
  apps:
    app:
      build:
        exec: forj build -o ./bin/app
        watch: [.go, .env, .env.*]
        ignore: [forj, _data, wire_gen.go, .git, .hg, .svn, .idea, .vscode, .settings, node_modules]
        root: .
        postpone: true
      run:
        exec: ./bin/app
      spas:
        frontend:
          path: ./cmd/app/frontend
          build: npm run build -s -- --logLevel silent
          watch: [.ts, .tsx, .js, .jsx, .vue, .css, .html, package.json, package-lock.json]
          ignore: [_data, node_modules, dist]
```

From that, `forj dev` knows how to:

- watch app Go/env inputs
- watch SPA source inputs
- build SPA output
- rebuild the embedding app when SPA output changes
- restart the app after its build succeeds
- leave omitted Apps completely unmanaged
- keep custom watchers available for project-specific work

The framework still provides the flexibility that made `wgo` useful, but the
default experience becomes app lifecycle configuration instead of a bag of
watcher flags.
