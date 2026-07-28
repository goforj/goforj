# App Asset Build Pipeline Design

## Status

- Design status: proposed
- Planning date: 2026-07-28
- Target repository: `goforj`
- Primary scope: `forj build`, App-owned frontend assets, incremental build state, `forj dev`, multi-App Projects, and starter-kit release workflows
- Related package boundaries: `internal/build`, a proposed `internal/appassets` package, `internal/forj`, and `project`

## Summary

`forj build` should produce a complete deployable App binary.

For an App with an embedded frontend, that means the frontend output must match the application source before Go compilation embeds it. The command should not require users or CI systems to remember a separate `npm run build`, and it must not silently compile stale `frontend/dist` files.

This does not mean every `forj build` should launch Node.

The recommended model is:

1. App-owned asset builds become part of the App build graph rather than a `forj dev`-only concern.
2. `forj build` prepares only the selected App's configured assets.
3. A successful asset build records a content fingerprint.
4. An unchanged build verifies the fingerprint and skips dependency installation and asset tooling without launching Node.
5. `forj dev` uses the same asset preparer and successful-build record, while retaining its optimized watcher and multi-SPA coordination.
6. Asset failure prevents Go compilation, so a successful `forj build` cannot publish a binary with known-stale embedded assets.

The normal release command becomes:

```bash
forj build
```

Low-level `go build` remains available when a user intentionally wants to compile the current filesystem state without GoForj preparation.

## Decision

Adopt an incremental App asset stage in `forj build`.

Do not:

- run every configured asset command on every Go build;
- infer npm merely because Web UI is enabled;
- treat file modification times as sufficient freshness proof;
- keep production asset ownership permanently under `dev.apps`;
- silently reinterpret arbitrary legacy development watchers as production build steps; or
- publish a Go binary after a required asset build fails.

The default mode should be `auto`: build missing or stale assets and otherwise take a Node-free fast path.

## Current Behavior

### `forj build`

The current pipeline is:

```text
generate
  -> templ generate when templ + htmx is selected
  -> Wire
  -> API index preparation
  -> go build
  -> publish API index
```

The pipeline has no frontend preparation stage.

Generated Web UI entrypoints embed:

```go
//go:embed all:frontend/dist/*
```

As a result, a standalone `forj build` trusts whatever is already present in `frontend/dist`. A clean Project works because starter output or a placeholder exists, but a successful build does not prove that embedded assets reflect current frontend source.

### `forj dev`

Structured development configuration already models the correct dependency edge:

```text
frontend source change
  -> SPA build
  -> owning App build
  -> runtime replacement
```

On startup, generated npm-backed starter kits run dependency setup, build their configured SPAs, build the App, and then start the runtime. During steady state, multiple changed SPAs owned by one App join into one App build. A failed SPA build suppresses that publication wave.

This behavior is correct, but it exists only inside the development lifecycle.

### Release and CI

Users currently have to reproduce part of the dev graph manually:

```bash
cd cmd/app/frontend
npm ci
npm run build
cd ../../..
forj build
```

That is easy to omit, varies across starter kits, and allows a stale embedded frontend to look like a successful release artifact.

## Problem

### The build command does not guarantee a complete artifact

`forj build` is already more than a wrapper around `go build`. It generates project source, runs starter-specific generation, updates Wire, prepares the API index, and compiles the selected App.

Leaving embedded assets outside that contract makes the final artifact only conditionally complete.

### Always rebuilding would make the command unnecessarily slow

Launching npm and Vite after every Go-only change would add seconds to a command whose common path should remain responsive.

The design therefore needs two properties at once:

- complete artifact correctness; and
- a fast, Node-free unchanged-asset path.

### Development configuration is not the right permanent production source of truth

The current structured SPA definitions live at:

```yaml
dev:
  apps:
    app:
      spas:
        frontend:
          path: ./cmd/app/frontend
          build: npm run build -s -- --logLevel silent
```

