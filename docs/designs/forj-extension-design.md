# GoForj Extensions Design

## Status

- Draft architecture, not scheduled for a release.
- The compile-time extension model is recommended.
- The descriptor schema, public contracts, and generated file shape are not implementation-ready until the Phase 0 spikes pass.

## Summary

GoForj should support reusable extensions as ordinary Go packages compiled into a generated App. An extension is installed explicitly, resolved through the App's Go module graph, and connected to the App through generated, typed adapters.

The design has three durable boundaries:

1. `.goforj.yml` records which extension packages are installed and which Apps use them.
2. A static descriptor in the extension package declares identity, compatibility, requirements, and contributions. Synchronization validates that descriptor and records a canonical, checked-in lock.
3. GoForj generates per-App adapters into the framework's existing route, command, job, event, schedule, lifecycle, and primitive composition seams.

This is not runtime plugin loading. There is no `plugin.Open`, dynamic library, service locator, dependency scan, executable manifest, or universal runtime extension registry.

The App still owns policy. Wire still constructs dependencies. Existing managers still own instrumentation, readiness, inspection, and shutdown. Extension support adds a reusable package boundary without creating a second framework inside GoForj.

## Problem

Generated Apps already expose useful composition points, but those points are App-local:

- `app/routes.go`
- `app/commands.go` and the generated `RootCmd`
- App Wire providers for jobs and event subscribers
- `app/schedules.go`
- `app/lifecycle.go`
- generated cache, storage, queue, event, mail, and database managers

A package author cannot publish reusable code that imports a generated App's `internal/...` packages. Go's internal-package rules prevent it, every App has a different module path, and named Apps have different composition directories.

GoForj therefore needs a stable public package boundary plus generated adapters inside the consuming App. Those adapters must join the current composition paths rather than replace them.

## Goals

1. Install reusable Go packages into one or more generated Apps.
2. Keep construction compile-time and statically checked through Wire and the Go compiler.
3. Preserve App ownership of middleware, authorization, naming, resource selection, and operational policy.
4. Reuse current managers so extension behavior retains metrics, Lighthouse inspection, readiness, source metadata, and lifecycle behavior.
5. Keep installation, ordering, update, and removal explicit and reviewable.
6. Support default and named Apps without duplicating module dependencies.
7. Avoid extension-specific discovery during ordinary generation and remain offline-capable when the normal Go module cache or vendor inputs are complete.
8. Preserve App-owned files and user-managed environment values.

## Non-goals

1. Runtime extension discovery or dynamic loading.
2. Scanning every `go.mod` requirement for possible extensions.
3. Executing extension code to discover its manifest during `generate` or `build`.
4. A global IoC container or an `[]any` contribution registry.
5. Replacing first-party components with third-party extensions.
6. Silently enabling components, changing drivers, or increasing the App's Go version.
7. Arbitrary extension-provided templates, output paths, install scripts, or shell commands.
8. Extension-owned database migrations, frontend assets, or Lighthouse UI modules in the first release.
9. Solving the operator-facing resource links described by the separate [Resource Registry design](resource-registry-design.md).

## Current Framework Boundaries

The extension design must fit the framework that exists now:

| Surface | Current composition | Requirement for extensions |
| --- | --- | --- |
| App layout | The default App uses `app/` and `app/wire/`; named Apps use `app/<name>/` and `app/<name>/wire/`. | Installation and generated glue are per App. There is no new top-level `wire/` package. |
| Routes | App-owned `ProvideRoutes` returns `[]web.RouteGroup`; framework-owned Wire code registers those groups. | Use `github.com/goforj/web` types and preserve route listing, API Index, OpenAPI, security analysis, and final-path collision checks. |
| Commands | App commands are concrete Kong-tagged fields in a generated `RootCmd`. Preboot help exists before the App is constructed. | Generate concrete command fields. A runtime `[]any` command list is not compatible with the current command model. |
| Jobs | App Wire code registers handlers through the generated queue manager before App construction. Workers already depend on the manager rather than individual jobs. | Extend the existing registration provider and readiness token. Do not add another worker registry. |
| Events | App Wire code registers subscribers and returns `EventSubscribersReady`; generated managers own bus startup and shutdown. | Register through the observed manager and preserve the readiness dependency. |
| Schedules | The App-owned `ScheduleRegistry` registers against the generated scheduler wrapper. | Adapt a public contract to the current wrapper without exposing generated `internal` types or losing inspection behavior. |
| Lifecycle | The App owns six phases: before/startup/after startup and before/shutdown/after shutdown. Startup fails fast. Shutdown preserves phase order, reverses hooks within each phase, and joins errors. | Preserve all six phases and their ordering/error semantics. A two-method startup/shutdown abstraction is insufficient. |
| Primitives | Generated managers, env discovery, the resource catalog, and `ResourcePlan` collectively own named instances, drivers, dependencies, services, readiness, and instrumentation. | Bind extension requirements to that system. Do not create a parallel resource container. |
| Rendering | Render reconciles framework-owned files and preserves render-once App-owned files. | Extension files must have explicit ownership and follow the same replacement rules. |
| Build pipeline | The supported path is generation, optional Templ generation, `wire` in each App Wire directory, API indexing, then build or run. `forj generate` does not run Wire. | Add/update/remove must validate the full affected pipeline and must not document a nonexistent `wire generate` command. |

The current ownership lines are described further in [Generated App Extension Points](../context/generated-app-extension-points.md), [Runtime Architecture](../context/runtime-architecture.md), and the completed [App Composition Layout design](completed/app-composition-layout-design.md).

## Terminology

### Component

A first-party GoForj capability such as Web API, Jobs, Events, Scheduler, Cache, or Storage. Components determine which framework code and operational surfaces exist in each App.

### Extension package

The exact Go import path containing an extension's public integration surface and static descriptor. A package path is not necessarily its owning module path.

### Installation intent

The ordered `.goforj.yml` entry that records the extension package and its target Apps. It expresses App-owner intent; it does not duplicate the dependency version or the extension's own contract.

### Descriptor

Versioned, declarative metadata shipped beside the extension package. It describes the extension without compiling or executing it.

### Lock

The canonical result of resolving and validating installed descriptors against the App's module graph and GoForj capabilities. Normal generation consumes the lock without discovering or executing third-party tooling.

### Generated adapter

Framework-owned Go code inside a target App that imports the extension and connects typed contributions to an existing composition seam.

### App-owned hook

A render-once customization point for policy that cannot belong to a reusable package, such as authorization middleware or a primitive binding choice.

## Sources Of Truth

| Concern | Authority |
| --- | --- |
| Installed package and target Apps | Ordered entries in `.goforj.yml` |
| Resolved module version and checksum | `go.mod` and `go.sum` |
| Extension identity and declared contract | Static descriptor shipped with the extension package |
| Validated, normalized integration plan | Checked-in extension lock |
| App policy and binding choices | `.goforj.yml` and preserved App-owned hooks |
| Compiled integration | Framework-owned generated adapters and Wire output |

This separation is deliberate. `go.mod` cannot distinguish an extension from an ordinary dependency, and `.goforj.yml` should not copy the extension's resource or capability definitions.

## Design Invariants

1. Extensions are normal, trusted Go dependencies compiled into the App.
2. Installation is explicit; ordinary `go get` does not activate an extension.
3. Every contribution is scoped to a target App.
4. Generated integration remains typed. Reflection and `[]any` are not substitutes for a capability contract.
5. Extension packages never import a generated App's `internal/...` packages.
6. App-owned contributions run before extensions by default; extensions follow declared installation order. Shutdown keeps its phase order and reverses hook order within each phase.
7. Normal `generate`, `build`, and cross-build operations never execute a descriptor program or install hook.
8. Missing required policy or dependencies fail closed and return an actionable error.
9. Removal deletes only framework-owned generated state. It never deletes App-owned code, user env values, persisted data, or queued work.
10. Extension installation never silently widens components, supported drivers, or the minimum Go version.

## Installation Intent

The project needs an ordered, top-level extension list. The exact schema remains a Phase 0 decision, but its minimal shape should be equivalent to:

```yaml
extensions:
  - package: github.com/acme/goforj-billing/goforj
    apps:
      - app
      - worker
```

`package` is the exact import path of the extension integration package. GoForj resolves its owning module and version from the App's module graph. The version is not copied into YAML.

The list order is user policy and is preserved in the lock and generated code. Target validation must use the same project-layout and configured-App inventory used by current per-App component resolution, including pending Apps during render. Listing an App here assigns an extension; it does not independently create a runtime App.

An extension can target several Apps while remaining one module dependency. Removing it from one App retains the dependency and lock entry while another App still uses it.

The default target is the default `app` only. Named-App assignment must be explicit. Requirements are validated against each target App's component selection, not merely the project-wide component envelope.

The implementation must add this as typed project configuration at the authoritative config source and regenerate any checked-in mirrors or templates. Config migration must retain unknown fields through the existing `Extra` preservation behavior so older and newer GoForj versions do not erase one another's settings.

## Static Extension Descriptor

Each integration package ships one static descriptor, provisionally named `forj-extension.yaml`, in its package directory. For an existing installation, `extension:sync` uses `go list -json <package>` to resolve the package directory and owning module through the App's active module graph, including private modules and local `replace` directives.

The integration package must include that file in a `//go:embed` pattern, and synchronization verifies it appears in the package's `EmbedFiles` metadata. GoForj still reads the static file from disk; it does not access an exported variable or execute package code. The embed requirement ensures `go mod vendor` retains the descriptor beside the vendored package.

For a new `package@version`, `extension:add` must first apply `go get package@version` to staged module files. It then runs `go list -json <package>` without the version suffix inside that staged module graph. `go list` does not accept the `package@version` form for this purpose.

The schema below is illustrative rather than final:

```yaml
schema: goforj.extension/v1alpha1
id: acme.billing
package: github.com/acme/goforj-billing/goforj

requires:
  capabilities:
    - routes/v1
  components:
    - web_api

contributes:
  routes:
    provider: ProvideRoutes
    declarations:
      - method: GET
        path: /billing/invoices

integration:
  provider_set: ProviderSet

primitive_requirements:
  - slot: session_cache
    kind: cache
```

The descriptor may declare:

- a stable, globally collision-resistant extension ID
- its exact integration package
- descriptor schema version
- required host capability versions
- required first-party components
- minimum Go version as a constraint, never as an upgrade instruction
- contribution kinds and statically named exported integration symbols
- collision and inspection identities required by each contributed capability
- a statically named provider set or other construction entry point
- primitive requirement slots
- structured, non-secret environment metadata
- extension dependencies or conflicts if a later phase proves they are necessary

The descriptor must not contain:

- executable Go manifest functions
- arbitrary commands or install hooks
- source snippets or templates
- caller-selected output paths
- secret values or secret defaults
- machine-specific absolute paths
- host-dependent output

Unknown fields, unsupported schemas, duplicate identifiers, unsafe names, and inconsistent package identity fail synchronization. GoForj canonicalizes set-like fields and preserves only order that the schema defines as meaningful.

Every capability must make its collision-relevant identity available as data or constrain its Go declaration to a source form GoForj can analyze without executing it. A route provider that computes paths dynamically, for example, cannot support pre-runtime collision checks or a trustworthy API Index. Descriptor declarations and analyzed source must agree. Effective collisions introduced by App grouping or policy are checked again during the staged API Index and compile pipeline.

### Why the descriptor is data

Go module metadata cannot enumerate packages that export a particular Go function. Calling `Manifest()` would require GoForj to know an import path first, generate an inspector, compile it, and execute third-party package initialization. It would also make routine generation host-dependent and unsafe to run offline or while cross-compiling.

A static descriptor is inspectable before extension code is wired into the App. An extension may provide its own authoring helper to produce the file, but the GoForj host consumes only the checked-in data.

## Canonical Extension Lock

Synchronization writes a checked-in lock; a provisional path is `.goforj-extensions.lock.json`. Its exact name and schema remain Phase 0 decisions.

For each ordered extension it records at least:

- descriptor and lock schema versions
- extension ID and exact package import path
- owning module path, resolved version, and available checksum identity
- descriptor digest
- target Apps
- required and selected host capability versions
- normalized contributions and primitive requirements
- normalized non-secret env metadata

The lock must not record secrets or absolute local replacement paths. For a local replacement it records the logical module identity and descriptor digest. Ordinary Go source changes remain the compiler's concern; descriptor changes require synchronization.

`forj generate` uses `.goforj.yml` plus this lock as its integration plan. It does not scan module dependencies, execute extension programs, or silently refresh versions. It may resolve only the packages already named in the lock and hash their static descriptors to detect drift; it does not interpret a changed contract. If config, descriptor bytes, or the selected module version no longer agree with the lock, generation fails with an instruction to run `forj extension:sync`.

This gives checked-in generated Apps reproducible input and makes ordinary generation deterministic after dependency download. Offline operation still requires the same complete module cache or vendor state as the current generator and its `go mod tidy` work. The lock also makes review show both installation intent and the resolved integration plan.

## Generated App Composition

Extension composition is per App. Candidate framework-owned files are:

```text
app/
  extensions_gen.go
  wire/
    inject_extensions_gen.go

app/worker/
  extensions_gen.go
  wire/
    inject_extensions_gen.go
```

The exact split must be proven by the reference fixture, but the ownership rules are fixed:

- generated files have a clear generated header and may be replaced or removed
- existing App-owned `routes.go`, `commands.go`, `schedules.go`, `lifecycle.go`, and `inject_*_app.go` files are not rewritten during extension add/remove
- any new App policy hook is created once and then preserved
- framework-owned templates expose stable aggregation points even when no extensions are installed
- a second generation with unchanged inputs produces no diff

One generated Wire `extensionSet` may aggregate extension provider sets for construction. The descriptor must name that exported set, or declare a complete alternative constructor contract, and Phase 0 must validate its symbol and signatures with `go/types` and Wire. Naming only a route or job provider is not enough to construct its private controllers and services.

The aggregate is a compile-time catalog, not a universal runtime registry. Contributions then flow through capability-specific adapters.

The renderer must learn the same ownership model so a later `forj render` refreshes framework glue without overwriting App policy. Existing generated Apps need a migration that adds framework-owned aggregation points without replacing their render-once files.

## Public Contract Shape

GoForj should not publish a broad `github.com/goforj/extension` SDK before a reference extension proves the minimum contract.

The eventual public surface should be split by capability where that avoids coupling every extension to every sibling library. A small core package may own descriptor-related vocabulary, while contribution contracts use existing public packages such as:

- `github.com/goforj/web`
- `github.com/goforj/queue`
- `github.com/goforj/events`
- `github.com/goforj/scheduler/v2`
- `github.com/goforj/cache`
- `github.com/goforj/storage`

The contract package must not import the GoForj CLI or generated App internals. Generated bridges translate public types into the consuming App's managers and wrappers. The Go compiler and Wire are the final contract checks.

Capability contracts should be versioned independently, for example `routes/v1` or `jobs/v1`. This is more precise than treating every generated layout change as a breaking extension API change.

## Capability Adapters

### Routes

Extension routes use `web.Route`, `web.RouteGroup`, `web.Handler`, and `web.Middleware`. They join the same registration path as App-owned groups.

App policy remains explicit. If an extension requires an authorization or tenancy wrapper, a missing hook is an error; it must not silently become identity middleware. A descriptor must distinguish deliberately public routes from routes that require App policy.

Route support is not complete until extension routes are visible to:

- `route:list`
- the API Index
- OpenAPI output
- security requirement analysis
- duplicate detection using normalized method plus the final group prefix and path

The API Index currently starts from the App's active `ProvideRoutes` source and does not execute Wire providers. The route spike must either expose generated route composition as a statically understood source or teach the indexer about the generated extension catalog. Shipping routes that work at runtime but disappear from GoForj's API contract is not acceptable.

### Commands

Extension commands must become concrete Kong fields in the generated command tree. Do not introduce `AddCommands(...any)`.

The command spike must prove:

- root and nested help
- the selected GoForj help formatter
- colon-delimited command signatures
- preboot parsing and delegated App commands
- native framework command precedence
- Wire construction of command dependencies
- command collision diagnostics
- Lighthouse command discovery where applicable

Framework-owned `RootCmd` generation can consume the extension lock. App-owned `Commands` remains untouched.

The native `extension:*` commands must also be added to GoForj's reserved command catalog so an App or extension cannot capture them during delegated parsing.

### Jobs

Generated adapters register extension handlers through the existing queue manager. In the first version that means every generated queue, preserving the current source, metrics, inspection, and lifecycle instrumentation.

The contract must distinguish:

- semantic job type
- handler
- registration scope
- physical queue lane or primitive binding when named targeting is supported

The adapter or generated manager must reject duplicate job types before registering handlers. The current manager is not collision-reporting, so this is a prerequisite rather than an existing guarantee. The existing App job readiness token remains part of App construction.

The current manager registers a handler on every generated queue. The first job contract may preserve that scope. Named-queue binding requires a new instrumentation-preserving manager method such as `RegisterNamed`; calling the public queue directly would bypass the generated manager's private handler instrumentation.

No Worker refactor is required. Workers already depend on the generated queue manager rather than one field per job.

### Events

Generated adapters register subscribers through the existing observed event manager and retain `EventSubscribersReady`. Runtime code should depend on the public `events.API`, not a generated internal bus type.

The first event contract may bind subscribers to the manager's default bus. Named-bus selection requires the later per-App primitive binding model; it must not be guessed or fanned out implicitly.

Subscription ordering, partial-registration cleanup, startup, and shutdown must match current event-manager behavior. Extensions must not start or close shared buses independently.

### Schedules

An extension cannot import the generated internal scheduler wrapper. A small public registrar contract and an App-local adapter must preserve task names, inspection wrapping, metrics, and scheduler behavior.

Schedule collisions are detected before registration. Order is App-owned schedules first, followed by extensions in installation order unless a later explicit policy says otherwise.

### Lifecycle

The lifecycle adapter must represent all six current phases:

1. before startup
2. startup
3. after startup
4. before shutdown
5. shutdown
6. after shutdown

Startup remains phase-order, registration-order, and fail-fast. Shutdown preserves `before shutdown` -> `shutdown` -> `after shutdown`, reverses registration order within each phase, and joins errors.

The current lifecycle marks the App started only after every startup hook succeeds, and `Stop` performs no cleanup after a partial startup failure. If extension support requires partial-start rollback, that is a deliberate lifecycle enhancement for the whole App, not behavior this adapter already preserves. Phase 0 must choose and test the policy before exposing lifecycle contributions publicly.

Named lifecycle hooks and duplicate-name validation would be new framework behavior, not a description of the current runtime. They should be added only if the lifecycle spike proves their value.

## Primitive Requirements And Bindings

An extension declares a primitive requirement slot such as `session_cache`; it does not automatically own a resource named from its extension ID.

The App binds that slot, per target App, to an existing default or named primitive. The generated adapter resolves the binding through the current manager and passes only the public sibling-library type into extension constructors.

This distinction prevents an extension from creating duplicate infrastructure when the App already has an appropriate cache, queue, storage disk, event bus, mailer, or database connection.

Rules for the first resource phase:

1. Only binding to an existing supported primitive is allowed.
2. The required component must already be enabled for the target App.
3. A missing component or binding is an actionable install error; GoForj does not enable it silently.
4. Extension code receives public types such as cache, storage, queue, or events APIs, never generated manager types.
5. Cross-extension resource ownership is not supported in the first version.

Creating a new named primitive is a later feature. Today's resource catalog and `ResourcePlan` cover catalog-declared framework resources, while ordinary named primitives are also discovered from env. Extension-created instances must deliberately extend or reconcile those paths rather than pretending an arbitrary named-resource plan already exists. The result must cover:

- supported driver compilation
- dependency and Go-version constraints
- env contract generation and named-App overlays
- Compose or external service planning
- readiness and About output
- metrics and Lighthouse inspection
- update, remove, and persisted-data ownership

