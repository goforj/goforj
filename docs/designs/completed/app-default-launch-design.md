# App Default Launch Design

Status:

- completed
- intended for generated GoForj App executables and `forj build`
- resolves the bare-binary launch decision deferred by the standalone runtime
  host design

## Purpose

This document defines what a generated GoForj App executable should do when it
is launched without command-line arguments.

The intended contract is:

```text
A runtime-capable App starts its normal standalone runtime.
A CLI-only App prints help.
Explicit command-line arguments always win.
```

This behavior should be part of the generated App, not a choice made while
building a particular artifact.

## Problem

Generated Apps currently have two possible no-argument behaviors.

Without special build flags:

```bash
./bin/app
```

prints root command help.

When built with either of these flags:

```bash
forj build --auto-run
forj build --default-launch run
```

the same executable invocation is rewritten to:

```bash
./bin/app run
```

The current implementation stores that choice in a linker-injected
`DefaultLaunchCommand` variable:

- [`internal/build/build_cmd.go`](../../../internal/build/build_cmd.go)
- [`templates/internal/cmd/default_launch.go.tmpl`](../../../templates/internal/cmd/default_launch.go.tmpl)
- [`templates/cmd/app/main.go.tmpl`](../../../templates/cmd/app/main.go.tmpl)

That puts an App semantic at the artifact-building boundary. Identical source
code can behave differently depending on whether it was built through plain
`go build`, a default `forj build`, or a specially flagged `forj build`.

This creates several tensions:

- operators cannot infer the executable's default behavior from the App source
- local, CI, container, and release builds can disagree
- deployment manifests need to know which build flags produced an artifact
- the generated App's actual runtime capability is ignored
- an arbitrary `--default-launch` can make packaging choices redefine the
  executable's product identity

GoForj already knows whether an App has a runtime. The generated command surface
only contains `run` when the App enables at least one runtime component. The
build command should not need to decide that again.

## Goals

- Make a runtime-capable App start its standalone runtime when launched with
  exactly zero arguments.
- Preserve no-argument help for CLI-only Apps.
- Preserve `run` as an explicit, documented command.
- Preserve every explicit argument exactly, including `--help`, `-h`, runtime
  leaf commands, and custom commands.
- Make launch behavior consistent across `go build`, `go run`, `forj build`,
  CI, and container builds.
- Derive launch behavior separately for every App in a multi-App project.
- Remove linker-injected default-command selection from the build pipeline.
- Keep the launch rule small, deterministic, and visible in generated source.

## Non-Goals

- Do not remove the explicit `run` command.
- Do not remove leaf runtime commands such as `http:serve`, `schedule:run`, or
  `queue:work`.
- Do not make CLI-only Apps expose a synthetic `run` command.
- Do not add a configurable arbitrary default command.
- Do not make `forj dev` automatically manage every runtime-capable App.
- Do not change standalone runtime topology or lifecycle behavior.
- Do not change how explicit command parse errors, exit codes, or signals are
  handled.
- Do not remove linker support used by unrelated compiled environment defaults
  and overrides.

## Decision

A generated runtime-capable App executable should treat exactly zero arguments
as `run`.

A generated CLI-only App executable should retain the existing zero-argument
help behavior.

The decision is intrinsic to each generated App entrypoint. It must not depend
on a build flag, linker value, environment variable, or deployment artifact.

The explicit `run` command remains part of the command surface. It is useful
for:

- scripts that prefer an explicit command
- documentation and command discovery
- distinguishing the combined standalone host from leaf runtime commands
- deployments that already use `app run`

The common direct-executable path simply no longer requires it.

## Invocation Contract

The generated executable contract should be:

| Invocation | Runtime-capable App | CLI-only App |
| --- | --- | --- |
| `./bin/app` | execute `run` | print root help |
| `./bin/app run` | execute `run` | report an unknown command |
| `./bin/app --help` | print root help | print root help |
| `./bin/app -h` | print root help | print root help |
| `./bin/app <command>` | execute that command | execute that command |
| `./bin/app <unknown>` | report an unknown command | report an unknown command |

"Zero arguments" is exact. GoForj should not replace arguments when any token is
present.

For a runtime-capable App, these two commands must enter the same `RunCmd`
implementation and have the same lifecycle, logging, signal, exit-code, and
runtime-topology behavior:

```bash
./bin/app
./bin/app run
```

The first is shorthand for the second. It is not a separate runtime path.

## Runtime-Capable App Detection

An App is runtime-capable when at least one of these components is enabled:

- Web API
- Web UI
- Scheduler
- Jobs

In component terms, the predicate is:

```text
WebAPI || WebUI || Scheduler || Jobs
```

This is already the predicate used to generate and inject `RunCmd` in:

- [`templates/app/root_cmd.go.tmpl`](../../../templates/app/root_cmd.go.tmpl)
- [`templates/wire/inject_cmd.go.tmpl`](../../../templates/wire/inject_cmd.go.tmpl)

