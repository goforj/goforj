# App Primitive Component Gating Plan

## Status

- Plan status: complete
- Planning date: 2026-07-14
- Phase 0 status: complete; the render-contract inventory, absence assertions,
  and focused render profiles are in place
- Phase 1 status: complete; component contract, legacy migration, App/project
  projections, and primitive-neutral Inspects observations are merged on this
  branch
- Phase 2 status: complete; Events has a truthful initial opt-out, mixed-App
  projection, safe additive enablement, and an explicit no-removal contract
- Phase 3 status: complete; File Storage now has a truthful initial opt-out,
  mixed-App projection, safe additive enablement, and an explicit no-removal
  contract that never treats runtime file data as generated residue
- Phase 4 status: complete; Background Jobs now owns the complete Queue
  surface, including mixed-App projection, environment, generation,
  observability, dependencies, and safe transition preflight
- Phase 5 status: complete; Inspects uses private bounded storage and Cache has
  a truthful opt-out, dependency closure, generated ownership markers, and
  transition preflight
- Phase 6 status: complete; resource planning, environment, generation,
  dependencies, runtime reporting, and service reconciliation honor effective
  component selections
- Phase 7 status: complete; the normal wizard exposes the four default-selected
  components without adding a resource, driver, topology, or mode stage
- Acceptance status: complete; unit, vet, race, integration, multi-App,
  Lighthouse, and all six sharded PR render jobs pass
- Scope: component modeling, project and App rendering, generated resource
  surfaces, configuration migration, and test-render coverage
- Target repository: `goforj`
- Related design: [`forj new` Components and Resource Defaults Design](forj-new-resource-topology-wizard-design.md)

This plan turns Cache, Events, Storage, and Background Jobs into truthful App
components without adding another resource or driver stage to `forj new`.

The implementation was organized as vertical render slices. Events, Storage,
and Background Jobs completed their disabled contracts before wizard exposure.
Cache was exposed before its full disabled contract landed; the later Cache and
cross-cutting slices brought the current tree to the same truthful contract.
The status above describes the current implementation and does not claim a
different commit chronology.

## Current Completion Evidence

- `project/components_yaml_test.go` proves legacy mapping migration, modern
  sequence opt-outs, canonical ordering, long-sequence formatting, extension
  preservation, and removal of the obsolete compatibility marker.
- `internal/forj/new_project_cmd_test.go` proves concrete database choices and
  all four default-selected primitives share the existing Components stage,
  while resource topology stays out of the wizard and project YAML.
- `internal/forj/primitive_template_projection_test.go` and
  `internal/forj/project_renderer_test.go` cover enabled and disabled generated
  surfaces and dependencies.
- `internal/forj/primitive_renderer_preflight_test.go` proves unsafe removal is
  rejected before configuration, environment, or owner files are written.
- `internal/forj/primitive_renderer_transition_test.go` proves additive
  enablement and App-owned-file preservation for Cache, Events, Storage, and
  Background Jobs, including last-owner removal refusal.
- `internal/forj/cache_cleanup_ownership_test.go` proves Cache cleanup accepts
  only explicitly marked generated artifacts and rejects owner source or
  environment dependencies before mutation.
- `internal/forj/resource_switching_docs_test.go` keeps generated component
  documentation explicit about active and included drivers.
- `internal/rendercheck` validates the high-signal real-render contract: package
  and support files, App accessors, exact named-App driver keys, dashboards,
  runtime markers, documentation, and direct module boundaries. Focused
  template, resource, readiness, About, metrics, and Compose tests cover the
  remaining detailed projections without duplicating every assertion in each
  real-render sentinel.
- The six-shard PR profile renders, wires, builds, and tests all 77 curated and
  pairwise combinations under `/tmp`. Its all-on, web-without-primitives, and
  historical-mapping sentinels also rerun `forj render` and bare
  `forj generate`, preserve representative App-owned edits byte-for-byte, and
  require canonical marker-free configuration.
- The full Go suite, vet, watcher race/stress lanes, multi-App smoke tests,
  Lighthouse integration suite, and consolidated framework plus rendered
  SQLite/MySQL/Postgres integration suite pass on the completed branch.

## Goal