Those fields contain useful path, command, and input information. However, omission from `dev.apps` intentionally means that `forj dev` does not manage that App. It should not also mean that a production build may embed stale assets.

Production build ownership must be explicit outside the development participation allowlist.

### Freshness cannot rely on `dist` existence or timestamps

`dist` may exist while being stale. Git checkouts, restored caches, generated files, failed builds, and filesystems with coarse timestamp resolution make timestamp-only checks unreliable.

The build needs a successful content-based record.

## Goals

1. Make `forj build` produce a complete selected-App artifact.
2. Keep unchanged frontend builds Node-free.
3. Keep the Go-only fast path close to current `forj build` performance.
4. Build assets only for the selected App.
5. Support multiple assets or SPAs owned by one App.
6. Preserve `forj dev` batching, watcher behavior, and last-known-good runtime semantics.
7. Make a clean CI checkout buildable with one framework command.
8. Fail before Go compilation when required asset preparation fails.
9. Keep custom package managers and build commands possible.
10. Introduce configuration additively and migrate only framework-owned generated shapes automatically.
11. Keep asset output App-owned and inspectable.
12. Provide useful timing and cache diagnostics without adding routine noise.

## Non-Goals

1. Turning `forj build` into a general-purpose task runner.
2. Inferring frontend tooling from the presence of `package.json`.
3. Treating every Web UI App as npm-backed.
4. Replacing Vite, npm, pnpm, Yarn, Bun, templ, Tailwind, or other asset tools.
5. Watching files inside `forj build`.
6. Deploying assets to a CDN or external static host.
7. Making `dist` framework-owned source.
8. Reinterpreting arbitrary `dev.watches` entries as release steps.
9. Guaranteeing reproducibility for a custom asset command that is itself nondeterministic.
10. Removing `go build` as a lower-level escape hatch.
11. Automatically registering or serving every custom asset output. Application source still decides how an output is embedded or consumed.

## Design Principles

### `forj build` publishes an App, not merely a Go package

The framework command should prepare every framework-declared input needed by the selected App artifact.

### Fast means skipping work, not weakening correctness

The unchanged path should avoid starting asset tooling. It should not achieve speed by trusting an unverified `dist` directory.

### Build ownership and development participation are different

An App can own a required build asset even when `forj dev` is not managing that App.

### Successful state advances atomically

The asset fingerprint advances only after the configured build succeeds and required outputs exist.

### Configuration is explicit at integration boundaries

GoForj may provide conventional shorthand, but the effective asset root, commands, inputs, and outputs should be visible and editable.

## Terminology

### App asset

A named build input owned by an App whose output is required by that App's compiled artifact.

The initial use is an embedded frontend, but the model is intentionally broader than "SPA." A Project may eventually use it for a second frontend, generated static documentation, or another deterministic embedded asset bundle.

### Asset preparation

Dependency preparation followed by the configured asset build when the asset is missing or stale.

### Asset fingerprint

A digest of the effective asset contract and its declared inputs. It proves that a successful build ran for that exact state.

### Asset manifest

The local cache record containing the fingerprint, dependency fingerprint, declared outputs, command identity, and successful completion metadata.

## Proposed Configuration

Add an App build graph outside `dev`:

```yaml
build:
  apps:
    app:
      assets:
        frontend:
          path: ./cmd/app/frontend
          prepare:
            exec: npm ci --no-audit --no-fund --loglevel=error
            inputs: [package.json, package-lock.json]
            outputs: [node_modules]
          build:
            exec: npm run build -s -- --logLevel silent
            inputs: ["**/*"]
            ignore: [_data, node_modules, dist, .git, .goforj, "**/*.tsbuildinfo"]
            outputs: [dist]
            environment: [APP_ENV, APP_APP_ENV, APP_URL, APP_APP_URL, APP_FRONTEND_*, FRONTEND_*, VITE_*, NODE_ENV]
```

For an additional App:

```yaml
build:
  apps:
    admin:
      assets:
        frontend:
          path: ./cmd/admin/frontend
          prepare:
            exec: npm ci --no-audit --no-fund --loglevel=error
            inputs: [package.json, package-lock.json]
            outputs: [node_modules]
          build:
            exec: npm run build -s -- --logLevel silent
            inputs: ["**/*"]
            ignore: [_data, node_modules, dist, .git, .goforj, "**/*.tsbuildinfo"]
            outputs: [dist]
            environment: [APP_ENV, ADMIN_APP_ENV, APP_URL, ADMIN_APP_URL, FRONTEND_*, VITE_*, NODE_ENV, ADMIN_FRONTEND_*]
```

The first implementation should support a scalar shorthand:

```yaml
build:
  apps:
    app:
      assets:
        frontend: ./cmd/app/frontend
```

The shorthand expands to the conventional npm-backed frontend contract generated for first-party Vue and React kits. Generated configuration should remain expanded so existing Projects snapshot their effective commands, inputs, and outputs.

The templ + htmx kit can use the same asset entry for its CSS and JavaScript bundle. Its existing `templ generate` step remains a separate Go source-generation stage.

Asset `inputs` and `ignore` values use slash-normalized glob syntax rooted at the asset path; `**` matches recursively. Environment entries support exact names and a trailing `*` prefix pattern. Invalid or ambiguous patterns fail configuration validation.

### Why `build.apps`

The selected App is already part of the `forj <app> build` command contract. App-scoped build configuration makes it possible to:

- prepare only the selected App;
- support multiple independently runnable Apps;
- support multiple assets per App; and
- let `forj dev` derive asset watchers without making production behavior depend on dev participation.

### Relationship to `dev.apps.<app>.spas`

The target model is:

```text
build.apps.<app>.assets
  -> production preparation contract
  -> conventional dev asset nodes when the App participates in dev

dev.apps.<app>
  -> development participation, App build/run overrides, asset watch overrides, and runtime behavior
```

Development configuration may override watch-only concerns, but it should not redefine the production build command.

When both a canonical asset and a legacy `dev.apps.<app>.spas.<name>.build` command exist, the commands must match after normalization. A mismatch is a configuration error with migration guidance; GoForj must not run one command and record freshness proof for the other.

### Embedding and consumption

The asset graph prepares outputs; application source determines how those outputs become part of the App.

First-party Vue, React, and templ + htmx Apps retain the conventional `cmd/<app>/frontend/dist` embed and SPA registration generated by GoForj. A custom asset entry does not automatically add a `go:embed`, route, or HTTP registration.

This separation keeps build orchestration independent from serving policy. A future multi-frontend generator may create multiple embed and route registrations, but that is not required to make the asset pipeline support multiple build outputs.

## Command Contract

### Default

```bash
forj build
```

Builds the default App and prepares its stale or missing assets.

```bash
forj admin build
```

Builds only the staff-facing `admin` App and its assets.

### Asset mode

Expose one advanced enum rather than several overlapping booleans:

```bash
forj build --assets=auto
forj build --assets=always
forj build --assets=skip
```

Semantics:

| Mode | Behavior |
| --- | --- |
| `auto` | Default. Prepare only missing or stale assets. |
| `always` | Rerun dependency preparation and rebuild every selected-App asset. |
| `skip` | Do not inspect or prepare assets; compile the current filesystem state. |

`skip` is an explicit escape hatch. Documentation should not make it the ordinary development or release path.

### Explicit Go package targets

The complete-artifact contract applies when `forj build` infers or receives the selected App package. If passthrough arguments explicitly target a different Go package, `auto` should skip App asset preparation and state that decision under timings or debug output.

`--assets=always` with a non-App package target should fail with an actionable message instead of claiming that the resulting package is a complete App artifact.

### Output

Routine terminal output should fit the existing build progress model:

```text
1/6 generate
2/6 templ
3/6 assets
4/6 wire
5/6 build:api-index
6/6 go build
```

When every asset is current, the asset stage should complete without launching a child process. Detailed cache decisions belong behind `--timings`, debug logging, or a future verbose flag:

```text
assets frontend: current (18ms)
```

## Build Ordering

The initial implementation should favor a clear correctness boundary:

```text
generate
  -> templ generate when required
  -> prepare selected-App assets
  -> Wire
  -> API index preparation
  -> go build
  -> publish API index
```

Publish each asset manifest after:

1. the asset command succeeds;
2. every declared output exists; and
3. the output validation completes.

Asset freshness and App compilation are separate successful boundaries. If Go compilation later fails, the prepared asset remains current and the next invocation may reuse it. API-index publication and binary replacement remain gated on successful Go compilation.

Parallel asset and Go preparation can be considered after profiling. It should not complicate the first implementation before the cached fast path is measured.

## Concurrency and Build Snapshots

GoForj must serialize asset writers across standalone builds and `forj dev`.

- An exclusive per-asset filesystem lock covers preparation, the asset command, post-command input validation, output validation, and manifest publication.
- Asset roots and output paths may not overlap across definitions, including definitions owned by different Apps.
- Inputs are fingerprinted before and after the asset command. If they change while the command runs, discard the result and retry once; a second change fails with an actionable "inputs changed during build" error.
- Before Go compilation, reacquire shared locks for every selected-App asset, validate manifests and output digests, and hold those locks through compilation.
- A competing GoForj process waits rather than rewriting an output being embedded.

External editors do not participate in advisory locks. The artifact corresponds to the last validated asset snapshot before Go compilation, which is the same snapshot model ordinary compilers use for source files changed during a build.

For asset-enabled builds, compile the App binary to a temporary sibling path. Revalidate output digests after compilation, then atomically rename the binary into place. The API-index candidate retains its existing post-compilation publication step. If output changed during compilation, discard the temporary binary and leave the previous published binary in place.

## Freshness Model

### Fingerprint inputs

The asset fingerprint should include:

- App name;
- asset name;
- normalized asset path;
- GoForj asset-manifest schema version;
- effective preparation and build commands;
- declared input matcher and ignore configuration;
- declared output paths;
- matched input file paths and content;
- declared environment variable names and effective values;
- package manager lockfiles; and
- relevant GoForj configuration affecting generated frontend environment.

The conventional first-party input contract is the complete asset root excluding dependency, output, VCS, GoForj cache, and command-owned intermediate paths. It intentionally includes images, fonts, public files, TypeScript configuration, PostCSS/Tailwind configuration, and future source formats without requiring an extension allowlist.

Generated ignores must include known compiler intermediates such as `**/*.tsbuildinfo`. A configured build command that writes elsewhere under its input tree must declare those paths in `ignore`; otherwise post-command input validation correctly treats the contract as unstable.

The fingerprint must not include:

- `node_modules`;
- declared output directories such as `dist`;
- unrelated process environment;
- timestamps as authoritative content; or
- secrets that are not declared build inputs.

Secrets included through declared environment patterns must be hashed into the digest and never written in plaintext to the manifest or logs.

### Fast path

The `auto` fast path succeeds when:

1. a successful manifest exists;
2. the effective contract fingerprint matches;
3. every declared output exists and matches its recorded output digest; and
4. no previous interrupted preparation is marked active.

The command should perform no package-manager or frontend-build process launch on this path.

The first implementation should read and hash declared input and output content on every freshness check. Path, size, or modification metadata may accelerate traversal but must not substitute for reading bytes, because same-size and restored-mtime replacements are valid filesystem states.

Any later digest-cache optimization needs a separate correctness proof and tests equivalent to Git's racy-clean handling before it can affect cache-hit decisions.

### Cache location

Store local manifests beneath:

```text
.goforj/cache/assets/<app>/<asset>.json
```

The directory is tool state, not Project source, and should be ignored by Git.

The manifest format must be versioned. Unknown or corrupt versions cause a cache miss, not a build failure.

### Output validation

Declared outputs must exist after a successful build. Directories must exist and contain at least one file.

The successful manifest should record a sorted content digest for each declared build output. A cache hit validates that digest so deleted, partially restored, or manually changed output cannot be mistaken for the result of the recorded build.