The implementation should centralize this rule as a component capability, such
as `Components.HasRuntime()`, and reuse it wherever the generator needs to know
whether `run` exists. That prevents the command surface and default launch rule
from drifting apart.

The `CLI` component must not be used to classify launch behavior. GoForj Apps
have a command surface, but that does not mean every App has a long-lived
runtime.

Examples:

| Enabled components | Runtime-capable | No-argument behavior |
| --- | --- | --- |
| CLI only | no | help |
| CLI and database | no | help |
| Web API | yes | `run` |
| Web UI | yes | `run` |
| Scheduler only | yes | `run` |
| Jobs only | yes | `run` |

## Multi-App Behavior

Runtime capability belongs to an App, not to the project as a whole.

A single project can validly contain:

```text
app             Web API + Web UI
ship            CLI only
ship-agent      CLI only
ship-sentinel   CLI only
```

The resulting binaries should behave independently:

```text
./bin/app             -> run
./bin/ship            -> help
./bin/ship-agent      -> help
./bin/ship-sentinel   -> help
```

The generated `internal/cmd` package is shared across App entrypoints, while
each `cmd/<app>/main.go` is rendered with that App's component set. The launch
decision therefore must be encoded at, or passed from, each generated
entrypoint. A project-global generated variable would be incorrect for mixed
runtime and CLI-only Apps.

## Generated Launch Behavior

The current entrypoint applies `EffectiveLaunchArgs` before environment loading,
preboot dispatch, Wire initialization, and command execution.

The implementation should preserve that ordering while replacing the
linker-populated global with an App-specific render-time value. Conceptually:

```go
args := cmd.EffectiveLaunchArgs(os.Args[1:], appHasRuntime)
```

The helper should:

1. return explicit arguments unchanged
2. return `[]string{"run"}` for a runtime-capable App with zero arguments
3. return an empty argument slice for a CLI-only App with zero arguments

The exact generated API may differ, but it must not accept an arbitrary default
command from build state. The generated entrypoint's component capability is
the source of truth.

This normalization only selects the command. It must not bypass the existing
environment load, preboot handling, application initialization, lifecycle, or
Kong command execution paths.

## Build Command Changes

The target design removes these build options:

```text
--auto-run
--default-launch
```

It also removes the launch-specific linker path:

```text
-X <module>/internal/cmd.DefaultLaunchCommand=<command>
```

`--auto-run` becomes redundant once runtime-capable Apps run by default.
`--default-launch` conflicts with the stronger invariant that App behavior comes
from the App definition rather than from artifact packaging.

The flags were added recently and are not used by repository documentation or
repository call sites outside tests. They should be removed in the same
implementation rather than preserved as a permanent compatibility surface.

Release notes should give these migrations:

```text
forj build --auto-run
-> forj build
```

```text
forj build --default-launch run
-> forj build
```

Deployments that intentionally want a leaf command should keep that intent in
the invocation:

```text
./bin/app http:serve
./bin/app schedule:run
./bin/app queue:work
```

Removing launch linker injection must be scoped carefully. The build pipeline's
module-path resolution and linker-flag merging are also used by compiled
environment defaults and overrides; those unrelated behaviors remain intact.

## No Default-Command Escape Hatch

This design does not replace the build flags with an App configuration field
such as:

```yaml
default_command: http:serve
```

That would move semantic variability without demonstrating a product need for
it.

The generated App already has two clear launch modes:

- bare or explicit `run` for the combined standalone host
- an explicit leaf command for a narrower runtime role

If a future use case requires a different intrinsic executable identity, it
should be designed at the App-definition level with its own constraints. It
should not be retained speculatively in this design.

## Relationship To The Standalone Runtime Host

The completed
[`app run` single-process host design](app-run-single-process-design.md)
made `run` the canonical standalone path and deliberately deferred whether bare
execution should enter it.

This design resolves that deferred decision:

```text
bare runtime-capable App execution -> app run -> standalone runtime host
```

It does not change what `RunCmd` hosts. Bare execution and explicit `run` use the
same command and therefore the same standalone topology.

## Relationship To `forj dev`

Default launch answers how a selected runtime App executable starts. It does not
answer whether an App participates in development watching.

The
[`forj dev` native watcher design](forj-dev-native-watcher-design.md)
should continue to use sparse App configuration to select participation. For
example:

```yaml
dev:
  apps:
    app:
      spas:
        portal: ./cmd/app/frontend
```

CLI-only Apps that do not participate remain absent:

```text
ship
ship-agent
ship-sentinel
```

For a listed runtime-capable App with no explicit runtime override, the watcher
executes:

```text
./bin/app
```

instead of redundantly encoding:

```text
./bin/app run
```

This does not require the watcher to infer participation from runtime
capability. Absence from `dev.apps` still means the watcher does not manage that
App. `run: false` selects build-only participation, while an explicit scalar or
mapped `run` value selects a leaf or custom runtime command.