Users can decide whether an App contains Cache, Events, Storage, or Background
Jobs from the existing Components experience. The common set remains selected
by default, so the normal creation path requires no new decisions.

When a component is disabled, its generated API, wiring, configuration,
commands, operational surfaces, infrastructure requirements, and Go
dependencies are absent. Driver selection remains an environment and build
contract concern rather than a component choice.

## Decision Summary

Add three new component keys and retain the existing Jobs key:

| Persisted key | Wizard label | Default | Meaning |
| --- | --- | --- | --- |
| `cache` | Cache | selected | Cache temporary and computed values |
| `events` | Events | selected | Publish and subscribe to application events |
| `storage` | File Storage | selected | Store private and public files or objects |
| `jobs` | Background Jobs | selected | Send and process work outside requests |

Queue is not a separate component. It is the driver-backed machinery behind
Background Jobs. Disabling Jobs removes queue managers, dispatch APIs, worker
runtimes, queue commands, queue environment, and queue dependencies together.

The plan does not introduce:

- a Queue component;
- a Job Workers component;
- a normal resource or driver wizard;
- driver-specific components such as Redis Cache;
- an App-wide standalone, shared, portable, or runtime mode;
- a broader Queue API redesign beyond correcting exposed dispatch and handler
  registration behavior;
- changes to the existing concrete database choices in Components.

The governing model is:

```text
Components     decide which capabilities an App contains
Resource plan  decides active and included drivers for those capabilities
Service plan   derives infrastructure from the resulting driver contract
Runtime choice decides which supported runtimes a deployment starts
```

An API process that dispatches jobs and a worker process that consumes them
both participate in Background Jobs. Running `api` in one deployment and
`worker` in another is runtime topology, not a reason to expose Queue and Job
Workers as separate components.

## User Experience

The normal wizard keeps one Components stage. Cache, Events, File Storage, and
Background Jobs are selected in the recommended defaults. Users who do not
customize components do not encounter another question.

Database remains visible in the same stage as a mutually exclusive concrete
choice. Driver defaults for the new component-gated resources remain implicit:

| Component | Active driver | Included drivers |
| --- | --- | --- |
| Cache | `memory` | `memory,redis` |
| Events | `inproc` | `inproc,redis` |
| File Storage | `local` | `local` |
| Background Jobs | `workerpool` | `workerpool,redis` |

Each current wizard deselection maps to a tested disabled render contract. The
driver defaults remain operational details rather than additional creation-time
questions.

### Changing components later

To enable a component later, add it to the default App's `render.components`
sequence or a named App's `apps.<name>.components` sequence, then run
`forj render`. GoForj resolves required component dependencies and creates the
missing framework-owned support without replacing render-once App-owned
registration files.

Removing a component from an existing project is deliberately conservative.
Changing the sequence is not an uninstall command: GoForj removes only artifacts
it can verify are framework-owned, never arbitrary source files, storage data,
queued work, cache state, or event history. If generated or user-owned residue
makes the transition unsafe, `forj render` refuses before writing and identifies
the path that must be moved or reconciled. Driver switching remains a separate
environment concern and does not require changing the component selection.

## Component Dependencies

Component choices describe useful capabilities, not every technically possible
permutation. Dependencies are resolved automatically and surfaced to users so
the wizard can offer fewer knobs without hiding why a component is present.

The new dependency rules are:

- Demo App requires Cache, Events, File Storage, and Background Jobs because
  its generated features use all four.
- Auth requires Web API, Mail, a database, and Cache. OAuth inherits those
  requirements through Auth.
- Metrics records only the enabled primitive families.
- Observability and Grafana project only the enabled metrics and dashboards.
- Web API, Web UI, Scheduler, CLI, and Database do not inherently require
  Cache, Events, Storage, or Background Jobs.
- Background Jobs benchmarks may exercise Cache or Storage when present, but
  benchmark tooling must not turn those optional capabilities into Jobs
  dependencies.
- Users may deselect any primitive that has no selected dependents. Dependency
  normalization keeps required primitives visibly selected in the same flat
  component checklist instead of hiding them behind another wizard stage;
  existing-project reconciliation may still refuse a destructive removal when
  component-owned or user-owned residue remains.

