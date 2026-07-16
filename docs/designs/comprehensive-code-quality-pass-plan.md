# Comprehensive Code Quality Pass Plan

## Status

- Plan status: accepted; implementation started
- Planning date: 2026-07-15
- Target repository: `goforj`
- Working branch: `refactor/code-quality-foundations`
- Execution model: small behavior-preserving vertical slices with focused tests and conventional commits

This plan covers framework code, generators, templates, generated application architecture, test infrastructure, and CI. It is not a mandate to split every large file or create packages solely to reduce line counts.

## Goal

Make GoForj easier to reason about and safer to change by:

- putting stable responsibilities behind clear package boundaries;
- removing successful no-ops that mask invalid construction or wiring;
- replacing process-global working-directory and environment mutation with explicit inputs;
- reducing duplicated generator, test, and CI machinery without reducing behavioral coverage;
- making generated application packages follow the same ownership rules as the framework;
- keeping tests readable, deterministic, and proportional to the behavior they protect.

## Guardrails

Every slice must follow these rules:

1. Fix a behavior or move one responsibility, not both in the same commit.
2. Keep command adapters thin and move reusable behavior behind narrow APIs.
3. Do not introduce `utils`, `helpers`, `common`, or another generic catch-all package.
4. Do not add nil guards around required wired dependencies. Validate construction where a useful error can be returned, then trust the invariant.
5. Keep nil handling for intentionally optional extensions, optional named resources, user inputs, and context normalization.
6. Treat templates and generators as sources of truth. Committed generated files are mirrors and validation targets, not independent implementations.
7. Prefer explicit roots, environment snapshots, and dependencies over `os.Chdir`, ambient environment mutation, and package globals.
8. Preserve distinct unit, render, integration, race, and stress coverage. Consolidate repeated setup and cases, not failure modes.
9. Run every test render outside the repository under `/tmp`.
10. Stop an extraction if it requires a wide exported API, creates import cycles, or makes call sites less descriptive.

## Audit Baseline

The repository currently contains approximately:

- 101,000 lines of non-template Go;
- 79,500 lines of Go templates;
- 22,000 production lines and 26,500 test lines in `internal/forj` alone;
- 1,038 Go test functions, including 565 in `internal/forj`;
- 484 `t.Setenv` calls but only 51 `t.Parallel` calls;
- several production files over 900 lines and one renderer method close to 1,000 lines.

The largest host-code responsibility clusters are:

| Area | Evidence | Direction |
| --- | --- | --- |
| Project rendering and mutation | About 8,000 production lines; `ProjectRenderer.Render` is about 960 lines | Extract stable layout and environment seams, then move rendering into `internal/projectrender` |
| Dev orchestration and UI | About 8,000 production lines across command, watcher, streamer, and TUI files | Stabilize orchestration, then extract `internal/dev`; consider `internal/devui` afterward |
| New-project wizard | About 2,850 production lines; `Update` and `View` own state, policy, and presentation | Move to `internal/newproject` after project selection rules are canonical |
| Primitive generation | About 6,700 production lines, including roughly 3,000 lines of embedded source templates | Make inputs and publication explicit before considering package splits |
| Render validation | `test_renders_cmd.go` is 961 lines and owns planning, sharding, workspaces, execution, and reporting | Extract a runner into `internal/rendercheck` |

## Package Creation Test

A new package is justified only when most of these are true:

- it owns a recognizable domain vocabulary;
- its inputs and outputs can be narrow and explicit;
- it can be tested without constructing the parent command or renderer;
- it has an independent reason to change;
- multiple callers currently depend on behavior hidden in the wrong package;
- moving it improves dependency direction;
- the extraction does not require exporting internal implementation details.

Large size alone is not sufficient. File decomposition inside an existing package is preferred when the domain is already cohesive.

## Ranked Host-Code Boundaries

### 1. `internal/projectlayout`

Move physical project-layout knowledge out of `project_renderer.go`:

- App name normalization and deterministic ordering;
- conventional App discovery;
- App, Wire, frontend, and runtime paths;
- explicit-root discovery APIs;
- intentionally distinct inventory policies with distinct names.

Renderer, dev, migrations, resources, and new-project behavior all use this knowledge today. This is the first structural extraction because it removes a backward dependency on the renderer without changing generated output.

Component and starter-kit decisions remain in `project`; filesystem layout belongs in `projectlayout`.