Output digest calculation follows the same byte-reading rule as input fingerprinting. Dependency preparation outputs such as `node_modules` require existence checks, not recursive content hashing.

## Dependency Preparation

Dependency preparation is a separate decision from asset freshness.

The preparation command runs when:

- a declared preparation output such as `node_modules` is missing;
- the preparation fingerprint does not match its declared inputs such as the lockfile and package metadata.

An ordinary frontend source change should run the build command without reinstalling dependencies.

`--assets=always` reruns both dependency preparation and the asset build. It is the explicit recovery path for a partial or corrupted dependency tree.

When an asset is stale and `auto` needs to run its build command, dependency freshness should also include the current operating system, architecture, and configured package-manager executable identity. Those checks occur only on the stale path; a complete asset cache hit must not launch Node or a package manager merely to ask for its version.

Generated npm-backed starter kits should use a lockfile-aware clean preparation command for new Projects. Existing Projects retain their visible configured dependency command during migration.

Custom assets may omit `prepare` when dependencies are managed externally.

The build must print the exact failing asset and phase:

```text
prepare asset app/frontend: prepare dependencies: npm ci exited with status 1
```

GoForj must not silently fall back from a failed clean install to a different package-manager command.

## `forj dev` Integration

`forj dev` should retain its native watcher graph:

```text
asset source changes
  -> prepare changed asset
  -> join all successful assets for the App
  -> one App build
  -> runtime replacement
```

The asset watcher and `forj build` should call the same `internal/appassets` service.

After a successful dev asset build, the service records the same manifest used by `forj build`. The following App build therefore takes the `auto` fast path and does not launch npm twice.

The running App remains last-known-good when an asset build fails. No App build or runtime replacement occurs for that failed wave.

Custom raw `dev.watches` remain independent. They do not become production build assets unless the Project adds an explicit `build.apps.<app>.assets` entry.

## Multi-App and Multi-Asset Behavior

- `forj build` prepares only the default App's assets.
- `forj <app> build` prepares only that additional App's assets.
- Multiple assets for one App may prepare concurrently when their paths and outputs do not overlap.
- The App compiles only after every required asset succeeds.
- Two asset definitions with overlapping roots or output paths fail configuration validation, including definitions owned by different Apps.
- An asset path must remain within the Project root unless a future explicit external-path policy is designed.
- Cache identity includes App and asset names even when two Apps use similar roots.

## Failure and Publication Semantics

### Asset command failure

- Stop the pipeline before Wire, API-index publication, and Go compilation.
- Do not advance the asset manifest.
- Preserve the previous compiled binary.
- Report the App, asset, command phase, working directory, and exit status.

### Missing output

A successful command that does not create every declared output is a build failure:

```text
prepare asset app/frontend: build succeeded but output dist is missing
```

### Interrupted build

An interrupted asset command does not publish a manifest. A later `auto` build treats that asset as stale.

### Go build failure after successful asset preparation

Retain the successful asset manifest so the next invocation does not repeat valid frontend work. Do not publish the API-index candidate or replace the previous binary.

## Configuration Migration

### New Projects

New npm-backed starter-kit Projects should generate:

- `build.apps.<app>.assets.frontend`;
- a `dev.apps.<app>` participation entry that derives its asset node from the build contract; and
- no duplicate generated frontend install pre-task.

### Existing generated Projects

A render migration may promote an existing SPA definition only when it exactly matches a known framework-generated shape:

- conventional App frontend path;
- known generated build command;
- known generated matcher and ignore lists; and
- known generated dependency installation task.

The migration adds the equivalent build asset, removes only the exact framework-owned install task, and leaves explicit development overrides intact.

For an exact generated SPA shape, migration also removes the duplicated dev build command after promoting it to the canonical asset. Dev retains only its matcher, ignore, debounce, and participation concerns.

### Customized or legacy Projects

Do not guess.