Dependency resolution must be identical in project creation, `make:app`, YAML
loading, generated project configuration, and tests.

## Truthful Disabled Contract

For an initial render, a disabled primitive means all of the following:

| Surface | Disabled behavior |
| --- | --- |
| Resource plan | No root selection and no generated named selections |
| Generated packages | No manager, accessor, driver manifest, README, or integration fixture |
| App API | No manager field or public accessor on `App` |
| Wire | No imports, providers, constructor parameters, sets, or lifecycle hooks |
| Commands | No component-specific shell, test, make, worker, or resource entry |
| Environment | No active driver, included-driver manifest, named-resource, metric, or connection keys |
| Runtime | No discovery entry, About section, readiness check, or lifecycle participation |
| Lighthouse | No advertised capability, registered command, server handler, or benchmark for that primitive |
| Metrics | No imports, collectors, observers, event methods, or metric toggles |
| Grafana | No dashboard or seed entry for that primitive |
| Services | No Compose profile or external requirement caused by its drivers |
| Dependencies | No primitive or driver module retained solely by generated code |
| Generators | `forj generate`, build, and rerender do not recreate the disabled package |
| Documentation | No generated documentation claims that the capability exists |

In a multi-App project, a capability may still exist in the shared module
because another App uses it. Per-App disabled means the selected App binary has
none of that capability's managers, accessors, wiring, checks, or runtimes.

Stale owner environment keys must never make a disabled component reappear in
runtime discovery or service planning.

The shared embedded Lighthouse UI may retain dormant client code for optional
primitive views. A disabled component must not be advertised by the runtime or
receive registered commands and handlers. Building a different frontend bundle
for every component combination is outside this plan.

## Project and Per-App Semantics

The renderer needs to distinguish two projections:

1. the components selected for one App binary;
2. the project capability envelope required to render shared packages and
   included drivers for all Apps.

The project envelope is derived from the union of the default App selection and
all named-App selections. It is not itself an additional wizard choice.

The current promotion path mutates `render.components` when a named App adds a
capability. The default App also reads `render.components`, so promotion can
silently add that capability to the default binary. Primitive component gating
must not extend that leakage.

The target contract is:

- `render.components` retains the default App selection and project-only
  tooling choices;
- `apps.<name>.components` retains each named App selection;
- the renderer derives `ProjectComponents` as their union;
- `.Components` passed to an App template is that App's selection;
- `.ProjectComponents` passed to shared templates is the derived envelope;
- adding a named App can widen shared generated support without changing the
  default App's composition.

Legacy configuration cannot always recover the original default-App intent.
The current promotion flow may already have copied a named App capability into
`render.components`, and no provenance was stored. Migration preserves those
promoted capabilities on the default App because that is the current rendered
behavior. Future named-App additions use the derived envelope and no longer
widen the default App implicitly.

Database support and App-prefixed resource environment must follow the same
derived-envelope model. Effective service consumers must use each App's actual
component selection rather than treating every App as a consumer of every
project capability.

## Configuration Compatibility

Canonical component YAML stores enabled names as a sequence. That raw shape is
also the migration discriminator: historical boolean mappings predate optional
`cache`, `events`, and `storage`, while omission from a modern sequence is an
intentional deselection. The obsolete `component_contract` marker remains
readable for compatibility but is not written into canonical configs.

Migration from the existing contract must:

- enable Cache, Events, and Storage for the default App;
- enable them for existing named Apps because those APIs were previously wired
  into every App;
- preserve the existing Jobs selection, which continues to own Queue;
- preserve all existing driver, named-resource, and App-prefixed environment
  values;
- persist the migrated selection as a component sequence without the obsolete
  marker before absence gains the new disabled meaning;
- update both GoForj's project configuration types and the configuration parser
  rendered into generated projects.

The migration must be idempotent and covered with both legacy mapping-shaped
and canonical sequence-shaped YAML fixtures.

## Render Ownership and Later Removal

Initial omission is safe because files are never created. Additive enablement
is safe only after primitive command and provider registrations no longer
depend on rewriting render-once or App-owned files such as `app/commands.go`.
Each vertical slice must move those registrations into a framework-owned
component boundary or explicitly defer additive enablement for that component.

Removal from an existing project is different because component directories
may contain user-authored events, subscribers, caches, disks, queues, and jobs.