Generating an accessor and a few env keys is not sufficient resource provisioning.

## Environment Contract

Raw `map[string]string` defaults are too weak. Descriptor env metadata should align with the separate [Env Contract Generation design](env-contract-generation-design.md) and eventually express:

- key
- description
- required or optional
- sensitive or non-sensitive
- safe runtime default, when one exists
- test default, when one exists
- target App scope

Required secrets never have descriptor defaults.

Extension-owned base keys use a normalized prefix derived from the stable extension ID, for example `acme.billing` -> `ACME_BILLING_`. Reusing an existing framework or App key is a declared binding, not ownership of a second key. Synchronization rejects normalized base-key and named-App overlay collisions across the framework, App contracts, and other extensions.

Named Apps use the existing `<APP>_<BASE_KEY>` overlay convention. Environment loading applies that overlay to the base key before Wire constructs the extension, so extension code reads the same base key in every target App while each App can supply a different value.

The current generator reads `.env.example` and then overlays `.env`. Runtime loading also includes compiled defaults and overrides, environment-specific dotenv files, ambient process values, and named-App overlays. A descriptor default does not become a runtime default merely because the generator read it.

The first extension slice should treat env declarations as validation and documentation metadata. Extension runtime code owns its own safe defaults until GoForj deliberately integrates extension keys into generated runtime env defaults.

Publishing behavior must reuse GoForj's ownership-aware env reconciliation:

- safe contract entries may be added to `.env.example`
- writing to `.env` requires an explicit command or flag
- existing values are never overwritten
- secret placeholders are never generated as real values
- removal never deletes user `.env` values
- generated example entries are removed only when ownership can be proven and the user has not taken them over

The design must not claim a simple three-level precedence until the compiled runtime and per-App overlay behavior are implemented and tested.

## Ordering And Collision Policy

Default order is:

1. App-owned contributions
2. extensions in `.goforj.yml` order
3. declarations in descriptor order where order is semantically meaningful

Within each shutdown phase, hook order reverses the corresponding registration order. Shutdown phase order itself does not reverse. If extension dependencies are later supported, dependency order must be validated explicitly rather than guessed from alphabetical order.

Synchronization first rejects collisions that are present in descriptor data. The staged generated command tree, API Index, Wire graph, and compiler then validate the effective identities those static tools can observe before any project files are published:

- duplicate extension IDs or package assignments
- commands with the same final Kong command path, including native framework names
- routes with the same normalized method and fully prefixed path
- duplicate job types
- duplicate event subscription identities if the event contract defines them
- duplicate schedule names
- duplicate primitive slot names or unresolved bindings
- incompatible capability versions

Current App-owned job and schedule registration is executable Go, so not every identity is statically recoverable. Before those extension capabilities ship, their manager or adapter must reject duplicates with an error before service startup. Static analysis improves install diagnostics but must not be the only guard when the App can construct an identity dynamically.

App-owned policy wins. An extension may not shadow a native framework command or silently replace an App contribution.

## CLI And Transaction Model

Commands should follow GoForj's current colon-delimited convention:

```text
forj extension:add github.com/acme/goforj-billing/goforj@v1.2.3 --app app
forj extension:update acme.billing --version v1.3.0
forj extension:remove acme.billing --app app
forj extension:sync
forj extension:list
forj extension:doctor
```

Exact argument names are a tooling decision, but package identity, extension ID, and module version must not be conflated.

These are native framework commands, not generated App commands. Management must remain available when an extension has made the App impossible to compile, especially for `extension:doctor` and `extension:remove`.

The explicit `--app` is intentional even though runtime App commands normally use `forj <app> command`. Extension management mutates project-wide module and lock state, may target several Apps in one transaction, and must not require a named App binary to compile before it can be repaired or removed.

### Add

`extension:add` performs one transaction:

1. Validate the requested package, version, and target Apps.
2. Create a transaction workspace with equivalent directory topology so relative `replace` directives still resolve.
3. Run `go get package@version` against the staged module files, never the live files.
4. Run `go list -json package` without `@version` in that staged graph to resolve the package directory and owning module.
5. Read the static descriptor without executing package code.
6. Show the resolved package, module, version, checksum identity, target Apps, requirements, dependency diff, and planned files.
7. Validate schema, host capabilities, App components, Go version, primitive bindings, and statically knowable collisions.
8. Stage `.goforj.yml`, the canonical lock, generated adapters, and owned env-example changes beside the staged `go.mod` and `go.sum`.
9. Generate imports before `go mod tidy` so the dependency is not removed as unused.
10. Run formatting and the supported generation -> `wire` -> API Index -> compile pipeline for every affected App.
11. Publish with a same-filesystem journal and per-file atomic replacements. Roll back ordinary failures; retain enough journal state for `extension:doctor` to recover an interrupted publication.

The command requires an explicit version in non-interactive use and never resolves `latest` silently. It must not silently change the App's Go directive or enable a component. It reports the exact unmet constraint and lets the owner decide.

### Sync and direct Go module changes

The preferred flow is `extension:add package@version`. A user who changes the module graph directly must run `forj extension:sync` explicitly.

Plain `go get` followed by `forj generate` does not discover a new extension. Conversely, updating an installed extension with `go get` makes the lock stale and ordinary generation stops with a sync instruction rather than silently accepting a new contract.

Only explicit `extension:add`, `extension:update`, and `extension:sync` operations interpret a new descriptor contract. Sync displays the old/new plan and rewrites the canonical lock and generated adapters transactionally. `generate` may hash the already-known descriptor bytes only to reject drift.

### Remove

Removal first detaches the extension from the selected Apps. It then removes only its lock records and framework-owned generated contributions. `go mod tidy` removes the dependency only when no target App or other import still uses it.

Removal preserves:

- App-owned policy hooks
- `.env` values
- user-owned `.env.example` edits
- database or storage data
- queued jobs and external topics
- resources still used by the App or another extension

Preserved framework hook files must depend only on host-owned contracts, not extension package types, so removing a broken dependency does not leave the App uncompilable. If ordinary App code imports the extension directly, removal reports those remaining imports and requires the owner to resolve them rather than deleting code.

Preserving external state does not make removal operationally safe. The plan must identify registered job types, subscriptions, and bound stateful resources, warn when queued payloads may be stranded, and leave drain, schema, or data migration to the owner. V1 provides no uninstall migration hook.

If an extension package no longer resolves, its config and lock still contain enough ownership information to remove framework-generated state safely.

## Compatibility And Trust

Compatibility has separate dimensions:

- descriptor schema
- capability contract version
- GoForj generated-layout capability
- required first-party components
- sibling-library module versions
- minimum Go version
- runtime configuration and persisted-data behavior

Go modules remain authoritative for library and Go dependency constraints. The descriptor adds GoForj-specific capability requirements. A new module major version is not automatically a breaking extension change; synchronization must identify the concrete incompatible contract.

An extension is trusted application code. Static descriptor inspection avoids executing it during discovery, but installing it still means the App will compile and eventually run that code. GoForj does not claim to sandbox extension source.

V1 must not support install scripts, post-update hooks, arbitrary generators, runtime downloads, or dynamic libraries. Private-module, proxy, checksum database, and `GOPRIVATE` behavior should follow the user's normal Go toolchain configuration.

Local `replace` directives are supported during development. The lock must avoid absolute machine paths, and published-module validation must also run with `GOWORK=off` so a local workspace does not hide missing releases or imports.

## Rollout

### Phase 0: prove the boundaries

Build one in-repo, multi-module fixture extension and integrate it manually through the current App seams. The spike must decide:

- exact intent and static descriptor schemas
- lock path and canonicalization
- generated versus App-owned file split
- default and named-App targeting
- a minimal public contract package shape
- Kong command field generation and preboot help
- API Index/OpenAPI visibility for external routes
- primitive requirement binding without new resource provisioning
- update/remove recovery when the dependency is broken or absent

Do not publish the public SDK or promise a release until these decisions work together.

### Phase 1: synchronization and one reference capability