## Compatibility And Migration

This is an intentional behavior change for rerendered runtime-capable Apps.

Before:

```text
./bin/app -> help and exit
```

After:

```text
./bin/app -> start the standalone runtime and remain running
```

The main compatibility risk is a script or health check that executes the
binary without arguments and expects it to print help or exit immediately. Such
callers must become explicit:

```bash
./bin/app --help
```

Existing explicit runtime invocations remain valid:

```bash
./bin/app run
```

CLI-only Apps do not change. Their no-argument behavior remains help.

Deployment manifests may remove the redundant `run` argument, but they are not
required to do so. Keeping the explicit command is fully supported.

The change should be called out in release notes because the new no-argument
runtime process blocks until shutdown instead of returning immediately.

## Risks

### Accidental Runtime Startup

A developer who invokes a runtime binary with no arguments may start services
when they intended to inspect help.

This is conventional executable behavior for an application server, and the
explicit discovery path remains:

```bash
./bin/app --help
```

CLI-only executables retain their safer help default.

### Capability Drift

If default launch and `RunCmd` generation use separate predicates, an App could
default to a command that was not generated.

The implementation must use one shared `HasRuntime` capability for both.

### Multi-App Leakage

A shared project-level default could cause CLI-only Apps to inherit the runtime
App's launch behavior.

The implementation must render or pass the decision per App entrypoint and add
mixed-capability integration coverage.

### Partial Build-Flag Removal

Removing generic linker helpers would accidentally break compiled environment
defaults and overrides.

The implementation should remove only launch-specific fields, validation,
linker values, and tests.

## Implementation Plan

- [x] Add one shared runtime-capability predicate for App components.
- [x] Use that predicate for `RunCmd` generation and injection.
- [x] Render the no-argument launch decision per `cmd/<app>/main.go`.
- [x] Preserve explicit arguments byte-for-byte and in their original order.
- [x] Keep CLI-only no-argument help behavior.
- [x] Remove the generated linker-populated `DefaultLaunchCommand` variable.
- [x] Remove `--auto-run` and its launch linker injection.
- [x] Remove `--default-launch` and its launch linker injection.
- [x] Preserve compiled env default and override linker behavior.
- [x] Update template, build, render, and integration tests.
- [x] Update user-facing build and runtime documentation.
- [x] Document the no-argument behavior change and build-flag removal.

## Test Plan

### Component Capability Tests

- CLI-only and database-only Apps report no runtime capability.
- Web API, Web UI, Scheduler, and Jobs each independently report runtime
  capability.
- The capability used by default launch matches the capability used to generate
  `RunCmd`.

### Generated Template Tests

- Runtime-capable entrypoints default zero arguments to `run`.
- CLI-only entrypoints preserve zero arguments.
- Explicit arguments are never replaced.
- Generated code no longer contains `DefaultLaunchCommand`.
- Multi-App entrypoints receive their own component-derived behavior.

### Rendered Integration Tests

All rendered test projects must be created under `/tmp`.

- A runtime-capable binary launched bare reaches `RunCmd`.
- The same binary launched with explicit `run` reaches the same path.
- A runtime-capable binary launched with `--help` prints help without starting
  its runtimes.
- A runtime-capable binary preserves custom and leaf commands.
- A CLI-only binary launched bare prints root help.
- A mixed multi-App project gives its runtime and CLI-only binaries independent
  behavior.
- Plain `go build` and default `forj build` produce the same launch semantics.

### Build Command Tests

- Build arguments no longer include launch-specific linker values.
- Launch-flag tests are removed or replaced with removal diagnostics as
  appropriate for the CLI parser.
- Existing linker merging for compiled environment defaults and overrides
  remains covered.

## Acceptance Criteria

The implementation is complete when:

- every runtime-capable generated App executes `run` when launched with zero
  arguments
- every CLI-only generated App prints help when launched with zero arguments
- explicit `run` remains valid and behaviorally equivalent to bare runtime
  launch
- `--help`, `-h`, custom commands, and leaf runtime commands remain explicit and
  unchanged
- mixed runtime and CLI-only Apps in one project receive independent behavior
- launch behavior is identical across ordinary Go and GoForj build paths
- no generated or framework code uses `DefaultLaunchCommand`
- `forj build` no longer exposes `--auto-run` or `--default-launch`
- compiled environment linker features continue to work
- the watcher can launch a selected standalone runtime App without appending
  `run`

## Deferred Decisions

- Whether a future App-definition-level launch policy is needed for a concrete
  use case that cannot use explicit commands.

This deferred decision does not change the core executable contract in this
design.

## Recommendation

Adopt the intrinsic launch contract now:

```text
runtime-capable App + zero args -> run
CLI-only App + zero args        -> help
explicit args                   -> unchanged
```

Remove build-time launch selection so every artifact produced from the same App
has the same default behavior.