This work must preserve these rules:

- framework-owned generated files may be reconciled only through an explicit
  ownership manifest or another verifiable framework-owned boundary;
- render-once and App-owned files are never deleted or wholesale rewritten;
- directories containing user-authored files are never recursively deleted;
- component-specific registration should live in framework-owned injectors
  where possible, reducing edits to owner files;
- a supported additive enable must create framework-owned registrations without
  rewriting owner files;
- disabling an existing component must fail with an actionable diagnostic when
  user-owned code still depends on it;
- storage data, queued work, cache state, and event history are never deleted as
  part of source reconciliation.

Automatic deletion of arbitrary user code is not an acceptance criterion. If
ownership-aware removal does not fit the initial implementation, the supported
first contract is truthful new-project omission and additive enablement. The
CLI and documentation must not claim that deleting a YAML key safely uninstalls
a component.

## Architectural Prerequisites

### Inspects must not import optional primitives

The generated Inspects manager currently imports Cache, Storage, and Queue event
types directly. A Storage or Jobs slice cannot prove dependency absence while
the shared diagnostic package retains those imports.

Move primitive event conversion to optional observer adapters before the first
component gate. Inspects accepts primitive-neutral observation values defined
inside its own package. This foundation change preserves current behavior while
allowing each later slice to remove its primitive package completely.

### Inspects persistence must not require App Cache

The generated Inspects manager currently stores diagnostic state in a named App
cache. That makes Cache mandatory even when the public cache capability is
hidden.

Move Inspects to its own internal store contract with a bounded in-memory
default. Inspection storage is framework diagnostic state, not application
cache state. A future distributed inspection store may have its own explicit
contract without forcing the App Cache component.

Remove or migrate `CACHE_INSPECTS_DRIVER` once inspection persistence is no
longer an App cache. Queue history is owned by the Queue implementation and read
through `queue.Queue.History`; the duplicate Lighthouse cache timeline and its
`CACHE_LIGHTHOUSE_DRIVER` setting have been removed. Cache browsing and cache
benchmarks remain conditional on the App Cache component.

### Background Jobs owns Queue completely

The existing Jobs component already controls most queue generation, but queue
surface remains in shared renderer groups and templates. Move all remaining
queue-owned surface beneath Jobs:

- queue manager and driver generation;
- `App.Queue()` and `App.Queues()`;
- `QUEUE_*` root, named, and App-prefixed environment;
- `make:queue`, `make:job`, and worker commands;
- readiness, metrics, observation, About, discovery, and dashboards;
- queue Lighthouse and benchmark functionality;
- queue and queue-driver dependencies.

The slice also corrects the exposed Jobs contract needed for enabled renders to
be truthful: logical named queues route through their generated runtime and
physical queue name, and every generated job handler is registered on every
queue where it may be consumed. This is correctness repair, not a new Queue
abstraction or component.

## Implementation Strategy

The work proceeds vertically. A slice is complete only when its enabled and
disabled renders compile and its absence assertions pass. Do not add all flags
first and leave partially truthful combinations on the branch.

### Phase 0: Baseline and render-contract inventory

- Record the framework-owned files, render-once files, generated directories,
  environment sections, commands, modules, and dashboards owned by each
  primitive.
- Add reusable test helpers for asserting generated file presence and absence,
  environment keys, Go modules, commands, and Compose services.
- Capture a default-render baseline before component gates change output.
- Add explicit coverage accounting for every new component in both enabled and
  disabled states.

Exit criteria:

- the current default render remains green;
- the test harness can prove both presence and absence;
- every affected surface has an owning component or a documented shared
  reason.

### Phase 1: Component model, migration, and App projections

- Add Cache, Events, and Storage keys to the canonical catalog, configuration
  structs, YAML ordering, generated project configuration, CLI parsing, and App
  component allowlists.
- Change the Jobs label and description to Background Jobs without changing
  its persisted key.
- Mark all four primitive components selected by default.
- Encode Demo App dependency closure.
- Add the component-contract migration discriminator and legacy migration.
- Derive project capability envelopes independently from per-App selections.
- Pass App and project projections consistently to renderer and generator
  inputs.