Implement explicit install intent, static descriptor resolution, the canonical lock, `extension:add`, `extension:update`, `extension:remove`, `extension:sync`, `extension:list`, `extension:doctor`, transactional rollback, and stable per-App no-op generated seams. Removal ships with installation; it is not deferred until after extensions can make an App unbuildable.

Ship one dependency-free reference capability end to end. Routes are a strong candidate because `github.com/goforj/web` already owns public types, but they ship only after API Index and OpenAPI parity is proven.

### Phase 2: commands, jobs, and events

Add commands through concrete Kong fields. Add jobs and subscribers through the existing manager registration and readiness seams. The first job version preserves registration on every generated queue; the first event version uses the default bus. Named queue or bus targeting waits for primitive binding support. Do not reshape Worker or event-bus lifecycle.

### Phase 3: schedules and lifecycle

Introduce the smallest public registrar adapters that preserve schedule inspection and the complete six-phase lifecycle contract. Decide partial-start cleanup as an explicit runtime enhancement; preserve shutdown phase order and reverse hooks only within each phase.

### Phase 4: existing primitive bindings

Allow extension requirement slots to bind to existing named primitives per App. Keep component gating and operational instrumentation intact.

### Phase 5: separately designed surfaces

Consider extension-created primitive provisioning, database migrations, frontend assets, Lighthouse UI modules, and operator resource links only through their respective ownership and lifecycle designs.

## Acceptance Criteria

### Determinism and ownership

- zero installed extensions adds no runtime behavior or extension dependencies
- a second sync, render, and generate produces no diff
- add, update, and remove touch only the declared transaction files
- removal deletes only framework-owned generated files or state and owned `.env.example` entries; it never rewrites App-owned Go files
- failed add/update/remove restores `.goforj.yml`, module files, lock, generated files, and env-example changes
- removal works when the dependency no longer compiles or resolves
- App-owned files and user env values survive every transition

### Module and App coverage

- default App and multiple named Apps can select different extensions
- removing one App target retains shared module state for other Apps
- local multi-module fixtures with relative `replace` directives work
- a published fixture validates with `GOWORK=off`
- private-module resolution follows normal Go settings
- no operation silently raises the Go directive or enables a component

### Contribution parity

- extension routes serve and appear in route listing, API Index, OpenAPI, and security analysis
- extension commands parse, execute, and appear correctly in every supported help mode
- native and App command collisions fail before staged project files are published
- jobs and subscribers retain metrics, inspection, source metadata, readiness, and shutdown behavior
- schedules retain names and inspection behavior
- lifecycle hooks preserve six-phase ordering, fail-fast startup, per-phase reverse shutdown, and joined shutdown errors
- any newly adopted partial-start cleanup policy is tested as an explicit runtime enhancement
- primitive bindings use existing managers and reject disabled components or unsupported names

### Tooling quality

- malformed, unknown-version, duplicate, and oversized descriptors fail clearly
- config and lock ordering is deterministic across platforms
- stale config/module/descriptor/lock combinations produce actionable sync diagnostics
- generated code is formatted and Wire-valid for every affected App
- the largest supported generated composition builds and tests from a `/tmp` render
- every relevant nested Go module is tested independently

## Open Decisions

The following are deliberate gates, not details to fill in while implementing unrelated work:

1. Final `.goforj.yml`, descriptor, and lock schemas and migration behavior.
2. Whether capability contracts live in one small module or capability-specific packages.
3. How generated extension route sources participate in static API indexing.
4. How concrete extension commands enter `RootCmd` without regressing preboot help or delegated command behavior.
5. The smallest preserved App-policy hook surface, especially for required route middleware.
6. Per-App primitive binding syntax and the adapter types for each sibling library.
7. Whether extension-to-extension dependencies are needed in V1.
8. How partial startup cleanup is represented across event and lifecycle contributions.

## Recommendation

Proceed with Phase 0 and one real reference extension before creating a broad public SDK.

The right GoForj model is explicit installation, static discovery data, a checked-in resolved plan, and generated per-capability adapters into existing App-owned seams. That keeps extensions understandable as normal Go code while preserving the framework features users already rely on.