### 2. `internal/projectrender`

Move the renderer cluster after layout and environment seams are stable:

- rendering orchestration;
- template mapping and publication;
- App rendering and removal;
- resource projection;
- owner-safe migration coordination;
- render statistics and reporting.

Leave the Kong command adapter in `internal/forj`. First perform a move-only extraction. Decompose the coordinator only in later commits.

### 3. `internal/newproject`

Move wizard state, update/view behavior, Atlas selection, target preparation, and creation orchestration behind a narrow renderer interface. Keep component dependency and deselection rules in `project`; the wizard should only present and invoke them.

### 4. `internal/dev`

Move dev-session orchestration, task execution, watcher compilation, process replacement, and lifecycle reporting after layout and dev configuration are explicit. Consolidate pre-dev and down task execution before moving it. Extract `internal/devui` only if the stabilized orchestration boundary leaves presentation genuinely independent.

### 5. `internal/rendercheck`

Move render profile selection, matrix construction, sharding, workspace lifecycle, execution, and result aggregation out of `TestRendersCmd`. The command remains an adapter. Worker failures must be aggregated as errors rather than calling `os.Exit` from goroutines.

### 6. Focused secondary packages

Create only after their seams are proven:

- `internal/envfile` for lossless dotenv parsing, merging, precedence, and publication;
- `internal/resourceenv` for resource-plan-to-environment reconciliation;
- `internal/devconfig` for generated dev tasks and legacy watcher migration;
- `internal/projectupgrade` for owner-safe and versioned project migrations;
- `internal/compileprofile` for report, import-graph, and profile analysis currently mixed into `internal/build/profile.go`;
- `internal/apiindex/authproof` for the large AST-based generated-auth contract verifier;
- `internal/backup/portable` for portable archive and SQL translation behavior;
- `internal/devprocess` only if watcher and process-supervisor APIs remain independently useful after the dev extraction.

Do not split `project` now. Its catalog and planning model is cohesive and broadly coupled. Do not split `coredeps`; it is too small. Keep API-index publication and the watcher engine intact unless a later extraction produces a narrower real boundary.

## Generated Application Boundaries

Generated packages deserve the same ownership discipline, but changes must begin in templates and be proven through `/tmp` renders.

High-confidence later extractions are:

| Package | Current problem | Target boundary |
| --- | --- | --- |
| `internal/codeedit` | Generic Go AST/import/call editing is buried in `makecmd` | Reusable source editor with explicit operations |
| `internal/modelgen` | Model generation mixes schema discovery, relationship inference, AST editing, Wire editing, and CLI behavior | Model-generation domain with a thin command adapter |
| Demo monitoring subpackages | Controller and check service exceed 5,000 lines together | `checks`, `targets`, `incidents`, and `retention` where seams are already stable |
| `internal/benchmarks` | Jobs Lighthouse owns cache, queue, storage, DB, and HTTP benchmark engines | Jobs registers and delegates benchmark execution |
| `internal/http/inspectcapture` | HTTP server owns response wrappers, body capture, redaction, and sensitive-key scanning | Focused request/response capture package |
| `internal/about` | Runtime package owns a 1,398-line inventory/report command | About reporting consumes runtime discovery |
| `internal/resourceshell` | Database and cache shell commands duplicate target, Compose, subprocess, masking, and formatting logic | Shared shell execution policy with resource-specific adapters |

Some large generated domains should be decomposed into files before packages:

- metrics stays one domain, split into HTTP, database, queue, storage, mail, and runtime files;
- auth stays one domain, split into sessions, OAuth, passwords, reset, verification, and delivery files;
- inspects stays one domain unless a stable storage or recorder seam emerges.

Prefer file-level component gates over line-level template conditionals. Do not replace honest App-versus-project projections with a growing set of derived booleans.

## Execution Plan

### Phase 0: Safety and construction invariants

1. Validate scenario catalogs before any filesystem work:
   - reject unsafe or path-like IDs;
   - reject duplicate IDs, unknown dependencies, and cycles;
   - validate the complete graph before joining paths, writing, or removing workspaces.
2. Remove nil-receiver success paths from required project helpers and generated primitive managers.
3. Ensure required default cache, event, queue, storage, mail, and database resources are established by constructors.
4. Preserve explicit optional contracts for observers, inspect recorders, contexts, and named storage resources.
5. Audit generated Wire providers that use nil as a feature flag. Replace them with omitted contributions or narrow no-op implementations.