- Refactor Inspects observation inputs so its core package no longer imports
  optional Cache, Events, Storage, or Queue types.
- Keep resource applicability and generated output on the compatibility path
  until each component's complete vertical slice flips its own behavior.

Exit criteria:

- old projects normalize without losing any previously available primitive;
- a new config round trips explicit component intent without prematurely
  exposing an unsupported disabled render;
- named-App capability support does not silently alter the default App;
- the default generated capability surface remains unchanged.

The new wizard rows remain hidden in this phase.

### Phase 2: Events vertical slice

- Split Events files, raw generator templates, make commands, subscriber
  wiring, and integration fixtures out of Core Components Rendering.
- Gate App bus fields and accessors, event manager providers, lifecycle hooks,
  subscriber sets, and event generators.
- Gate event root and named environment, readiness, discovery, About, test
  pipeline, metrics, observers, dashboards, and documentation.
- Make Events resource applicability and default planning follow the Events
  component in the same change as its template gates.
- Move Demo-only topics and subscribers under the Demo dependency boundary.
- Pass the Events component to full render, build, and `forj generate` paths.
- Ensure stale `EVENTS_*` values are ignored when Events is disabled.

Exit criteria:

- representative Events-enabled and Events-disabled projects compile and test;
- disabled output contains no event package, App API, environment, commands,
  dashboard, Redis consumer, or event module;
- enabled defaults remain `inproc` with `inproc,redis` included.

### Phase 3: Storage vertical slice

- Split Storage documentation, observer, generated manager, and driver
  generation into a Storage render group.
- Gate App storage fields and accessors, providers, warnings, HTTP readiness,
  runtime discovery, About, resource description, metrics, dashboards, and
  environment.
- Conditionally gate the HTTP server's Storage field, provider input, and
  Lighthouse storage-download callback.
- Make Lighthouse storage browsing and storage benchmark suites conditional.
- Make Storage resource applicability and default planning follow the Storage
  component in the same change as its template gates.
- Encode Demo App's Storage dependency and keep favicon behavior unchanged
  when Storage is enabled.
- Make build and `forj generate` respect Storage absence rather than recreating
  `internal/storages`.

Exit criteria:

- representative Storage-enabled and Storage-disabled projects compile and
  test;
- disabled output has no storage package, App API, environment, dashboard,
  service consumer, or storage module;
- enabled defaults remain local and generated Demo storage behavior is
  unchanged.

### Phase 4: Background Jobs and Queue closure

Queue remains implementation machinery owned wholly by the existing Jobs
component. This phase closes the remaining Queue surface without introducing a
second component or coupling component selection to whether a deployment runs
an `api`, a `worker`, or both.

Implement the closure in this order:

1. Add a Jobs transition preflight before changing generated output. It must
   recognize project-owned and per-App Jobs residue, reject unsafe removal
   before configuration or owner files are written, and protect legacy
   App-owned Jobs injectors from cleanup. User-authored files under
   `internal/jobs` and queued runtime data are never deletion candidates.
2. Move queue observers, queue dashboards, generated managers and accessors,
   make commands and raw templates, worker support, and Jobs integration files
   beneath the Jobs render boundary. Shared Jobs files follow
   `ProjectComponents.Jobs`; App APIs and framework injectors follow that
   App's `Components.Jobs`.
3. Make Queue environment and dependency output truthful. Root driver and
   supported-driver keys exist only when the project envelope contains Jobs;
   App-prefixed active-driver keys exist only for participating Apps. Queue and
   queue-driver modules are synchronized only for a Jobs-enabled project.
4. Close mixed-App constructor seams. Shared Jobs code must not be shaped by
   the default App's Database, Metrics, Storage, or other optional selections.
   Existing disabled providers satisfy optional Metrics and Storage shapes.
   When another App makes Database part of the shared constructor shape, a
   Jobs App without Database receives one explicit App-local typed-nil Wire
   binding.
5. Make every generation entry point component-aware. Full render, App-only
   render, build generation, bare `forj generate`, explicit `--queue`
   generation, and observability-role discovery must use configuration intent
   when it is available. Stale directories and environment keys must not
   recreate Queue support.