- Arbitrary `dev.watches` remain development-only.
- Customized structured SPA commands remain under `dev` until the user adds or accepts an explicit build asset.
- A canonical asset combined with a differently configured legacy dev build command is invalid until the owner chooses one command contract.
- `forj build` may warn when the selected App embeds a conventional `frontend/dist` but has no build asset contract. It must not invoke npm based on that inference alone.
- A future `forj config:migrate` or render plan may present the exact proposed asset entry for review.

## Performance Contract

Performance is part of the feature, not a later optimization.

The implementation should establish benchmarks for:

- one conventional frontend with no changes;
- two unchanged assets in one App;
- a large frontend input tree excluding `node_modules` and `dist`;
- one changed source file;
- a missing output directory; and
- a lockfile change.

Initial acceptance targets on a warm local filesystem:

- no Node or package-manager process on an `auto` cache hit;
- no dependency install for a source-only frontend change;
- unchanged-asset inspection adds no more than 100 ms at p95 for a representative first-party starter kit;
- a Go-only change does not rerun the frontend build command; and
- `forj dev` does not build the same asset twice in one publication wave.

If content hashing cannot meet the target, optimize traversal and I/O with a separately proven correctness-preserving technique before weakening the freshness contract.

The p95 target must run on a pinned CI runner with a committed benchmark fixture: a rendered first-party React frontend plus 5,000 synthetic input files totaling approximately 25 MiB, excluding dependency and output trees. The harness should perform one priming build followed by at least 50 freshness checks and report median and p95 separately. Developer-machine timings remain diagnostic rather than release gates.

## Package Boundaries

### `project`

Add configuration types for:

- build App selection;
- named assets;
- scalar and mapping forms;
- asset paths;
- preparation and build commands;
- preparation and build inputs, ignores, and outputs; and
- declared environment inputs.

Unknown fields must continue to round-trip through the existing compatibility model.

### `internal/appassets`

Own:

- config validation;
- input discovery;
- fingerprint calculation;
- manifest read, staging, and publication;
- dependency preparation;
- asset command execution;
- output validation; and
- structured results for build and dev UIs.

This package must not depend on `internal/forj`.

### `internal/build`

Own:

- selected-App resolution;
- asset mode parsing;
- placement in the build transaction;
- progress and timing output; and
- preservation of the existing API-index and binary publication boundary.

### `cmd/forj`

Own:

- recognition of `--assets` before the Go passthrough boundary is inserted; and
- parser-boundary coverage proving the flag reaches `build.Cmd` rather than `go build`.

### `internal/forj`

Own:

- rendering and conservative migration of config;
- `forj dev` watcher compilation;
- dev batching and runtime replacement; and
- mapping successful dev asset work into the shared manifest contract.

## Compatibility

### Source and API compatibility

The design does not require removing or renaming public Go APIs. New exported project configuration types require normal documentation and tests.

### Configuration compatibility

The new `build` field is additive.

Existing Projects without build assets retain current behavior unless a conservative renderer migration promotes an exact generated SPA shape. Unknown fields and custom development configuration remain preserved.

Because unknown top-level fields currently round-trip through `ProjectConfig.Extra`, decoding must inspect the raw `build` node before treating it as the new typed contract. Only a mapping with the recognized `apps` shape enters `BuildConfig`; an existing scalar or unrelated mapping remains preserved as an unknown field and disables the new asset contract until its owner migrates it.

### Runtime behavior

The resulting App runtime is unchanged except that a successful framework build embeds current declared assets.

### Operational behavior

On the first build after migration, a selected App may run an asset command or dependency installation that previous `forj build` versions did not run. Release notes must call this out.

`--assets=skip` provides an explicit temporary escape hatch for specialized pipelines.

### Persisted data

No application persisted-data format changes.

### Minimum Go version

No Go version increase is required by the design.

## Security Considerations

- Commands execute only from explicit configuration or known generated conventions.
- Asset roots must be validated within the Project root.
- Manifest paths must not permit traversal through App or asset names.
- Environment values contribute only through declared patterns.
- Manifest files must never contain plaintext secret values.
- Logs should show variable names, not secret contents.
- Output validation must not follow an unchecked path outside the Project.