The named-storage outage policy needs an explicit decision: required by default with opt-in optionality, or documented optional metadata. Connection failures must not silently become healthy merely because a disk is named.

### Phase 1: Correctness and deterministic foundations

1. Fix observability generation so malformed config and filesystem errors are returned, role filtering is per role, and managed empty plans publish an empty target set.
2. Remove `os.Chdir` from build and new-project execution; pass explicit command directories and roots.
3. Replace the generator's temporary process-environment installation with an immutable project environment snapshot.
4. Introduce one atomic generated-artifact publisher with a desired inventory, write-if-changed behavior, and errorful stale-file removal.
5. Make primitive driver metadata the authority for driver imports, environment keys, supported manifests, and emitted root-key inventories.

### Phase 2: Test and CI foundations

1. Correct nonexistent generator cache-key paths in CI.
2. Pin Wire from one authority instead of mixing `@latest` with v1.2.0.
3. Extract repeated checkout/setup/tool steps only after the commands are identical.
4. Split container/Compose execution from generic `testkit` environment and filesystem helpers.
5. Move repeated generated Go fixture programs to `internal/generate/testdata` and use one fixture builder.
6. Consolidate the three model database integration templates into common contracts plus small dialect adapters.
7. Replace fixed sleeps with observable readiness or bounded polling where the behavior permits it.

### Phase 3: Dependency-unlocking package extractions

Execute one move-only extraction per commit:

1. `projectlayout`;
2. `envfile` and `resourceenv` if their APIs remain narrow;
3. `projectrender`;
4. `newproject`;
5. `devconfig`, then `dev`;
6. `rendercheck`;
7. `projectupgrade`.

After each move, run focused tests before changing logic. A later simplification commit may then shorten coordinators, remove duplicate paths, or improve names.

### Phase 4: Generator and generated-package decomposition

1. Move embedded primitive source templates into embedded `.tmpl` assets after the artifact publisher and environment snapshot are stable.
2. Split high-conditional templates by capability-owned file.
3. Extract generated packages in the ranked order above, one rendered vertical slice at a time.
4. Replace the separately maintained generated `project.Config` clone with either a generated single authority or a lossless YAML-node patcher for the fields Lighthouse actually edits.

### Phase 5: Readability, documentation, and redundancy

1. Split navigation-heavy tests by behavior and move them with extracted packages.
2. Convert homogeneous repeated cases to table tests; do not turn unrelated workflows into opaque boolean matrices.
3. Share setup and semantic assertions across expensive integration tests without combining their failure boundaries.
4. Remove exact duplicate cases only after proving they execute the same production path and assert the same contract.
5. Add concise, name-first, complete-sentence comments to every package, type, function, and method after names and ownership settle.
6. Add an AST-based comment audit over representative and full `/tmp` renders, then make it blocking only after existing violations are resolved.

## Test Strategy

Keep these layers distinct:

| Layer | Purpose |
| --- | --- |
| Focused unit and table tests | Pure policies, parsers, plans, reducers, path validation, and formatting |
| Template projection tests | File inclusion, capability gates, template formatting, and generated signatures |
| Render plus Wire plus build | Complete generated dependency graph and compile contract |
| Rendered package tests | Behavior emitted into a real App |
| Tagged database/runtime integration | External service and dialect contracts |
| Race and watcher stress | Process replacement, churn, concurrency, and shutdown invariants |

Test reduction is successful only when setup and duplication shrink while those failure classes remain covered.

## Validation Gates

Each slice runs the smallest relevant package suite with:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./path/to/changed/package
```

Milestones run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
```

Template and generator milestones additionally run the PR render matrix under `/tmp`. Runtime, database, backup, generator, and watcher changes run their existing tagged integration, race, and stress jobs before merge.

## Completion Criteria

The pass is complete when:

- command packages primarily adapt CLI input to domain services;
- renderer, dev, new-project, and validation responsibilities no longer live in one `forj` package;
- required dependencies cannot silently become successful no-ops;
- build, generation, rendering, and tests do not depend on process-global cwd or temporary ambient env mutation;
- generator artifacts publish atomically from one authoritative inventory;
- the test suite retains its behavioral layers with less duplicated setup and fewer fixed sleeps;
- package and entity documentation follows the repository convention;
- full CI, render, tagged integration, and race/stress validation is green.