6. Gate Queue participation in readiness, About, runtime discovery, Atlas and
   resource projections, metrics, Lighthouse registration and benchmarks,
   Grafana panels, service consumers, and generated documentation. A shared
   package may exist for another App, but an App without Jobs must not
   advertise, wire, or execute it.
7. Close generated-job registration and named-queue routing. App-owned
   `inject_jobs_app.go` owns constructor and handler registration, `make:job`
   updates both seams, and the Queue manager registers each handler with every
   generated queue runtime. Logical `default` and named resource choices map to
   their configured physical queue names without changing the public dispatch
   API.
8. Preserve enabled behavior and deployment flexibility. Jobs-enabled Apps may
   dispatch and process work using any supported runtime topology, and the
   existing worker, driver, Lighthouse, metrics, Demo, and multi-App behavior
   remains covered.

The mixed-App boundary is the highest-risk part of this phase. A named App may
be the only App with Jobs, so shared templates cannot branch on the default
App's components. Conversely, a Jobs-disabled named App must not receive Queue
managers merely because another App widened the project envelope. Metrics and
Storage already expose disabled providers for Apps that omit them. Database
has no equivalent provider, so the Jobs Wire set owns the one explicit nil
binding needed when shared source includes Database but the current App does
not. Jobs still has a known Cache dependency in its Lighthouse benchmark and
inspection plumbing. Removing that dependency belongs to Phase 5 and must not
be hidden by making Cache an implicit Jobs requirement.

The removal preflight must distinguish source ownership from runtime state.
Framework-owned Jobs files can be reconciled only after the preflight proves
the transition safe. App-owned injectors, generated-job source edited by the
user, arbitrary files in Jobs directories, external queue contents, and local
queue data are preserved. Legacy owner-file migration must be conflict-aware;
cleanup must never silently delete the former top-level Jobs injector.

Legacy rerender adds the App-owned registration seam and registers only the
framework jobs whose constructor and handler contracts are known. Older custom
`make:job` providers are preserved, but they are not guessed from arbitrary
constructors. Their one-time migration is explicit: add the job as a typed
parameter to `registerJobHandlers` and call
`queueManager.Register(<TypeName>, <job>.HandleTask)`. Future `make:job` calls
write both the provider and registration automatically.

The Jobs-disabled absence contract applies to Queue-owned generated and runtime
surface. The persisted Jobs component field, primitive-neutral inspection
observations, generic runtime source vocabulary, and dormant embedded
Lighthouse client code may remain shared when they import no Queue package and
do not advertise or register the capability.

Exit criteria:

- A fresh Jobs-disabled render has no `internal/jobs` or `internal/queues`
  package, Queue observer, App Queue accessor, job or queue make command,
  worker command, `QUEUE_*` environment, Queue metric toggle, Queue dashboard
  or platform panel, Queue service consumer, Queue runtime advertisement, or
  Queue and driver module retained by generated code.
- A Jobs-enabled render with Events and Storage disabled compiles every App,
  runs generated tests, generates Wire output, and retains workerpool as the
  active default with workerpool and Redis included.
- Both mixed-App directions are proven: default App disabled with a named App
  enabled, and default App enabled with a named App disabled. Shared Queue
  support is rendered once, each App binary contains only its selected
  participation, and only enabled Apps receive prefixed Queue environment.
- With Jobs disabled, stale `QUEUE_*` values and stale Jobs or Queue
  directories do not affect service planning, About, discovery, Atlas,
  metrics, build generation, bare generation, or rerender. An explicit
  `forj generate --queue` fails with an actionable component-disabled error.
- Enabling Jobs through full render and App-only render creates the missing
  framework boundary, persists component intent, generates Queue accessors,
  and preserves byte-for-byte every existing App-owned mapping.
- Disabling Jobs when user-owned or App-owned Jobs residue remains fails before
  any configuration, environment, migration, entrypoint, or owner-file write.
  Detection recognizes receiver methods such as `Queue()` and `Queues()`
  without false positives from comments, strings, fields, or free functions;
  removing the last Jobs-enabled App follows the same rule.
- Existing Queue driver, worker selection, Lighthouse queue health, Jobs
  metrics, observability, Demo, generator, and multi-App integration tests stay
  green, and rerender plus generation is idempotent.