## Implementation Phases

### Phase 1: Configuration and asset preparer

- Add additive build configuration types and round-trip tests.
- Implement validation, fingerprints, manifests, dependency state, command execution, and output checks.
- Add focused unit tests and benchmarks.

### Phase 2: `forj build`

- Add `--assets=auto|always|skip`.
- Insert the asset stage into the selected-App build transaction.
- Integrate progress, timings, failure output, and successful asset-manifest publication.
- Prove unchanged builds do not launch Node.

### Phase 3: `forj dev`

- Compile structured asset watchers from the shared build contract.
- Preserve multi-asset join and last-known-good runtime behavior.
- Record shared manifests after successful dev builds.
- Prove the following App build skips duplicate asset work.

### Phase 4: Rendering and migration

- Generate build assets for first-party npm-backed starter kits.
- Conservatively promote exact generated SPA shapes.
- Preserve customized and legacy watcher behavior.
- Update the largest supported generated composition.

### Phase 5: Documentation and release

- Replace manual frontend release sequences with `forj build`.
- Document asset modes and custom package-manager configuration.
- Add release notes for first-build operational behavior.
- Move this design to `docs/designs/completed` only after acceptance criteria pass.

## Testing Strategy

### Unit tests

- scalar and expanded asset configuration decode and round-trip;
- pre-existing scalar and unrelated-map top-level `build` values remain preserved;
- safe App, asset, root, input, and output validation;
- deterministic contract and content fingerprints;
- same-size input replacement with a preserved modification time changes the fingerprint;
- corrupt or unknown manifests become cache misses;
- missing and empty outputs fail;
- changed output content invalidates an otherwise matching manifest;
- secret environment values are not persisted or logged;
- dependency and source fingerprints invalidate independently.

### Build behavior

- clean structured frontend runs dependency setup, builds assets, and produces the App binary;
- second unchanged build launches neither dependency nor asset commands;
- Go-only change skips asset commands;
- frontend source change runs the asset build once;
- image, public asset, TypeScript config, `APP_ENV`, and App-prefixed frontend environment changes invalidate the asset;
- lockfile change runs dependency setup and the asset build;
- missing preparation output reruns dependency setup;
- `--assets=always` replaces a simulated partial dependency tree before rebuilding;
- missing `dist` rebuilds even when the input digest matches;
- modified `dist` content rebuilds even when the input digest matches;
- failed asset build prevents Go compilation and API-index publication;
- successful asset command with missing output fails;
- a source change during the asset command retries once and then fails if the input never stabilizes;
- command-owned intermediates such as `tsconfig.tsbuildinfo` do not trigger a retry or invalidate the next cache hit;
- concurrent standalone and dev preparation of one asset serializes through the same lock;
- a competing output mutation prevents temporary binary publication;
- `--assets=always` rebuilds;
- `--assets=skip` performs no asset inspection or command execution.

### App selection

- default build prepares only `app`;
- prefixed build prepares only the selected additional App;
- explicit passthrough package targets do not prepare a different App accidentally;
- multiple assets join before one Go build;
- overlapping outputs fail validation.

### CLI parsing

- `forj build --assets=skip` reaches `build.Cmd`;
- `--assets` does not cross the inserted Go passthrough boundary; and
- ordinary Go build flags and package arguments retain their existing parsing behavior.

### Dev integration

- initial dev preparation and App build share one successful asset manifest;
- steady-state frontend changes build once and publish one App build;
- failed asset work keeps the last working runtime;
- matching canonical and legacy dev commands migrate cleanly;
- conflicting canonical and dev build commands fail with migration guidance;
- custom raw watchers remain independent.

### Rendered integration

- Render every relevant module in a `/tmp` workspace.
- Validate Vue, React, templ + htmx, Web UI with no npm starter, and no-Web-UI Projects.
- Validate the largest supported multi-App, multi-SPA composition.
- Run applicable module tests with `GOCACHE=/tmp/gocache` and `GOMODCACHE=/tmp/gomodcache`.
- Run a published-module resolution pass with `GOWORK=off` where the rendered composition consumes released sibling modules.
- Regenerate checked-in mirrors and verify a second generator run produces no diff.

## Acceptance Criteria

1. A clean first-party frontend Project produces a current embedded binary with one `forj build`.
2. A second unchanged build does not launch Node or a package manager.
3. A Go-only change does not rebuild assets.
4. A frontend change cannot produce a successful binary with stale declared outputs.
5. Asset failure prevents Go compilation and preserves the previous binary.
6. Default and additional App builds prepare only their own assets.
7. Multiple assets owned by one App complete before one App build.
8. `forj dev` retains batching and does not duplicate successful asset work.
9. Existing custom and legacy watcher configurations are not silently promoted.
10. The unchanged-asset performance target is met by benchmarks.
11. Documentation can teach `forj build` as the complete release build command.

## Alternatives Considered

### Always run frontend builds from `forj build`

Rejected because Go-only changes would pay package-manager and bundler startup costs even when assets are unchanged.

### Keep the current manual sequence

Rejected because it permits successful binaries containing stale assets and makes every CI pipeline reconstruct framework dependency ordering.

### Add `forj package` and keep `forj build` Go-only

This is the fallback if a reliable fast path cannot meet the performance contract.

It gives the commands clean meanings:

```text
forj build    -> framework-aware Go compilation
forj package  -> prepare assets and produce a deployable artifact
```

The drawback is that users must learn which command is safe for release, while `forj build` continues to produce a binary that may contain stale embedded output. The incremental build design is preferable if its cache hit is cheap and reliable.

### Use `dist` timestamps

Rejected because timestamps do not prove that output corresponds to current source or build configuration.

### Infer npm from Web UI or `package.json`

Rejected because Web UI supports non-npm and custom frontend architectures, and filesystem inference would execute commands users did not configure.

### Use `dev.apps.<app>.spas` as the permanent build contract

Rejected because dev participation and production artifact ownership are different decisions. It remains useful as a conservative migration source for exact generated configurations.

## Risks

### The asset model becomes a generic task runner

Mitigation: require named App-owned inputs, declared outputs, and one build purpose. Do not add arbitrary dependency graphs unrelated to App artifacts.

### Fingerprinting costs more than expected

Mitigation: benchmark before rollout, exclude dependency/output trees, optimize traversal and I/O only with a separately proven correctness-preserving technique, and retain `forj package` as the fallback command model.

### Environment-dependent builds receive false cache hits

Mitigation: generated contracts declare relevant environment patterns, custom contracts expose the same field, and documentation treats undeclared environment dependencies as a configuration error.

### Automatic dependency installation surprises CI or offline users

Mitigation: run it only when dependency state requires it, show the command phase clearly, allow an omitted preparation command, and provide `--assets=skip` for externally prepared pipelines.

### Migration changes customized workflows

Mitigation: promote only byte-for-byte known generated shapes. Leave ambiguous configurations untouched and present an explicit migration suggestion.

## Related Designs

- [forj Dev Native Watcher Design](completed/forj-dev-native-watcher-design.md)
- [Starter Kits Design](completed/starter-kits-design.md)
- [React Starter Kit Design](completed/react-starter-kit-design.md)
- [templ + htmx Starter Kit Design](completed/templ-htmx-starter-kit-design.md)
- [App Composition Layout Design](completed/app-composition-layout-design.md)
- [Render Ownership And Upgrade Reconciliation Design](render-upgrade-reconciliation-design.md)

## Recommendation

Proceed with the incremental App asset stage only if Phase 1 proves the unchanged path meets the performance contract.

If it does, make `forj build` the complete App artifact command and share the same asset preparer with `forj dev`.

If it does not, do not hide an unconditional frontend build behind `forj build`. Introduce a distinct `forj package` command and preserve the current build latency explicitly.