- `make:job` create and remove update the App-owned constructor, typed handler
  parameter, and Queue registration together. Named dispatch reaches the named
  runtime and configured physical queue; explicit `default` dispatch respects
  the configured default physical queue.
- No Queue or Job Workers component is introduced.

### Phase 5: Cache and Inspects vertical slice

- Introduce the internal Inspects store and remove its dependence on the App
  Cache manager.
- Split cache documentation, cache shell, observer, generated manager, named
  caches, and driver generation into a Cache render group.
- Make Cache resource applicability and default planning follow the Cache
  component in the same change as its template gates.
- Gate App cache fields and accessors, providers, warnings, HTTP readiness,
  Lighthouse cache browsing, metrics, dashboards, About, discovery, commands,
  environment, and dependencies.
- Keep Auth on its single cache-backed repository implementation and make the
  Auth-to-Cache dependency explicit instead of generating a cache-free fork.
- Keep Demo settings and other Demo cache usage behind Demo's Cache dependency.
- Remove the inspection named cache from the App Cache resource catalog once
  diagnostic storage has moved. Background Jobs reads queue history from Queue
  directly and does not own a Lighthouse named cache.

Exit criteria:

- Cache-disabled Apps retain working Inspects and Lighthouse diagnostics;
- selecting Auth resolves Cache and uses the existing cache-backed repositories;
- representative Cache-enabled and Cache-disabled projects compile and test;
- Background Jobs compiles and runs without Cache;
- disabled output has no App cache API, cache environment, cache command,
  dashboard, Redis consumer, or cache module;
- enabled defaults remain memory with memory and Redis included.

### Phase 6: Cross-cutting reconciliation

- Make core dependency synchronization component and resource-plan aware.
- Make effective resource consumers honor each App's component projection.
- Ensure `.env.example`, `.env.host`, Compose, and service reconciliation do not
  resurrect disabled components from stale keys.
- Make GoForj-side resource discovery, including
  `internal/forj/resources.ProjectResolver` and Atlas projections, require the
  effective project or App component selection instead of trusting stale
  environment keys.
- Gate scenario specifications and generated documentation by explicit
  primitive requirements.
- Audit every shared template import for optional primitive leakage.
- Implement the agreed ownership-aware behavior for disabling an already
  rendered component, or explicitly restrict and diagnose that operation.
- Run `go mod tidy` only after all optional imports have been removed.

Exit criteria:

- the full absence contract is satisfied across generation, rerender, build,
  runtime reporting, and local service planning;
- no test helper or generator silently assumes the old always-on primitives;
- user-owned files and resource data are preserved.

### Phase 7: Wizard exposure and documentation

- Add Cache, Events, File Storage, and the renamed Background Jobs rows to the
  existing Components presentation.
- Keep all four selected in recommended defaults.
- Keep Database in Components and keep driver choices out of the normal flow.
- Ensure confirmation remains component-focused and does not add a resource
  hierarchy.
- Update the resource-default design's applicability table from `always` to
  the corresponding components.
- Update generated READMEs and user documentation with the distinction between
  changing a component and switching an included driver.
- Document supported additive enablement and any limits on removal.

Exit criteria:

- the normal wizard remains no-decision beyond its existing component screen;
- every exposed deselection maps to a proven disabled render;
- confirmation and generated docs tell one consistent story.

## Test-Render Strategy

Four independent booleans create sixteen primitive combinations before any
existing Web, Auth, Database, Metrics, Docker, starter-kit, or multi-App axes
are considered. Do not multiply the entire current render matrix by sixteen.

Use three layers of coverage.

### Contract unit tests

Cover:

- component defaults, parsing, ordering, dependency resolution, and
  deselection;
- old configuration migration and idempotent persistence;
- resource applicability, named selections, and service derivation;
- default-App and named-App projection isolation;
- generator inclusion predicates;
- owner-file and framework-file reconciliation decisions.

### Render sentinels

Maintain these explicit profiles:

| Profile | Purpose |
| --- | --- |
| Recommended default | Proves existing full behavior remains intact |
| Lean CLI | Cache, Events, Storage, and Jobs all disabled |
| Cache only | Proves Cache has no hidden dependency on other primitives |
| Events only | Proves event wiring and lifecycle isolation |
| Storage only | Proves storage and Lighthouse conditionality |
| Background Jobs only | Proves Queue and workers do not require Cache, Events, or Storage |
| Web without primitives | Exercises HTTP, readiness, Inspects, and Lighthouse degradation |
| Metrics without primitives | Exercises optional imports and collector construction |
| Auth | Proves dependency closure includes Cache and retains one repository path |
| Demo App | Proves dependency closure restores all required capabilities |
| Mixed multi-App | Gives different Apps disjoint primitive selections |
| Legacy project | Proves migration preserves the old always-present surface |
| Enable and disable reconciliation | Proves the supported later-change contract and owner safety |

Add pairwise profiles so every pair of primitive enabled/disabled states occurs
at least once alongside representative Web, Metrics, Database, Docker, and
multi-App axes. Pairwise coverage supplements the explicit sentinels; it does
not replace them.

### Render execution and absence assertions

Every test render is created outside the GoForj repository under `/tmp` and
uses isolated Go caches. Depending on the profile, verification includes:

```text
GOCACHE=/tmp/gocache
GOMODCACHE=/tmp/gomodcache
```

Each sentinel must:

1. render successfully;
2. compile every generated App binary;
3. run generated Go tests appropriate to that profile;
4. run Wire generation or validate checked-in generated Wire output;
5. assert expected files and environment keys are present;
6. assert disabled files, methods, commands, environment keys, dashboards,
   modules, and Compose consumers are absent;
7. rerun render and generation to prove idempotence;
8. validate project configuration round trips without widening components.

Generator absence coverage must exercise every entry point, not only the
ordinary rerender:

- bare `forj generate`;
- `forj generate --cache`;
- `forj generate --events`;
- `forj generate --storage`;
- `forj generate --queue`;
- generation invoked by build and full project render.

Bare generation skips disabled capabilities. An explicitly requested disabled
generator must refuse with an actionable component-disabled error rather than
silently recreating its package.

Absence assertions are mandatory. Successful compilation alone can hide stale
files, unused environment, unnecessary modules, and misleading runtime output.

Networked integration tests remain limited to profiles that deliberately
activate their services. Inactive included Redis drivers must not connect or
start Redis during ordinary default tests.

## Acceptance Criteria

The goal is complete when all of these are true:

1. Cache, Events, Storage, and Background Jobs are default-selected components
   in the normal Components experience.
2. Queue remains implementation machinery under Background Jobs and is not a
   separate user choice.
3. A disabled primitive satisfies the full absence contract for an initial
   project render.
4. Per-App disabled capabilities are not wired into that App even when another
   App requires shared project support.
5. Existing project and named-App configurations migrate without losing their
   previous primitive surface.
6. Inspects and core Lighthouse diagnostics work without App Cache.
7. Auth transparently requires Cache, while Jobs works without Cache, Events,
   or Storage.
8. Background Jobs owns all Queue surface without introducing another user
   component.
9. Disabled components do not create runtime discovery entries, readiness
   checks, dashboards, Compose consumers, or retained Go modules.
10. `forj generate`, build, and rerender do not recreate disabled component
    packages.
11. The curated and pairwise render matrix compiles and tests entirely from
    `/tmp` with explicit presence and absence assertions.
12. No render path deletes user-owned source or resource data.
13. Any advertised additive enablement uses framework-owned registration
    boundaries; unsupported changes fail with an actionable diagnostic.
14. The wizard contains no new resource, driver, topology, or mode stage.
15. The related design and generated documentation describe the implemented
    component applicability accurately.

## Iteration Protocol

This plan is intentionally suitable for iterative goal execution:

- complete one phase or one component vertical slice at a time;
- keep at most one slice in progress;
- run its focused contract and render sentinels before widening the matrix;
- commit working vertical behavior using conventional commit messages;
- do not expose a wizard choice for a partially gated component;
- record newly discovered template ownership or cross-component dependencies in
  this plan before expanding scope;
- update acceptance evidence after each completed phase;
- keep unrelated driver-switching and component-removal UX out of a slice
  unless it is required for that slice's truthful contract.

The default project should remain buildable at every phase boundary. A slice
that only adds configuration flags while leaving generated assumptions in place
is not a valid stopping point.
