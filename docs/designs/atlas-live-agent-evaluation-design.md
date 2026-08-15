# Atlas Live Agent Evaluation Design

## Status

- Design status: proposed
- Implementation status: diagnostic vertical slice complete locally; released
  module integration pending
- Planning date: 2026-08-15
- Target repositories: `atlas` and `goforj`
- Primary ownership: Atlas evaluation library, agent adapters, guidance
  profiles, verification, and reports
- GoForj ownership: the `forj atlas:eval` CLI, executable Project scenarios,
  Project rendering, and the baseline `AGENTS.md`
- Related design: `docs/designs/completed/goforj-atlas-agent-integration-design.md`
- Cross-repository execution plan: this checked-in design is normative until
  Atlas has a checked-in implementation plan or tracked issues that reference
  it. The ignored local `/workspace/code/atlas/IMPLEMENTATION.md` is not a
  reviewable source of truth and must not direct this work in its current form.

## Summary

Atlas should provide runnable integration evaluations that start a real,
independent coding agent in an isolated GoForj Project, give it a natural user
request and a selected slice of framework guidance, and verify that the agent
takes the correct framework actions.

The existing deterministic Atlas workflow evaluations answer:

> Does Atlas return the right plan, tools, files, and warnings for this task?

The live-agent evaluation layer should answer:

> Given only this guidance, does a fresh agent discover and follow the intended
> GoForj workflow and leave behind a correct Project?

Both layers are required. Deterministic evaluations remain the fast, stable
test of Atlas knowledge. Live-agent evaluations test instruction discovery,
tool selection, framework usage, implementation quality, and validation as one
end-to-end behavior.

GoForj should own the user-facing command, consistent with every other Atlas
entry point:

```bash
forj atlas:eval run add-http-controller --agent codex --guidance agents
```

The command should be a thin wrapper over an Atlas evaluation library. Projects
must not need a separately installed `atlas` executable, and GoForj must not
implement a second evaluation engine.

## Decision

Build a provider-neutral live-agent evaluation harness in Atlas with:

1. versioned live-evaluation definitions that reference stable executable
   GoForj scenario IDs plus Atlas workflow and verifier IDs;
2. GoForj Projects prepared through the existing scenario system in isolated
   temporary directories;
3. explicit guidance profiles;
4. fresh agent processes with no inherited conversation;
5. command, tool, transcript, filesystem, and supervisor-computed diff capture;
6. deterministic behavioral and artifact verification;
7. optional qualitative review that cannot override hard failures;
8. repeatable trials and comparative scorecards;
9. best-effort local execution for development; and
10. sandboxed scheduled and release-gate execution.

Do not treat a model-generated answer as the test result. The result is the
agent's observable behavior and the Project it leaves behind.

## Motivation

GoForj agents frequently fail before implementation quality becomes relevant.
They may:

- create a controller manually instead of running `forj make:controller`;
- miss registration points that the generator would have updated;
- edit `wire_gen.go`;
- put a feature in the default App when the request belongs to another App;
- place data access in controllers instead of repositories;
- invent framework conventions from general Go knowledge;
- use hosted Unreleased documentation against an older Project without
  reconciling the installed version; or
- run a narrow validation command that does not prove the feature works.

GoForj can render a concise baseline `AGENTS.md` whenever baseline agent
guidance is selected, whether or not Atlas is installed. Atlas can additionally
provide skills, project inspection, version-aware documentation, and MCP
workflows. However, writing correct guidance does not prove an agent will
notice or obey it.

A live evaluation closes that gap. It lets maintainers make claims such as:

- `AGENTS.md` alone is sufficient for the core generator-first workflow;
- installing a route skill materially improves route implementation quality;
- Atlas MCP prevents incorrect named-App placement;
- a guidance edit fixes a known failure without regressing other scenarios;
  and
- a new Atlas or GoForj release preserves agent success rates.

## Goals

1. Execute actual coding agents against realistic GoForj tasks.
2. Prove the generated `AGENTS.md` baseline works without Atlas installation.
3. Measure the incremental value of Atlas skills and MCP tools.
4. Verify both the action path and the final Project.
5. Keep hard correctness checks deterministic and reviewable.
6. Diagnose failures from retained, redacted artifacts and deterministically
   replay verification when the retained reconstruction contract permits it.
7. Convert real failures into permanent scenarios.
8. Support multiple agent providers without weakening isolation.
9. Track nondeterministic behavior through repeated trials rather than one-off
   demonstrations.
10. Keep live model cost and credentials out of ordinary unit-test execution.

## Non-Goals

1. Replacing Atlas unit, workflow, or MCP protocol tests.
2. Requiring byte-for-byte golden diffs from agents.
3. Requiring one exact sequence of harmless implementation steps.
4. Using an LLM judge as the sole correctness oracle.
5. Benchmarking general model intelligence.
6. Ranking agent vendors publicly from a small private fixture set.
7. Giving the evaluation agent access to a maintainer's normal home directory,
   global skills, credentials, or conversation history.
8. Running paid live-agent evaluations on every ordinary GoForj test command.
9. Allowing the runner to publish branches, open pull requests, or mutate
   external services.
10. Hiding framework defects by teaching the evaluator to accept a workaround.

## Design Principles

### Evaluate behavior, not prose

An agent succeeds by changing the Project correctly. A polished final response
cannot compensate for an incorrect App, a skipped generator, or a broken build.

### Verify the workflow and the outcome

A compiling controller created by hand is not a complete success when the
framework contract requires `forj make:controller`. Command traces and final
artifacts measure different aspects of correctness; the evaluator needs both.

Reports must keep those endpoints separate:

- `framework_outcome_pass` means the resulting application satisfies its
  independent behavioral, ownership, compatibility, and maintainability
  contract; and
- `workflow_conformance_pass` means the agent followed the scenario's required
  GoForj generator, inspection, registration, and validation workflow.

A scenario may define `contract_pass` as requiring both, but skipping a
preferred workflow must not erase evidence that an application is otherwise
correct. Claims about application quality use the framework-outcome endpoint;
claims about guidance and generator-first behavior report conformance.

### Guidance must be attributable

The runner should expose only the requested guidance profile. If global MCP
servers or skills leak into the run, the result cannot establish whether the
tested `AGENTS.md` or Atlas skill was effective.

### Hard facts use deterministic checks

Go parsing, route inspection, command traces, file ownership, supervisor diffs,
builds, and tests should decide objective facts. Qualitative model review may
supplement those checks but must not turn a hard failure into a pass.

### Success is statistical

Agent behavior is nondeterministic. Important claims should be based on repeated
trials with recorded agent and model versions, not one successful transcript.

### Failures become regressions

A real failure should be reducible to the smallest safe scenario that preserves
the failed behavior. The resulting scenario belongs in the evaluation suite.

## Relationship To Existing Atlas Evals

Atlas currently has deterministic workflow fixtures that score whether its
planning surface selects the right:

- App;
- command path;
- files;
- documentation;
- Atlas tools;
- ownership policy;
- validation checks; and
- mistake warnings.

Those fixtures do not call a model. They should remain fast enough for normal
`go test ./...` execution.

The live layer consumes the same workflow vocabulary where practical but tests
a different boundary:

```text
deterministic workflow eval
  task + project facts
    -> Atlas plan
      -> score Atlas knowledge

live-agent eval
  isolated Project + prompt + selected guidance
    -> independent agent actions
      -> changed Project
        -> score behavior and outcome
```

The live layer should not duplicate expected workflow metadata by hand when an
existing deterministic scenario can be referenced. It may add implementation
assertions that only make sense after an agent has edited a Project.

## Product Surface

### List scenarios

```bash
forj atlas:eval list
```

Example output:

```text
add-http-controller       Add an HTTP controller for existing invoice behavior
add-app-command           Add and register an App command
add-job                   Add a job with a typed payload
add-named-app-route       Add a route to an additional App
create-model              Create a model from a database table
repair-wire-provider      Diagnose and repair a missing provider
```

### Run one scenario

```bash
forj atlas:eval run add-http-controller \
  --agent codex \
  --guidance agents \
  --intent authoritative \
  --backend container-ci
```

### Compare guidance profiles

```bash
forj atlas:eval compare add-http-controller \
  --agent codex \
  --guidance none,agents,skills,recommended-hermetic \
  --trials 5
```

### Run a suite

```bash
forj atlas:eval suite core --agent codex --guidance agents --trials 3
```

### Inspect a retained run

```bash
forj atlas:eval report /path/to/run-artifacts/attempt-01J5Y6KQ7M4C
```

Authoritative output should be concise:

```text
add-http-controller  ·  Codex  ·  AGENTS.md only

✓ Inspected .goforj.yml
✓ Selected app
✓ Ran forj make:controller invoices
✓ Preserved generated-file ownership
✓ Returned the seeded invoice through the established domain boundary
✓ Project tests pass
✓ No unrelated changes

Outcome PASS  ·  Workflow PASS  ·  42s
```

An unconfined diagnostic run must be explicit:

```bash
forj atlas:eval run add-http-controller \
  --agent codex \
  --guidance agents \
  --intent diagnostic \
  --backend unconfined-local
```

It may report deterministic artifact checks, but it cannot upgrade them into
trusted action evidence:

```text
Artifact checks PASS  ·  Outcome INELIGIBLE  ·  Workflow INELIGIBLE
Unconfined diagnostic: run with --intent authoritative --backend container-ci
to produce evaluation evidence.
```

On failure, the report should name the failed invariant and point to the
retained trace and diff:

```text
Outcome PASS  ·  Workflow FAIL

✗ Expected command: forj make:controller invoices
  Agent created internal/invoices/controller.go directly.

Impact: application behavior is correct, but the supported GoForj workflow was
not followed and generator-owned registration may drift.
Next: inspect guidance discoverability before changing the verifier.

Artifacts: /path/to/run-artifacts/attempt-01J5Y6KQ7M4C
```

## Scenario Model

A live evaluation is a versioned test contract, not merely a prompt. It should
compose existing sources of truth instead of defining another Project recipe
language.

GoForj already owns executable `ScenarioSpec` definitions with component
selection, ordered steps, dependencies, verification commands, isolated
workspaces, and stable scenario IDs. The current unversioned schema has one
ordered `Steps` stream that feeds both execution and generated documentation.
It does not distinguish fixture preparation from the feature-building steps an
agent should perform. Atlas already owns workflow guidance and deterministic
evaluation metadata. The live layer should add only that missing boundary; it
must not turn either repository's metadata into the other's policy language:

```text
GoForj ScenarioSpec ID
  -> Project starting state, golden target, and reference-path validation

Atlas WorkflowExpectationID
  -> expected framework workflow and guidance

Atlas VerifierID
  -> independent outcome contract and change policy

Live evaluation manifest
  -> prompt plus references to those three contracts
```

GoForj should add one minimal versioned boundary:

```text
render Project
  -> complete dependencies
  -> apply optional preparation steps
  -> verify starting state
  -> stop for live-agent preparation

ordinary scenario:test continues
  -> apply target steps
  -> verify completed state
```

One `ScenarioSpec` represents one starting state and one golden target. Its
existing `steps` remain the target recipe. Its full `depends_on` closure is
always applied before preparation, and an optional `prepare` block adds only
fixture work that does not deserve a separate reusable scenario. Preparation
and target steps have stable IDs rather than relying on titles or list
positions. A scenario that needs a different starting state or target gets a
new scenario ID and composes existing dependencies; it does not add another
projection mode to the same file.

The additive GoForj shape is deliberately small:

```yaml
schema_version: 2
id: invoice-http-route
depends_on: [invoice-domain]
app:
  module_name: example.com/scenarioapp
  components:
    web_api: true
prepare:
  steps:
    - id: seed-invoices
      title: Seed invoices
      command: ["go", "run", "./internal/testseed"]
  checks:
    - command: ["go", "test", "./internal/invoices"]
steps:
  - id: scaffold-controller
    title: Scaffold the Controller
    command: ["forj", "make:controller", "invoices"]
checks:
  - command: ["forj", "build"]
  - command: ["go", "test", "./..."]
```

The existing `markdown` block remains available because it generates the
runnable-scenario documentation, but it does not acquire evaluator policy.
`prepare` is rendered as fixture or prerequisite context; ordinary target steps
remain the reader-facing workflow. The schema rejects unknown versions,
duplicate step IDs across preparation and target, and dependency cycles.
Preparation tests prove that only dependency and preparation IDs execute and
that starting-state checks fail if target behavior is already present.

Legacy and external scenario catalogs remain valid. An unversioned legacy spec
keeps its current `scenario:list`, `scenario:generate`, `scenario:test`,
dependency, and documentation behavior. It is not live-evaluable until it has a
versioned preparation boundary; attempting to prepare it returns a stable
`unsupported_live_scenario` error before mutation. Checked-in specs may migrate
incrementally. Migration tests must compare ordinary golden-path execution and
generated documentation before and after migration.

For a migrated spec, ordinary `scenario:test` still exercises the full golden
path: completed dependencies, preparation, starting-state checks, target steps,
then final checks. Live preparation stops after starting-state checks. Existing
dependency meaning does not change, so legacy execution and generated
documentation remain understandable without a projection matrix.

The imported Atlas verifier is authored independently from the GoForj golden
steps. It defines observable requests and responses, persistence fixtures,
required registrations and ownership, prohibited behavior, allowed
implementation families, and scenario-specific domain constraints. Golden
steps and reference final checks prove the documented path; their file contents,
identifiers, exact code shape, and output substrings are not the live verifier
oracle. Both contracts may call the same independently tested black-box helper
when they genuinely assert the same public behavior.

The preparation API should complete dependencies and preparation steps, return
the resolved Project configuration, exact catalog/spec and transitive dependency
digests, run the starting-state checks, and leave target steps unapplied for the
agent.
It must use an explicitly supplied GoForj executable and `PrepareOptions`
rather than whichever `forj` or shared test cache happens to be available in
the current scenario executor. Toolchain, execution-backend, and cache policy
belong to `PrepareOptions`, not `ScenarioSpec`, because they are runner policy
rather than scenario semantics.

### Scenario schema maintainability rules

The GoForj scenario format is a small, closed recipe schema, not a general
workflow language:

- unversioned files retain v1 behavior; live preparation requires explicit
  `schema_version: 2`;
- one file has one scenario ID, one starting state, and one golden target;
- `depends_on` is the only reuse mechanism and always means “complete this
  scenario before mine”;
- dependencies express required application state, never documentation order;
  keep their closure focused rather than inheriting an unrelated cumulative
  showcase;
- `prepare.steps` and `steps` use the same closed action union: exactly one of
  `command`, `write`, `append`, or `replace`;
- action commands are structured argument arrays and succeed only on a zero
  exit; preparation and final checks use `command` plus optional `contains`;
- YAML never contains shell pipelines, conditionals, loops, expression
  evaluation, or executable hooks;
- step IDs are machine contracts, while titles and explanations remain freely
  editable documentation;
- maps reject unknown fields, duplicate keys, aliases, merge keys, multiple
  documents, unknown action types, and unknown schema versions;
- defaults are applied in one decode phase, semantic validation happens before
  filesystem mutation, and execution consumes only the validated plan; and
- errors identify the file, scenario, field path, and conflicting ID or value.

Dependency steps execute once in topological order. A parent does not rerun
each dependency's final checks; its own `prepare.checks` owns the combined
starting-state contract. This preserves current execution semantics and avoids
turning a dependency chain into repeated full-suite validation. Documentation
ordering continues to use narrative metadata and links rather than dependency
edges.

Adding an action or changing dependency meaning requires a schema version and
migration tests. Shared orchestration behavior belongs in typed Go helpers,
not new YAML mini-languages. Large code bodies may remain in the existing recipe
when they are the source for executable documentation; they are never parsed as
live-verifier expectations.

File length alone is not a reason to add includes, inheritance, YAML anchors,
or fragment discovery. Keep one obvious source file for one documented recipe.
Extract another scenario only when it represents independently reusable
application state with its own stable contract.

Decode v1 and v2 through separate strict wire structs selected by the small
schema header, then normalize both into the same internal plan types. Do not
grow one struct full of version-dependent optional fields or scatter version
branches through execution and documentation.

The loader compiles every validated spec and its dependency closure into one
immutable `ScenarioPlan` with five ordered stages: dependency steps, preparation
steps, starting checks, target steps, and final checks. Existing
`scenario:test` consumes the complete plan; live preparation consumes only the
prefix through starting checks. Documentation reads the same plan metadata.
There is one dependency walker, one action executor, and one command runner;
the live path does not introduce a second interpreter or special-case YAML
execution.

Atlas evaluation YAML is even smaller. It only joins a prompt and three promoted
IDs with scenario-specific budgets. Workflow requirements, observation
capabilities, change policy, and outcome assertions stay in typed, versioned
contracts where they can be tested once. Loader tests should prove that a
minimal valid manifest loads, every field has one owner, imported capabilities
compose deterministically, and no manifest field can weaken an imported
workflow or verifier.

The harness test matrix stays compact and contract-focused:

- every existing v1 scenario produces identical generated documentation and
  complete execution after the v2 loader lands;
- a v2 complete run equals prepare-prefix execution followed by its target and
  final checks;
- dependency diamonds execute each dependency exactly once and preserve order;
- live preparation never executes a target step, including after preparation or
  starting-check failure;
- strict decoding rejects every unknown field, duplicate key or ID, invalid
  action union, alias, merge, cycle, and unsupported version before mutation;
- manifest resolution rejects missing or incompatible scenario, workflow, and
  verifier versions; and
- two valid implementation families and targeted mutants calibrate each live
  verifier independently from the golden recipe.

Atlas deterministic workflow fixtures should expose a separate stable,
versioned `WorkflowExpectationID`. A live definition imports that expectation
as required behavior, may add stricter live-only requirements, and may not
weaken or silently override imported requirements. Planner recommendations
remain distinct from live hard requirements. Free-form text and substring-only
fixtures cannot satisfy a hard requirement. Conflicts fail manifest loading.
The identifier belongs to the Atlas live definition, not GoForj's
framework-neutral scenario catalog. An integration contract validates every
Atlas scenario reference against the embedded GoForj catalog.

Suggested Atlas layout:

```text
eval/
  evaluations/
    add_http_controller/
      evaluation.yaml
      prompt.md
  verify/
    add_http_controller.go
    add_http_controller_test.go
  agent/
    adapter.go
    codex.go
    claude.go
  guidance/
    profile.go
    materialize.go
  isolate/
    environment.go
    project.go
  capture/
    events.go
    redact.go
  report/
    scorecard.go
    terminal.go
    json.go
```

The GoForj CLI should contain only the thin `atlas:eval` wrapper and Project
scenario adapter required to call this library.

An initial evaluation manifest could use this shape:

```yaml
schema_version: 1
id: add-http-controller
summary: Add an HTTP controller for existing invoice behavior
suite: core
project_scenario: invoice-http-route
workflow: goforj-add-http-route/v1
verifier: add-http-controller/v1
limits:
  wall_time: 10m
  commands: 80
  shell_network: off
```

The prompt is always the adjacent `prompt.md`; its digest is recorded in the
resolved manifest. The manifest composes three independently versioned
contracts and does not restate them:

- `project_scenario` owns the starting Project and golden target recipe;
- `workflow` owns required framework actions and their observation classes;
  and
- `verifier` owns outcome checks, allowed implementation families, protected
  ownership, and the semantic change budget.

The runner derives required capabilities as the union of the imported workflow
and verifier contracts. Run intent, backend selection, agent, guidance profile,
trial count, and provider policy are invocation or experiment policy and do not
belong in evaluation YAML. This keeps an evaluation stable when infrastructure
or experimental treatments change.

`invoice-http-route`, `goforj-add-http-route/v1`, and
`add-http-controller/v1` are proposed identifiers, not current executable
catalog entries. Phase 1's first deliverable is a promoted, tested mapping that
adds or deliberately maps all three. Manifest loading fails before Project
mutation when a scenario, workflow, verifier version, or imported requirement
is absent, stale, ambiguous, or conflicting.

Every workflow or verifier predicate declares the trusted observation class
that proves it. Project-inspection behavior, for example, may use `file_reads`
or trusted `mcp_tool_calls`; it cannot be required when the adapter and backend
expose neither. The first slice treats Project inspection as a quality signal
unless trustworthy observation is available.

Command matching should normalize the supervisor-observed executable by trusted
digest, not its spelling on `PATH`. Each generator gate should compile into a
typed `GeneratorRequirement` that declares:

- trusted executable identity and accepted argument grammar;
- App and resource normalization;
- required successful exit;
- protected paths or ownership classes;
- process ancestry and write-provenance rules;
- generated outputs and permitted post-generation edits; and
- concrete assertions proving those outputs remain in use in the final tree.

Generator verification must distinguish trusted generator descendants from
manual writes before or after the invocation. Calibration should cover the
wrong resource, wrong App, failed generation followed by manual work, correct
generation after an earlier protected write, replaced output, copied binaries,
shell invocation, and absolute executable paths.

Each verifier declares a machine-readable change budget through semantic
classes rather than an exhaustive path recipe. Classes include feature
implementation, trusted generated output, focused tests, required
configuration, and module or dependency changes. Explicit protected paths and
known-unrelated changes may hard-fail. Other out-of-budget changes are reported
for calibration before becoming gates. Every hard budget rule needs a valid
variant that proves it does not reject necessary supporting work.

The typed verifier should own checks that cannot be expressed cleanly as
manifest data. Examples include parsing a constructor, proving a route calls a
service, or checking that data access remains behind a repository.

The manifest parser should reject unknown fields for its current schema version
so a misspelled requirement cannot silently disappear. Schema migrations must
be explicit.

### Prompts

Prompts should resemble normal user requests and should not leak the answer.

Good:

```text
Add an endpoint that returns an invoice by ID. Keep the HTTP layer thin and use
the existing database connection.
```

Bad:

```text
Run forj make:controller invoices, edit these four files, and then run these
exact tests.
```

The evaluated guidance is responsible for teaching the framework workflow.

### Project preparation

Project fixtures should be prepared from the existing GoForj scenario catalog,
not maintained as copied Projects or parallel Atlas render recipes. A retained
evaluation should resolve and record:

- GoForj scenario ID and scenario schema version;
- exact GoForj executable path, digest, version, and commit when available;
- selected components, starter kit, Apps, and resource drivers projected from
  the scenario;
- exact Go, Wire, package-manager, Atlas, docs, and sibling-module identities;
- seed schema or fixture-data digest;
- expected healthy or intentionally failing starting-state contract; and
- the live evaluation and verifier versions.

The runner should construct a fixed `PATH` containing only the resolved
toolchain. Retained contracts must not use an unresolved value such as
`goforj: current`. Authoritative preparation runs in a pinned container or VM
image, records that image and the exact GoForj executable digest, disables
toolchain auto-install or upgrade behavior, and fails when the supplied Wire,
Go, or package tool identity is unavailable. Local diagnostics may record
individual host tool identities instead.

Atlas should own the dependency-inversion boundary while GoForj implements it.
The contract is data-only and must not require Atlas to import GoForj internal
packages:

```go
type ProjectPreparer interface {
	Capabilities(context.Context) (PreparationCapabilities, error)
	Resolve(context.Context, PreparationRequest) (ResolvedPreparationPlan, error)
	Prepare(context.Context, PreparationRequest, ResolvedPreparationPlan) (PreparedProject, error)
}

type PreparedProject interface {
	Result() PreparationResult
	Close(context.Context) error
}
```

`PreparationRequest` identifies one scenario, destination, candidate GoForj
executable, non-secret trusted orchestration identifier, fixture inputs, and
explicit `PrepareOptions`. It never carries signing keys, bearer authority,
broker credentials, or reusable supervisor handles.

The trusted promoted catalog produces a signed or authenticated, versioned
`ResolvedPreparationPlan`. It contains the Project configuration, ordered preparation
actions, proof that target steps are omitted, starting-state checks,
environment projection digest, and expected catalog and dependency digests.
Candidate GoForj code may execute only product operations named by this plan;
it cannot substitute its embedded scenario catalog or redefine preparation or pass
criteria. A candidate-catalog mismatch is ineligible, not self-consistent
evidence.

`Prepare` receives both the original trusted request and its resolved plan. This
keeps preparers stateless across resolution and execution, so retries, concurrent
attempts, and process-separated implementations do not depend on an in-memory
request registry. The orchestration identity in the request, plan, and result
must match. The overall plan digest binds the scenario-prefix digest, exact
GoForj executable digest, and the non-secret material environment projection;
those component digests remain explicit fields so a mismatch can be diagnosed
rather than appearing only as an opaque hash failure.

`PreparationResult` returns resolved identities, tree and catalog digests,
health-check evidence, and ownership of every created path. Atlas owns the
destination root; GoForj may populate only the requested destination. `Close`
is idempotent and runs under a fresh bounded context. If `Prepare` acquires any
resource before failing, it returns a cleanup handle or transactionally removes
all partial children, listeners, mounts, caches, and files before returning.

The trusted supervisor resolves the plan and invokes the candidate GoForj
executable only inside the untrusted preparation boundary. Bearer authority and
signing material remain outside; any authenticated callback is single-use and
bound to the destination and operation.
The known-good target or repair oracle runs in a separate disposable
materialization and can never modify the agent trial. Before any mutation, the
CLI and library exchange versioned capability sets; an unavailable preparation
schema or observation capability returns `unsupported_evaluation_capability`.

All rendered Projects must be created outside the GoForj repository, under an
explicit temporary root. Release-resolution scenarios should validate the
selected module graph with `GOWORK=off`; candidate-source scenarios should
record intentional local replacements explicitly.

## Deferred Project Cache Contract

This section is an implementation appendix rather than part of the initial
product surface. The first slice uses only per-command immutable preparation;
cross-command persistence remains Phase 5 work after scenario and product
feedback loops are operating.

Repeated trials should not repeatedly render the same Project, download the
same modules, install the same frontend dependencies, or perform other
deterministic fixture preparation.

The first vertical slice should prepare one immutable Project base per command
and copy it for repeated trials. It should use wholly trial-private Go module,
Go build, and package-manager caches. Persistent cross-command caching and
dependency seeds are deferred until representative small and large scenarios
measure a real bottleneck.

The first request for a prepared-base key should:

1. render the Project into a staging directory;
2. materialize the deterministic fixture environment;
3. perform the referenced scenario's declared dependency preparation;
4. run any required environment-sensitive code generation;
5. verify that the starting Project has the expected healthy or intentionally
   broken state;
6. remove transient and sensitive state, including the rendered `.env`;
7. write a manifest, full tree digest, and completion marker; and
8. atomically publish the staging directory as an immutable cache entry.

Every trial should then create its own writable Project by recursively copying
that prepared base into the trial's temporary directory. The prepared base must
be guidance-neutral. After the private copy exists, the selected profile must
atomically project both the durable guidance configuration and every managed
instruction file before the trusted baseline snapshot is captured. The
projection is part of the treatment, not temporary filesystem decoration.

Preparation should therefore request a guidance-neutral projection from the
GoForj scenario adapter, even when the source Project selects baseline
guidance. It should not render `AGENTS.md` and then attempt to guess which
unmanaged content is safe to delete. The overlay phase sets the private
fixture's durable value to `none` or `baseline`, persists the selected native
projection targets, and invokes the canonical writer. Later `forj render`,
`forj build`, and `forj make:*` commands must preserve the selected profile
rather than materializing missing guidance or removing selected guidance.

Profile qualification runs those lifecycle commands inside every profile and
then reasserts selected-file hashes, unselected-file absence, managed-marker
ownership, and the unchanged non-guidance tree digest. Any drift invalidates
the trial. `none` removes only managed framework projections from the private
fixture; it never rewrites the source Project or unmanaged instructions.

The base must also be runtime-environment-neutral. Current Project rendering
creates random application, diagnostics, Lighthouse, and JWT values in `.env`,
so byte-identical preparation inputs do not produce identical trees. The
preparer should use a declared deterministic fixture environment before every
environment-sensitive generation or health command, omit `.env` from the
published base, and recreate the same non-secret shape in every trial before
baseline capture and health checks. Each private trial then receives unique,
revocable synthetic credentials for only its own fixture endpoints. The
projection digest records names and classifications rather than secret values.
Cached and uncached named database, cache, queue, storage, and other resource
accessors must remain generation-identical after the overlay.

```text
GoForj scenario + preparation request
  -> immutable prepared base (once)
    -> private trial copy A
    -> private trial copy B
    -> private trial copy C
```

The trial must never run directly inside the cached base.

### Cache location

Local runs should default to an Atlas-owned directory beneath the platform user
cache directory so separate evaluation commands can reuse prepared bases. CI
and test callers should be able to point the cache at a job-scoped temporary
directory or a restored CI cache through an explicit flag or environment
setting.

The cache must never live inside the source GoForj or Atlas repository. Its
resolved location should appear in debug output and the run manifest, while the
normal scorecard should report only whether materialization was a cache hit or
miss.

The resolved root must be a dedicated Atlas-owned directory with a validated
format and ownership marker. Cache commands refuse filesystem roots, home,
repository or workspace roots, empty or unresolved paths, symlinks, and mount
crossings. `prune` and `clear` delete only recognized unpinned entries beneath
that root; fake-filesystem tests prove unrelated files survive every malformed
or overly broad target.

Suggested override:

```bash
forj atlas:eval run add-http-controller \
  --project-cache-dir /tmp/atlas-eval-projects
```

### Cache key

Define a canonical `PreparationRequest` before rendering. Its digest should be
the cache key and should include every pre-render input capable of changing the
prepared Project:

- schema-versioned canonical JSON with sorted map keys, normalized relative
  slash paths, and explicit absent-versus-empty semantics;
- digest of the resolved GoForj scenario spec, catalog entry, and
  transitive dependency specs;
- exact GoForj executable digest and resolved release or commit;
- Atlas evaluation preparation-engine digest;
- pinned authoritative container or VM image digest;
- fixture seed files and seed-schema digest;
- selected components, drivers, starter kit, and Apps;
- relevant render configuration;
- digests of the resolved Go, Wire, package-manager, and preparation
  executables or scripts;
- resolvable revisions and tree digests for allowed external module
  replacements;
- effective `GOOS`, `GOARCH`, architecture variant, `CGO_ENABLED`, compiler,
  `GOROOT`, and `GOEXPERIMENT` identities;
- preparation command definitions;
- fixed `GOFLAGS`, `GOWORK`, `GOENV=off`, `GOTOOLCHAIN=local`, and module-proxy
  policy;
- non-secret environment inputs explicitly declared by name and value; and
- cache format version.

Persistent-cache mode should reject non-hermetic external replacements whose
identity cannot be resolved. Local ephemeral preparation may permit them only
when it records the run as non-reusable.

Generated dependency manifests, lockfiles, and the final tree digest are not
pre-render key inputs because they do not exist yet. Record them as preparation
postconditions and verify them before publication and on every cache hit. A
postcondition mismatch invalidates the entry; it does not create an alternate
key after the fact.

Guidance hashes should not invalidate a guidance-neutral Project base. This
allows the same prepared Project to test `none`, `agents`, `skills`, `mcp`, and
`recommended-hermetic` guidance profiles without rerendering application code.
The Atlas preparation-engine identity remains part of the key because it owns cache
materialization and hygiene. If other Atlas output is genuinely required to
create a fixture, declare that as a separate Project-preparation capability
rather than silently including it in the common base.

The cache manifest should record the resolved inputs, preparation steps,
created files, and validation result so a retained evaluation can explain which
base it used.

### Dependency preparation

Prepared bases should contain source and deterministic generated outputs, but
should exclude `node_modules` by default. Recursively copying a frontend
dependency tree can cost more than rendering the Project. A scenario may opt
into prepared frontend dependencies only after measurement proves that
copy-on-write materialization is available and materially faster on the target
platform.

Go module, Go build, and package-manager download caches should remain separate
from the immutable Project-base cache. The first authoritative implementation
uses wholly trial-private caches. A trusted dependency-acquisition phase may
hydrate them before baseline capture through pinned, checksum-enforcing proxies
or declared local fixture endpoints. Network `off` means no undeclared external
egress; it does not prohibit supervisor-declared loopback fixtures. The first
slice may accept repeated verified downloads rather than weakening provenance.

Later seed support is opt-in per tool and requires a tool-specific
immutability, writable-overlay, corruption, and performance contract. A generic
directory overlay is not assumed to work for Go, npm, pnpm, and yarn.

An agent must never receive write access to a cache shared with another trial
or a future trusted run. Any entry or signing key reachable by
`unconfined-local` belongs to an untrusted cache namespace and can never satisfy
an authoritative hit or be promoted into one.

Cache namespaces must separate trusted default-branch/release preparation from
untrusted candidate preparation. A pull-request or other untrusted job must
never publish or promote a cache consumed by a trusted run. Authoritative cache
storage and signing authority must be physically inaccessible to the
unconfined identity; a namespace label alone is not a trust boundary.

Prepared dependency state must not weaken the test. If a scenario is intended
to verify dependency installation or first-run behavior, its contract should
explicitly disable or vary that preparation step.

### Materialization

The portable implementation should recursively copy only regular files and
ordinary directories. It should preserve normalized executable permission bits
and directory structure while stripping setuid/setgid bits, ACLs, extended
attributes, file capabilities, and other execution-relevant metadata not
covered by the authenticated manifest.

Version 1 should reject symlinks in prepared bases. This avoids absolute,
escaping, dangling, looping, and cross-platform link semantics at the security
boundary. If a future GoForj scenario genuinely needs a symlink, it should add
an explicit capability that permits only relative links whose fully resolved
targets remain beneath the prepared base and trial root, backed by containment
tests on every supported platform.

The same containment rule applies to prompts, seeds, guidance overlays, agent
configuration, cache metadata, final-tree walking, redaction, and artifact
archiving. Version 1 freezes or terminates the sandbox before final walking,
uses `Lstat` without dereferencing links, rejects hard links and mount or
reparse crossings, and never opens FIFOs, sockets, devices, or unknown entry
types. Agent-created symlinks may be recorded as metadata but not followed.
Per-file size and processing limits apply to hashing, redaction, diffing, and
archiving. OS-specific descriptor-relative beneath-root operations harden the
authoritative backend but are not a false portable `filepath` guarantee.

Platforms may provide copy-on-write cloning or reflinks as an optimization, but
the result must behave as a private writable copy. Plain hard links are not
acceptable for files an agent may modify because one trial could mutate the
cached base or another trial. The implementation should fall back to an ordinary
recursive copy whenever clone semantics are unavailable or uncertain.

The materializer should exclude cache metadata that does not belong to the
Project and should never copy cached Git metadata. After guidance is installed,
the trusted supervisor should capture an immutable baseline tree and digest
outside the agent-writable namespace. Final diffs and scoring must compare
against that supervisor-owned baseline rather than trusting `.git`, hooks,
attributes, filters, or commits the agent could modify. A convenience Git
repository may be exposed to the agent, but it is never evaluation evidence.

### Concurrency and publication

Parallel requests for the same missing key should coordinate through a
cross-process per-key lock. The lock should record owner and start metadata,
define bounded waiting and stale-owner recovery, and remain outside the copied
Project tree. One process prepares the base while the others wait and then
consume the completed entry.

The portable tree digest should hash every materialized entry through sorted
relative slash paths, entry type, regular-file contents, empty directories, and
normalized permission bits relevant to execution. Transient paths are removed
and their absence verified before publication; they are never copied while
excluded from the digest. Supervisor metadata remains outside the Project.
Preparation should reject links, unsupported entry types, and case-folding path
collisions so an entry cannot materialize ambiguously across platforms.

An entry is valid only when its completion marker, manifest schema, request
digest, authenticated provenance, postconditions, and full tree digest verify.
Authoritative cache storage must be writable only by a trusted supervisor and
use a supervisor MAC/signature or equivalent authenticated metadata; an
attacker replacing both content and an unauthenticated digest must not create a
valid hit. Preparation failures
must remove or quarantine the staging directory and must never publish a
partial entry. Publication should use an atomic rename on the same filesystem.

Readers acquire a shared per-entry pin under the entry lock before validation
and retain it through copy completion. Prune and clear acquire exclusive access
and cannot remove a pinned entry. Pins use supervisor-owned tokens and bounded
heartbeats for stale-reader recovery. Pruning uses explicit byte and entry
limits, a minimum age, and deterministic oldest-unpinned selection; staging and
quarantine cleanup follows separate bounded rules.

Full hashing is mandatory on publication, every cache hit, and after creating
the private trial copy. Authenticated metadata proves who published the expected
digest, not that unread live content still matches it. A future fast path
requires immutable storage that cryptographically binds live content to that
digest and passes the same corruption tests. Cache validation cost is included
in cached-versus-uncached benchmarks.

Parallel trials may read the same published base, but each must receive a
different writable materialization directory, agent home, Atlas state, and Git
repository.

### Cache hygiene

The immutable base must not retain:

- `.git` directories;
- agent transcripts or evaluation reports;
- provider credentials or authorization configuration;
- runtime `.env` files and untracked environment secrets;
- runtime databases unless they are declared fixture inputs;
- process IDs, sockets, locks, special files, or active service state;
- logs from fixture preparation inside the Project tree; or
- absolute paths that bind the Project to its staging directory.

Bounded redacted preparation summaries and command statuses belong in
supervisor-owned cache metadata. Full redacted preparation logs belong only in
an explicit debug artifact, never in the materialized Project.

In Phase 5, GoForj should provide explicit cache inspection and removal
commands rather than silently rebuilding every fixture:

```bash
forj atlas:eval cache list
forj atlas:eval cache prune
forj atlas:eval cache clear
```

The Phase 1 ephemeral implementation may use `--no-project-cache` to disable
reuse within one invocation. In Phase 5, the same flag also bypasses the
persistent cache:

```bash
forj atlas:eval run add-http-controller --no-project-cache
```

### Cache verification

Runner tests should prove that:

- identical preparation requests reuse one prepared base;
- a meaningful scenario, preparation, or renderer change produces a new key;
- two concurrent cache misses publish one complete entry;
- readers racing prune and clear retain a valid pinned entry;
- failed preparation cannot be observed as a cache hit;
- a modified entry fails full-tree verification;
- forged content plus forged unauthenticated metadata fails provenance
  verification;
- mutations in one trial do not affect the cached base or another trial;
- guidance from one profile does not leak into another profile;
- runtime `.env` overlays are trial-private and identical inputs yield an
  identical reusable base;
- cached and uncached named-resource fixtures generate identical accessors and
  driver imports from the same environment projection;
- links, special files, unsupported metadata, and transient leftovers are
  rejected or normalized before publication and materialization;
- an agent cannot write a shared dependency cache in authoritative mode;
- unconfined entries and keys cannot satisfy an authoritative hit;
- a cached Project and an uncached render produce equivalent starting trees
  after the same trial-private overlays;
  and
- cached materialization is materially faster than rendering and preparing the
  same scenario again under a predeclared benchmark protocol.

The benchmark records render, dependency preparation, health checks, hygiene,
digest, copy, overlay, agent, and verifier timings plus files and bytes. It
compares serial cold dependencies, warm dependencies without a Project cache,
and verified cache hits over at least ten repetitions. Persistent caching is
not enabled by default until both the smallest and largest core fixtures show a
material whole-run improvement.

## Guidance Profiles

Guidance profiles are the central experimental control.

### `none`

Expose no GoForj-specific agent instructions, Atlas skills, or Atlas MCP server.
The Project source and normal CLI remain available. This is a baseline, not an
expected production configuration. Because the prepared base is already
guidance-neutral, this overlay persists `render.agent_guidance: none`, selects
no native projection targets, and validates absence before the trusted baseline
is captured. If an unmanaged or uncertain instruction projection exists,
preparation fails rather than deleting user-authored data.

This profile does not erase a model's prior knowledge or provider-native system
instructions. It measures behavior without additional materialized GoForj
guidance, given normal Project source and CLI help.

### `placebo`

Expose a neutral instruction projection with similar length and placement to
the baseline guidance but no GoForj workflow content. This optional experimental
control helps distinguish framework guidance from the mere presence of a
project instruction file.

### `agents`

Persist `render.agent_guidance: baseline` and the declared native projection
targets, then expose only the baseline instructions rendered by GoForj through
the canonical Atlas composer. This profile proves that a new Project remains
usable even when Atlas installation, skills, and MCP configuration are absent.

### `skills`

Expose the baseline instructions and the scenario-selected Atlas skills. Do not
configure MCP. Atlas must implement a real skill allowlist before this profile
can claim scenario-level attribution; the current recommended-catalog writer is
not sufficiently selective. Until selected and absent paths, including
user-owned skills, are proven, the runner reports this profile unsupported and
excludes it from comparative results.

### `mcp`

Expose the baseline instructions and Atlas MCP, but no optional skill content.
This isolates the value of project-aware tools. Evaluation MCP must enforce a
server-side tool allowlist, use pinned static documentation and fixture
diagnostics, disable mutable docs refresh and environment overrides, and start
from a trusted absolute executable with an explicit trial root.

Until the supervisor-owned interposer has qualified allowlist enforcement and
absence tests, `mcp`, `recommended-hermetic`, and custom tool slices are
unsupported treatments rather than partially trusted comparisons.

### `recommended-hermetic`

Expose a hermetic projection of the recommended Atlas installation: baseline
instructions, project-selected skills, and MCP configuration. The
evaluation-specific skill and MCP allowlists, pinned docs, trusted executable,
environment, state, and network restrictions still apply. The name
distinguishes this attributable, safe treatment from an unrestricted installed
Project.

A separate production-parity contract should prove that its guidance content,
skill selection, tool schemas, and version-selection behavior match a normal
recommended installation except for enumerated unsafe capabilities. A future
production-shaped replay is observational rather than authoritative unless it
meets the same isolation contract.

### Custom slices

After skill and MCP allowlists exist, maintainers should be able to name exact
capabilities:

```bash
forj atlas:eval run add-http-controller \
  --guidance agents,skill:goforj-add-http-route,tool:workflow-plan
```

The run manifest must record the resolved files, skill versions, MCP tools, and
documentation reference. It must also prove unselected files and tools are
absent. A profile name alone is insufficient evidence because its contents may
change over time.

## Isolation Contract

Isolation can control runtime exposure, not model pretraining or provider-native
system behavior. The measured treatment is the incremental effect of the
selected materialized guidance given the recorded agent, model, Project source,
CLI, toolchain, and provider behavior.

The runner should support explicit execution backends:

- `unconfined-local`: process-based development diagnostics with no security or
  release-evidence claim;
- `sandboxed-local`: platform-specific filesystem, process, and network
  enforcement with reported capabilities; and
- `container-ci`: the authoritative CI and release backend using an isolated
  container or VM boundary.

Every command also selects an execution intent:

- `diagnostic` may continue with an unconfined or partially capable backend,
  but marks every affected outcome and conformance endpoint `ineligible`; and
- `authoritative` requires the complete promoted capability set and rejects
  the run before agent startup when the selected backend cannot enforce it.

Scenarios declare endpoint capabilities rather than naming a sandbox tier.
Backend and intent are runner policy, remain visible in the run manifest and
terminal report, and cannot be weakened by a candidate manifest. A diagnostic
run may report deterministic artifact-check results, but those results never
enter an authoritative outcome, conformance, comparative, holdout, or release
denominator.

Each run should receive:

- a fresh temporary Project directory;
- a fresh allowlisted process environment;
- an evaluation-specific agent home and configuration directory;
- no parent conversation or session resume token;
- no user-global instruction files;
- no user-global skills, plugins, or MCP configuration;
- no raw provider credential visible to the agent process or agent-issued
  children;
- an explicit network policy;
- bounded CPU, memory, PID, disk, file-size, descriptor, time, and output use;
  and
- no Git remote with write credentials.

Authoritative backends should use a dedicated identity, read-only host
filesystem, private temporary storage, no host sockets or devices, no inherited
file descriptors, and explicit mounts. The environment allowlist must exclude
Git credential helpers, SSH agents, cloud credentials, Docker and Kubernetes
configuration, host XDG paths, and unrelated service credentials.

Supervisor launches use structured argument vectors and explicit standard
input. Prompts, paths, profiles, candidate metadata, and scenario values are
never interpolated into shell commands, action identifiers, cache keys, or
artifact destinations.

The trusted supervisor, scenario contract, verifier, redactor, baseline,
capture stream, and report writer must remain outside the agent-writable
namespace. Candidate code and agent-produced source are untrusted. The
supervisor should independently walk and hash the final tree; it must not trust
agent-writable Git metadata or agent-reported events as the only evidence.

Changing the working directory, `HOME`, or environment variables alone is not a
security boundary. An `unconfined-local` result must say so prominently and
cannot satisfy authoritative isolation gates.

### Network modes

Shell network modes should include:

- `off`: no shell network;
- `docs-only`: pinned static docs or an enforced exact-destination proxy with
  redirect and DNS-rebinding protection; and
- `normal`: ordinary agent shell access for a realistic but non-hermetic local
  run.

Provider API transport may require network access even when the agent's shell
network is disabled. Authoritative backends must use a supervisor-owned
credential broker or equivalent separation so the raw provider token is never
available through environment, files, descriptors, process inspection, or the
broker protocol. An adapter that cannot preserve this boundary is ineligible
for authoritative execution.

The broker is not a general authenticated proxy. Each unguessable session is
bound to one trial and authenticated to the trusted adapter. Policy fixes the
provider origin, tenant, model, method and path, request schema, response size,
turn, token, rate, and spend budgets. The broker injects authorization
server-side and rejects caller-supplied authorization, redirects, `CONNECT`,
cross-trial replay, and persistent or administrative APIs such as files,
batches, fine-tuning, stored conversations, and account management. Version 1
permits no exception. A
future external-mutation capability requires a disposable tenant or resource
namespace, independently bounded credentials, cleanup verification, and an
outcome that cannot claim no external mutation. Redacted broker audit events
remain outside the sandbox. Negative tests cover those authority boundaries as
well as raw credential recovery. `normal` cannot prove that external mutation
did not occur and is ineligible for that gate.

Broker secrecy covers delegated authority as well as raw credential text. The
trusted adapter should frame provider calls outside the sandbox. If a reusable
broker capability must cross the boundary, it must be bound to a
non-inheritable OS identity or process label and unavailable to every
agent-issued descendant. Qualification attempts endpoint discovery,
inherited-descriptor reuse, process-filesystem recovery, copied-client use, and
independently constructed requests. Proving only that the token is unreadable
is insufficient.

### Secrets and external mutation

The agent process should receive the smallest credential surface possible.
Provider credentials must not be copied into scenario artifacts. The run must
not inherit GitHub, cloud, database, or deployment credentials from the
maintainer's environment. Use short-lived, narrowly budgeted provider
credentials where the provider supports them.

Every agent-readable password, signing key, token, or service capability used
by a fixture must be synthetic, unique to one trial, restricted to that
trial's private endpoint, and revoked during cleanup. Deterministic fixture
inputs may select drivers and generation shape, but no real, reusable, or
cross-trial credential may enter a prepared base, trial overlay, prompt,
guidance projection, or verifier fixture.

Prevention is primary; redaction is defense in depth. Artifacts should record
environment variable names and non-sensitive classifications, not raw values.
Before upload, scan all retained files for known secret canaries and values and
fail closed. Destroy isolated homes, writable caches, temporary files, raw
buffers, and crash artifacts according to a declared retention policy.

The scenario should also declare a source-data classification and approved
provider data-handling class. Preflight should record the provider endpoint or
tenant, server-side retention policy, training policy, and available telemetry
controls. Disable provider-native telemetry and local session logs where
supported. A provider policy incompatible with private candidate source fails
preflight.

## Agent Adapter Contract

Each agent adapter should know how to:

- resolve and verify an absolute agent executable outside the trial;
- report supported observation and isolation capabilities;
- create isolated configuration and home directories;
- install the selected native instruction projection;
- install the selected skills;
- configure only the selected MCP servers;
- start a new non-interactive session;
- provide the prompt;
- apply time and command limits;
- capture structured tool and command events where supported;
- capture stdout and stderr as a fallback;
- determine process completion; and
- report the exact agent and model identity;
- terminate the complete sandbox or platform job on cancellation; and
- clean up temporary homes, MCP children, listeners, and partial preparation.

Suggested interface:

```go
type EvaluationAgent interface {
	Name() string
	Capabilities(context.Context) (AgentCapabilities, error)
	Prepare(context.Context, RunEnvironment, Guidance) (AgentPreparation, error)
	Start(context.Context, PreparedAgent) (EvaluationSession, error)
}

type AgentPreparation interface {
	Agent() PreparedAgent
	Close(context.Context) error
}

type EvaluationSession interface {
	Turn(context.Context, AgentTurn) (AgentTurnResult, error)
	Wait(context.Context) (AgentResult, error)
	Close(context.Context) error
}
```

`Prepare` must return a non-nil cleanup handle whenever it acquires resources,
even when it also returns an error. The preparation should otherwise remain
transactional and clean its own partial state. `Close` must be idempotent and
run under a fresh bounded cleanup context rather than an already-cancelled run
context.

`EvaluationSession` supports a one-turn prompt without requiring conversation
resume and permits bounded scripted clarification when the scenario declares
it. The supervisor owns turn selection and never lets candidate code redefine
the hidden user-response script. Session cleanup follows the same non-nil,
idempotent, fresh-context rules as preparation cleanup.

Authoritative cleanup destroys the supervisor-owned container, VM, or platform
job on every terminal path and verifies that no child, listener, mount, or
writable cache survives. Results retain separate `agent_outcome` and
`evaluation_status` fields plus structured endpoint results and secondary
failures. Cleanup or capture failure sets the evaluation status to
`evaluator_error` without
overwriting what the agent did.

The common evaluation model must not depend on one provider's private event
schema. Adapters should normalize observable events such as:

```text
file_read
file_write
command_started
command_finished
mcp_tool_called
message
run_finished
```

Behavioral gates require trustworthy observation. Command evidence should come
from the sandbox or supervisor boundary and include the resolved executable
digest, arguments, working directory, timestamps, and exit status. Provider
events may enrich that record but are not sufficient by themselves when the
agent can tamper with or omit them.

Trusted MCP evidence requires an authenticated server-side event stream to the
supervisor. Each event records the MCP server executable and configuration
digest, tool name, canonical argument digest, sequence, completion status, and
result classification. Qualification covers fake servers, restarts, stdio
tampering, dropped events, and calls that start without completing. Client or
provider claims alone cannot satisfy an MCP gate.

Candidate MCP code cannot attest to itself. A supervisor-owned protocol
interposer outside the candidate process enforces the promoted tool allowlist
and observes and binds request, response, effective process identity,
configuration digest, completion, and sequence. It filters tool discovery and
rejects unselected requests, responses, notifications, and registrations after
server restart. Candidate registration is data, never authorization. An
allowlist discrepancy invalidates the trial. A trusted released MCP may
additionally sign events, but candidate-emitted events remain untrusted data.
The transport may be a stdio shim, socket proxy, or other qualified mechanism;
the independent enforcement and evidence boundary is normative.

The first authoritative backend should target Linux and use a qualified,
supervisor-owned execution and filesystem observation facility. `ptrace`,
seccomp user notification, a runtime audit facility, or another mechanism is an
implementation choice; the observable contract is normative.

The sandbox defines named compartments with explicit read, write, and execute
rules: Project, private home and configuration, private temporary and build
state, private dependency caches, immutable toolchain and runtime, and declared
fixture endpoints. Tracing starts before the first agent instruction and is
complete for every event class used by an evaluation predicate and every
policy-relevant compartment crossing. Namespace or MAC enforcement may prove
other containment without duplicating every ordinary runtime read in the event
stream.

Each backend qualification profile declares how it tracks fork/clone ancestry,
cwd and root changes, descriptor inheritance, executable operations,
fd-relative file operations, create/truncate, rename, unlink, links, read and
file-backed mapping activity, and background descendants. Unsupported or
unmodeled mechanisms are denied.
Inspection predicates state whether a successful open, byte read, or content
digest constitutes inspection. Events use canonical sandbox-relative paths and
trusted process provenance. Loss, overflow, ambiguity, or late events make the
affected predicate ineligible or the evaluation invalid. A `PATH` wrapper alone
is not sufficient because the agent can use an absolute path or copy an
executable.

Trusted execution identity covers the effective immutable execution closure,
not only one pathname digest. The executable, interpreter, loader, relevant
libraries, and toolchain image are bound at execution time; loader overrides,
process injection, and replacement between inspection and execution are
unavailable. A backend may satisfy this through a read-only image, static
binary, descriptor or inode binding, runtime attestation, or an equivalent
qualified mechanism.

Qualification tests should cover nested shells, absolute paths, copied
binaries, failed execs, signals, timeouts, background descendants, inherited
descriptors, mmap mutation, temp-file rename replacement, link escape attempts,
and attempted monitor evasion. Other operating systems may support
authoritative evaluation only after an adapter selects and qualifies an
equivalent facility.

Before a trial begins, compare the scenario's required observation classes with
the adapter and backend capability set. If a required class is unavailable, the
trial is ineligible for that behavioral endpoint and should fail preflight
rather than infer evidence from prose. Artifact-only results remain useful but
must be reported as a separate outcome. An explicit local diagnostic mode may
continue to collect those artifacts after reporting the missing endpoints; it
cannot emit a behavioral pass or enter an authoritative denominator.

## Verification Model

Verification has four layers.

### 1. Environment verification

Before scoring the agent, prove that:

- the fixture matches its referenced GoForj scenario and preparation request;
- the intended `AGENTS.md` or native projection is present;
- unselected guidance is absent;
- the requested Atlas MCP server is reachable when selected; and
- the Project satisfies its versioned starting-state health contract.

That health contract should include:

- a baseline tree digest covering every materialized entry;
- deterministic seed-data identity;
- the expected successful preparation commands; or
- for a seeded defect, the exact failing command and bounded error predicate,
  proof that no collateral failure is present, and a known-good repair oracle.

Cached and uncached preparation must satisfy the same contract. Scenarios that
measure first-run dependency behavior must declare that purpose and bypass
prepared dependency state.

An invalid fixture is an evaluation infrastructure failure, not an agent
failure.

### 2. Behavioral verification

Use supervisor-observed execution evidence to check actions such as:

- inspecting sufficient Project evidence, which may be `.goforj.yml`, the
  corresponding Atlas Project tool, or another profile-approved source;
- invoking the matching `forj make:*` command;
- using the correct additional-App prefix;
- calling `workflow-plan` or another required Atlas tool;
- consulting version-aware documentation when needed;
- avoiding prohibited commands; and
- running appropriate validation.

Do not infer generator usage solely from the final file shape.

### 3. Artifact verification

Inspect the final Project through trusted deterministic code:

- a supervisor-computed diff against the immutable baseline;
- Go AST and type information;
- generated-file ownership rules;
- route, command, job, schedule, and resource registrations;
- Wire provider sets and generated parity;
- migration and model placement;
- scenario-declared application, domain, transport, and persistence boundaries;
- formatter output;
- build, test, vet, and relevant GoForj inspection commands; and
- regeneration stability.

Tests must cover all relevant nested Go modules in the rendered Project.

Execute builds, tests, vet, generation, and other trial-controlled code in a
fresh sandbox with no provider, Git, cloud, or host credentials and network
`off` by default. Before snapshotting, the supervisor stops provider
interaction, terminates or freezes the sandbox job, proves no descendants or
listeners remain, and seals or copies the final tree. Late mutations or events
invalidate the evaluation.

Every executable verification phase receives a separate disposable clone of
that sealed tree. Verifier implementations, hidden expected answers, and future
cases remain outside the candidate-readable namespace. A supervisor-side
black-box harness drives the application or a bounded fixture endpoint and
reveals only the current minimal request. Structural checks also remain
supervisor-side. Only private build and temporary state is writable. The
supervisor discards verifier writes, kills all descendants after each phase,
and receives results over a supervisor-controlled channel.

When a language-specific linked or mounted test necessarily reveals its oracle,
that invariant is explicitly non-adversarial and cannot support a claim that it
resists candidate gaming. A Project's own successful `go test` cannot replace
the hidden trusted black-box oracle.

A scenario requiring a service should receive a disposable unauthenticated
fixture endpoint with bounded protocol and path semantics and no retained
request data; `docs-only` access should not carry into executable verification.
Use trusted absolute tool paths and a sanitized `PATH`. Run non-executing
structural checks first. Never load verifier implementations, hooks, plugins,
filters, or tool configuration from the agent-writable tree as trusted code.

### Verifier calibration

Every scenario verifier should include known-good variants and deliberate
mutants. Each named invariant must reject its intended mutant without rejecting
acceptable implementation variation.

Each verifier version owns a calibration registry. Every required invariant
records its positive implementation variants, targeted negative mutants,
framework/version applicability, owner, review trigger, and false-positive or
false-negative incidents from live triage. No required gate ships without at
least one independently authored positive variant and one targeted negative
mutant. Framework, generator, or verifier releases rerun the complete registry.

The controller scenario, for example, should calibrate against:

- a correct hand-built controller that passes the outcome endpoint but fails
  required generator conformance;
- a route registered in the wrong App;
- a manual `wire_gen.go` edit;
- a route that compiles but returns the wrong entity or status;
- repository access placed directly in the controller;
- a generator invocation followed by bypassing its output; and
- an unrelated Project diff.

Generator-specific mutants additionally cover the wrong resource, wrong App,
failed generation followed by a manual implementation, generation after an
earlier protected write, deleted or replaced output, and shell, copied-binary,
or absolute-path invocation.

Black-box behavior, such as a seeded request and persistence assertion, should
complement AST and build checks. Verifier mutation tests run without a live
model and belong in ordinary CI.

At least two valid implementations for each core target should differ in
nonessential names or structure, such as an existing domain service versus a
use-case or query boundary. They prove the oracle recognizes framework and
behavioral invariants rather than one golden implementation.

### 4. Qualitative review

An optional reviewer may assess properties that are difficult to encode
initially, such as whether a form is usable or an explanation is clear.

Qualitative review must:

- use a versioned rubric;
- receive only redacted artifacts;
- report its own model identity;
- remain separate from deterministic scores; and
- never override a failed required invariant.

Repeated qualitative failures should be converted into deterministic checks
where practical.

## Scoring

Scenarios should distinguish framework outcomes, workflow conformance, and
scored quality signals.

Framework-outcome gates include:

- correct owning App;
- no prohibited generated-file edit;
- required observable behavior and registrations;
- successful build;
- successful scenario tests; and
- no out-of-scope external mutation when the backend can enforce and observe
  that property.

Workflow-conformance gates include only requirements explicitly declared for
the target, such as:

- matching successful generator invocation;
- correct additional-App prefix;
- required Project or documentation inspection;
- prohibited manual ownership changes; and
- proportionate validation.

A failed outcome gate fails `framework_outcome_pass`; a failed workflow gate
fails `workflow_conformance_pass`. `contract_pass` may require both but reports
the two results independently. A trial that lacks a required observation is
ineligible for that endpoint rather than silently passing. Unconfined runs
cannot claim gates their backend cannot enforce.

Quality signals can include:

- inspected sufficient Project evidence before editing;
- used selected Atlas tools when they materially helped the task;
- kept transport code thin;
- preserved established application, domain, and persistence boundaries;
- propagated cancellation;
- added focused tests;
- avoided unrelated changes;
- selected proportionate validation; and
- completed within reasonable command and time budgets.

Reports should expose raw counts rather than only one composite number.

A quality signal may become a required gate only after maintainers identify the
framework or scenario contract it protects, calibrate multiple valid variants
and targeted mutants, and review observed false-positive and false-negative
rates. Preference or style alone is never promoted into a hard invariant.

Across repeated trials, track:

- framework-outcome pass rate;
- workflow-conformance pass rate;
- combined contract pass rate when declared;
- generator compliance rate;
- correct-App selection rate;
- first-pass build and test rates;
- prohibited-file edit rate;
- unrelated-diff rate;
- clarification rate;
- median duration;
- median model tokens when available; and
- median command and tool-call counts.

### Trial outcomes and denominators

The trusted lifecycle state machine records these monotonic milestones:

```text
preflight
  -> provider_session_started
  -> prompt_delivered
  -> first_agent_action
  -> agent_terminal
  -> evaluation_terminal
```

The first action is any trusted read, write, command, MCP call, or final agent
response. Retry eligibility is determined from these recorded milestones, not
from a subjective judgment of “meaningful work.” A timeout or provider failure
before `first_agent_action` may be retried under policy; after it, the logical
trial is complete and is not retried to obtain a pass.

Every attempt records one `agent_outcome`:

- `not_started`: preflight or fixture preparation ended before a provider
  session;
- `completed`: the agent reached a terminal response or process exit;
- `abstained`: a safe-abstention scenario ended with an accepted clarification
  and no prohibited mutation;
- `provider_error`: the provider disconnected or returned an invalid response,
  with the reached milestone retained;
- `adapter_error`: the agent adapter failed independently of the provider, with
  the reached milestone retained;
- `timeout`: the attempt exceeded its budget, with the reached milestone
  retained; or
- `cancelled`: an operator stopped the run.

It also records one `evaluation_status`: `valid`, `valid_abstention`,
`not_evaluated`, `ineligible`, `fixture_error`, or `evaluator_error`, plus
structured capture, verification, and cleanup failures. Valid evaluations carry
separate `framework_outcome`, `workflow_conformance`, and optional combined
`contract` endpoint results.

A provider, adapter, cancellation, or timeout failure before the first action
is `not_evaluated`. After the first action it is non-retryable: complete trusted
evidence may produce failed endpoints, while incomplete evidence produces
`evaluator_error`. Agent metrics use only evidence-valid logical trials.
Feature-completion rates use framework-outcome passes and failures; accepted
abstentions are reported as a separate safe-abstention metric and may be
aggregated only across scenarios whose endpoint explicitly permits abstention.
Reports show every excluded state and milestone separately.

Distinguish an attempted-trial ID from its logical-trial ID. Retry only bounded
`provider_error`, `adapter_error`, timeout, or equivalent infrastructure failure before
`first_agent_action` under a versioned policy. Retries remain visible as
attempts and replace the incomplete attempt within the same logical trial.
Reports include both attempt and logical-trial counts so provider instability
cannot disappear from the scorecard. Tests cover provider failure and stall,
timeout and provider or adapter failure before and after a read, write, or
command, cancellation, agent exit followed by verifier failure, and cleanup
failure layered on every agent outcome.

## Initial Scenario Suite

The first suite should stay small and cover the highest-leverage framework
decisions.

Before implementing the Atlas side, each live evaluation must name its exact
GoForj scenario, workflow, and verifier IDs. The current catalog does not yet
cover every entry below; missing topology, resource-driver, seeded-defect, and
preparation-boundary support belongs to the GoForj scenario contract first.

### `add-http-controller`

Prompt for an invoice-by-ID endpoint in a fixture that already establishes a
repository-backed invoice domain. Require the controller generator, thin
transport behavior, correct use of the existing application boundary, correct
registration, and focused tests. Accept calibrated service, use-case, query, or
domain-handler shapes that preserve those boundaries.

### `add-app-command`

Prompt for a billing-report synchronization command. Require the command
generator, correct command collection registration, invocation of the owning
application behavior, and appropriate context propagation.

### `add-job`

Prompt for a receipt job carrying an order ID. Require the job generator, typed
payload conventions, handler placement, invocation of the owning application
behavior, and job tests.

### `add-schedule`

Prompt for hourly stale-session cleanup. Require the schedule generator and the
framework's actual schedule registration shape rather than an invented one.

### `create-model`

Seed a database table and ask for application access to it. Require
`make:model`, repository ownership, and correct connection selection.

### `add-named-app-route`

Render `app` and `admin`, then ask for an admin audit endpoint. Require the
additional-App command prefix and prohibit edits to the default App's route
registry.

### `add-named-resource`

Ask for a named queue or storage resource. Require generator/config behavior,
named accessor usage, and correct dependency injection.

### `repair-wire-provider`

Seed a missing provider and ask the agent to fix the build. Require inspection
of provider sets and prohibit manual edits to `wire_gen.go`.

### `unknown-framework-shape`

Treat this as a separate safe-abstention scenario rather than ordinary feature
completion. Require bounded evidence gathering, no unsupported framework API,
no out-of-scope mutation, and either a specific clarification question or an
evidence-backed direct implementation permitted by the scenario oracle.

In noninteractive mode, accepted clarification is the terminal `abstained`
state. The scenario must define a machine-checkable clarification schema and
require trusted final-response capture. After the agent emits that response it
must exit successfully without waiting for a user turn. The verifier checks the
evidence-gathering budget and independently confirms the allowed no-mutation
scope.

### Scripted clarification

Safe abstention proves that an agent does not invent an answer, but it does not
measure collaboration. A separate scripted multi-turn scenario defines hidden
user intent, accepted decision-relevant clarification fields, a deterministic
user response, and a strict turn budget. The verifier checks clarification
precision, whether the response was incorporated, whether settled questions
were repeated, and whether the final application satisfies the ordinary
outcome and conformance endpoints.

## Scenario Portfolio Governance

The initial primitive suite is a conformance foundation, not a representative
sample of application development. Before comparative or release claims, the
portfolio should cover at least these task families:

- new scaffolding;
- extending an existing feature and its conventions;
- seeded runtime diagnosis from incomplete evidence;
- behavior spanning multiple components or runtimes;
- behavior-preserving refactoring;
- Project-version and documentation mismatch;
- frontend or full-stack integration;
- ambiguity and clarification; and
- framework ergonomics defects that should not be worked around silently.

The scenario registry records framework area, App topology, starting-state
complexity, task family, prompt style, source provenance, and whether the case
is `representative`, `holdout`, or `regression`. Prompt families should include
ticket-like, terse, bug-report, and constraint-heavy variants. Guidance authors
must not inspect holdout prompts or fixtures. Authoritative holdouts live in a
versioned protected evaluator input store, are mounted only into the trusted
control plane, and expose only their prompt and minimal runtime fixtures to the
agent. Reports record the holdout-set version and digest without publishing its
hidden oracle. Comparative reports macro-average by scenario family so one easy
or high-volume task cannot dominate.

Oracle secrecy is independent of source classification. A holdout verifier may
emit only promoted invariant IDs, endpoint status, and explicitly declassified,
bounded diagnostics. Expected values, hidden requests, mutant identities,
verifier source locations, panics, and stack traces remain in a separately
access-controlled oracle bundle, if policy permits retaining them at all.
Repeated failures must not reveal enough information to reconstruct a holdout.

Real issues, transcripts, and support questions are preferred scenario sources
after redaction. A historical regression remains valuable but cannot silently
redefine the representative benchmark; it carries recurrence, review, and
retirement metadata.

## Comparative Guidance Experiments

The evaluation system should measure the incremental effect associated with a
guidance profile under one recorded agent, model, projection, Project,
toolchain, provider environment, and time window. It cannot erase model prior
knowledge or justify universal claims from a small sample.

Three results should remain distinct:

- framework-outcome runs ask whether the resulting application is correct;
- conformance runs ask whether the agent followed the declared GoForj workflow;
- comparative runs ask whether one materialized guidance treatment changes the
  repeated-trial outcome relative to another treatment.

For important scenarios, use the same:

- GoForj scenario and prepared Project base;
- prompt;
- agent and model version;
- limits;
- verifier; and
- trial count.

Change only the guidance overlay. Treat `(provider adapter, native projection,
guidance profile)` as the treatment. Establish effects within one provider
before attempting cross-provider comparisons, and compare providers only over
a declared common-capability subset.

A versioned experiment protocol should predeclare:

- macro-averaged scenario-family `framework_outcome_pass` as the default
  primary endpoint and workflow conformance as a secondary endpoint;
- scenario-family weights and the primary aggregation unit;
- minimum valid trial count or stopping rule;
- randomized and interleaved profile order within time blocks;
- placebo use where applicable;
- provider and infrastructure retry rules;
- missing-data, time-block drift, and exclusion handling;
- confidence-interval method;
- multiplicity or family-wise decision rules across profiles and metrics;
- minimum detectable effect or non-inferiority margin;
- whether each claim uses superiority or non-inferiority; and
- the exact rule that alerts or blocks a release.

An example report:

```text
add-http-controller  ·  Codex  ·  10 trials

Guidance       Outcome   Workflow   Valid/Assigned   Excluded
none              3/10       2/10            10/10          0
agents              8/9        8/9             9/10          1 provider
skills              9/9        9/9             9/10          1 capture
recommended       10/10      10/10            10/10          0

agents - none outcome effect: +58 percentage points [interval shown here]
```

Model sampling controls should be recorded when a provider exposes them, but
the system should not claim perfect replayability from a seed. Comparative
reports should include effect sizes and uncertainty alongside raw counts. They
also show an intent-to-run operational success rate and complete exclusion
waterfall per treatment so provider, eligibility, capture, or setup instability
cannot disappear behind the evidence-valid endpoint. Three- or five-trial CLI
examples are diagnostics, not evidence of treatment effect.

## Artifacts And Redaction

Reports should have three deliberate layers:

1. Human summary: user request, observed result, failed outcome or conformance
   contract, user impact, likely product cause and owner, and recommended next
   action.
2. Diagnostic evidence: expected versus actual behavior, focused diff excerpt,
   command and tool timeline, and the available reconstruction, verifier-replay,
   or live-rerun command with its semantics labeled.
3. Machine detail: lifecycle milestones, attempts, evidence classes, hashes,
   exclusions, endpoint results, and denominators.

Terminal output uses plain phrases such as “Agent did not start,” “Could not
evaluate safely,” or “Implementation failed.” Internal enum names remain
available in details and JSON rather than becoming the primary user experience.

Each run should produce an attempt-ID-addressed artifact directory created
exclusively by the supervisor:

```text
manifest.json
run.json
scorecard.json
summary.txt
events.jsonl
transcript.redacted.txt
commands.jsonl
diff.patch
verification.json
environment.json
triage.json
```

Every failure records a triage state. `triage.json` gains a reviewed product
disposition only after confirmation; before that it records `unreviewed` or
`needs-evidence`, suspected causes, confidence, and the evidence needed next.
Automated output must not present a suspected cause as an established one.

The artifact directory must live outside the agent-writable namespace and use
private permissions. Human-readable timestamps and scenario names may appear
in metadata but never serve as the unique directory key. `manifest.json` is
supervisor-authenticated and covers every retained file, its classification,
the execution-plan digest, baseline and final-tree digests, and all promoted
contract, verifier, toolchain, and fixture identities. The supervisor should
write append-only event and command capture directly; the agent must not be
able to replace, truncate, or delete evaluation evidence. Streaming redaction
must occur before the first disk write, including append-only capture.

The product distinguishes three operations:

- report renders retained inert evidence without executing it;
- verifier replay reconstructs or opens a sealed Project and reruns only the
  authenticated promoted deterministic verifier in a sandbox; and
- live rerun starts a fresh stochastic agent attempt and is never described as
  replaying the original model behavior.

Verifier replay never executes commands, verifier code, paths, or configuration
supplied by an artifact. It resolves the promoted contract by authenticated
identity and verifies all manifest and tree digests first. A replayable run
either retains policy-compatible sealed baseline and final-tree bundles, or an
immutable retrievable baseline identity, patch, reconstruction manifest, and
pinned toolchain identities. If neither is retained, the report promises only
diagnosis, not later reproduction.

Artifacts should record:

- scenario and verifier version;
- GoForj and Atlas commit or release;
- resolved guidance files and hashes;
- agent executable version;
- model identity;
- network mode;
- start and end times;
- lifecycle milestones, agent outcome, evaluation status, endpoint results,
  and secondary failures;
- source and artifact classification; and
- every unavailable observation.

Redaction must occur before persistence. It should cover provider credentials,
environment values, connection strings, authorization headers, cookies,
tokens, and scenario-specific sensitive fixtures. Raw unredacted transcripts
should not be uploaded as CI artifacts.

Artifact walkers consume contents only from bounded regular files. They record
or reject links and unsupported entry types without opening them, reject mount
or reparse crossings, and apply per-file and total size and processing-time
limits so a FIFO, socket, device, or oversized file cannot block collection.

All human renderers treat candidate and agent content as inert data. Typed
serializers escape or remove ANSI, OSC, bidi, and other control characters,
sanitize Markdown and HTML, canonicalize displayed paths, and never interpret
captured content as commands or markup. Raw semantics remain only in safely
encoded machine fields.

Redaction alone is not sufficient. Scan agent homes, provider-created files,
diffs, transcripts, crash output, and retained artifacts for known credential
values and canaries before upload. Qualitative reviewers should receive inert,
delimited content with no tools, credentials, or external-mutation capability.

Scenario source classification propagates to every artifact, destination, and
qualitative reviewer. CI uploads only minimized scorecards by default. Full
diffs and transcripts require an explicit compatible policy covering
repository visibility, reader ACLs, encryption, maximum lifetime, deletion
enforcement, and reviewer/provider eligibility. The run fails closed when an
artifact store cannot meet the classification. Failed artifacts may be kept
longer than passing artifacts only within those bounds, and deletion should be
verifiable.

## CI And Release Use

Live-agent evaluations should be opt-in because they require credentials, incur
cost, take longer, and may be nondeterministic.

### Pull requests

Provider-backed evaluation must never execute arbitrary pull-request code with
trusted credentials. Do not use `pull_request_target` to run a candidate
renderer, verifier, guidance, MCP server, or preparation command.

Relevant changes may request a smoke evaluation only through a protected
workflow that:

- runs from trusted default-branch supervisor, verifier, redactor, and uploader
  code;
- authorizes an immutable repository and commit SHA;
- acquires candidate code in a trusted passive fetch step with exact-origin
  egress, no candidate execution, and no persisted credential;
- disables candidate hooks, filters, submodules, LFS smudging, and credential
  helpers unless separately pinned and required;
- strips `.git` and hands the sandbox a filesystem-policy-validated archive or
  copy;
- uses a protected environment and short-lived provider credential;
- grants GitHub only read access;
- pins actions and tools by immutable revision; and
- runs all candidate preparation, agent work, and executable verification
  inside the authoritative sandbox.

Protected evaluation consumes a trusted, versioned execution plan. Candidate
code may supply the renderer, guidance, MCP implementation, or product under
test, but cannot change promoted evaluation manifests, limits, verifier selection,
observation requirements, tool allowlists, retry policy, or the pass predicate.
Candidate versions of those contracts run only as non-gating self-checks until
reviewed and promoted. Tests attempt to lower limits, remove gates, replace a
verifier, alter an allowlist, and relabel a required observation.

The workflow restricts authorized actors and forks, binds approvals and budgets
to the immutable SHA, caps trials and spend per pull request, and prevents
candidate strings from controlling shells, paths, actions, caches, or artifact
settings.

A small trusted smoke suite may contain:

```text
2 agents or one primary agent
3 to 5 core scenarios
1 to 3 trials
```

Trusted baseline verifiers may authoritatively evaluate candidate framework,
guidance, or product behavior. A candidate verifier cannot authoritatively
approve itself: its mutation tests and self-checks run sandboxed as a separate
non-gating signal, and it becomes part of authoritative evaluation only after
merge or explicit trusted promotion.

Deterministic Atlas workflow tests remain mandatory on every pull request.

### Scheduled evaluation

A scheduled workflow should run the broader matrix and publish a comparative
report. Scheduled runs should detect model behavior changes even when Atlas and
GoForj code did not change.

### Release readiness

Atlas and GoForj releases that materially change agent guidance should compare
the candidate against the previous released baseline. Release policy should be
based on independently reported framework-outcome and workflow-conformance
regressions, macro-averaged by representative scenario family, not one failed
stochastic trial or one pooled composite.

Initial release thresholds should be observed before being enforced. Once the
suite is stable, reasonable gates include:

- no regression in prohibited-file edits;
- no regression in correct-App selection;
- no deterministic build or fixture failures;
- workflow conformance above the predeclared threshold; and
- no drop beyond the predeclared regression margin and confidence rule.

The automation phase may publish advisory reports before this protocol is
fixed, but it must not introduce an enforcing release gate until the scenario
set, baseline version, minimum valid trials or stopping rule, interval method,
regression margin, and alert-versus-block decision are versioned and approved.

## Generated `AGENTS.md` Contract

GoForj should render the baseline `AGENTS.md` from its durable guidance setting,
independently of Atlas installation. That file should be treated as a product
surface with its own evaluation profile.

The reusable canonical baseline composer should remain in the Atlas library,
which GoForj already imports. GoForj owns the product decision to select and
render that baseline during Project creation and render; Atlas owns the reusable
content API plus optional skill and MCP enrichment. Atlas must not import
GoForj, avoiding a dependency cycle, and the implementation must not maintain
two manually synchronized copies.

GoForj needs one durable, versioned source of truth for baseline guidance,
separate from whether Atlas skills or MCP are installed. The Project setting
is:

```yaml
render:
  agent_guidance: baseline # baseline | none
```

GoForj is the sole writer of the managed baseline projection during creation
and render. `.goforj/atlas.json` remains the source of truth only for optional
Atlas agents, skills, and MCP features. Atlas update options are tri-state:
omitted values preserve stored selection, while explicit enable or disable
values change it. An all-omitted update must never normalize to every surface
enabled.

Phase 1 transfers existing guideline writes for Codex, Claude, Cursor, and
Copilot to this ownership model. Atlas installation and update stop writing
native instruction files directly; they return a versioned reconciliation
result that GoForj persists and renders through the canonical writer. Managed
marker migration and native projection composition land for all existing
targets together so the transfer does not regress non-Codex users. Phase 8 is
reserved for additional live-agent adapters and new projection targets.

The committed Atlas `agents` selection is also the durable list of native
baseline projection targets even when skills and MCP are disabled. GoForj must
not redetect agents during render because the result would vary by machine. If
legacy migration finds no target selection, it persists the stable Codex
fallback and maintains only `AGENTS.md` until the user selects more agents.

Its core contract should include:

- inspect `.goforj.yml` and identify the owning App;
- run the matching `forj make:*` generator when the requested artifact is
  supported and should be registered in the selected App;
- inspect every generator-touched registration and Wire file;
- never manually edit `wire_gen.go`;
- keep transport and composition thin;
- place business behavior behind the owning application or domain boundary;
- keep persistence behind a repository or equivalent port when persistence is
  part of the feature;
- use the additional-App CLI prefix when applicable;
- prefer local configuration, generated source, and CLI evidence;
- use Atlas when available;
- consult version-aware GoForj documentation when Atlas is unavailable; and
- ask rather than invent a framework convention.

When an artifact intentionally should not be registered, guidance should first
inspect generator help and local Project conventions, then permit an advanced
manual implementation or ask for clarification. Scenario-specific generator
gates remain strict where the prompt and fixture require registration.

The `agents` guidance profile is the acceptance test for that contract. Atlas
being installed must not be required for those scenarios to pass.

Agents that do not consume `AGENTS.md` should receive a native instruction
projection generated from the same canonical source. Evaluations should name
the actual projection used so cross-agent results remain interpretable.

Migration must preserve user-authored content outside managed markers. For a
legacy Project with no durable setting, migration infers `baseline` only from
an existing Atlas-managed guidance block or stored `features.guidelines: true`;
otherwise it persists `none`. Existing Atlas-managed blocks upgrade in place,
while `.goforj/atlas.json` continues to describe optional Atlas installation
state. The `none` evaluation overlay removes only managed framework guidance
and native projections; it must never delete user-authored instructions.

Map existing wizard choices explicitly:

- Recommended renders the canonical baseline and installs selected Atlas skills
  and MCP configuration;
- Minimal renders the canonical baseline without skills or MCP;
- Custom renders the baseline only when guidelines were selected and installs
  skills or MCP independently, preserving Custom-without-guidelines; and
- Skip renders no agent guidance or Atlas integration.

Projects that previously used Atlas's marker-managed guidelines should migrate
that managed block in place without changing the surrounding file.

Create, render, Atlas update, and migration tests should cover all four wizard
modes, Custom selections with skills or MCP but no guidelines, omitted update
flags, Claude-only and Copilot-only projections, missing legacy config, render
on another machine, repeated rendering, and user content around managed
markers. A GoForj-only test with no Atlas installation state must prove that
`baseline` persists, renders, and survives rerender on another machine, while
`none` removes only GoForj-managed material.

Evaluation treatment tests start from the same prepared base, apply `none` and
`agents`, and prove identical non-guidance tree and health digests. They then
run render, build, and a representative generator in each private fixture and
prove that durable settings, selected managed projections, and absence of
unselected projections remain stable.

## Failure Classification

Reports should separate failures into:

- `fixture_error`: the starting Project or isolation environment was invalid;
- `provider_error`: the provider failed, with its lifecycle milestone retained;
- `adapter_error`: the agent adapter failed independently of the provider;
- `timeout`: the attempt exceeded its budget, with its last lifecycle milestone
  retained;
- `abstained`: a safe-abstention scenario completed with an accepted
  clarification and no prohibited mutation;
- `endpoint_failure`: a valid evaluation failed the framework-outcome,
  workflow-conformance, or both endpoints, reported independently with
  behavioral, artifact, and validation subcategories;
- `ineligible`: required observation or backend enforcement was unavailable;
  and
- `evaluator_error`: trusted capture or verification could not produce a
  trustworthy result.

Infrastructure failures should not be counted as model failures. Missing
observability should not silently pass a required behavioral assertion. These
failure classes populate the structured agent outcome, evaluation status, and
endpoint results defined above rather than replacing them with one lossy
status.

## Failure-To-Product Feedback Loop

Adding a regression is not itself a product fix. Every confirmed failure should
record:

- suspected cause and confidence;
- user impact;
- affected framework surface and version;
- recommended owner;
- linked issue, change, or decision;
- recurrence count; and
- review or retirement date.

The required disposition is one of: guidance discoverability, documentation,
CLI or generator ergonomics, framework/API defect, generated extension-point
defect, evaluator/verifier defect, agent behavior, or needs more evidence.
Maintainers should prefer improving the framework, generator, CLI help, or docs
when that would help humans and agents alike. New prompt text is not the default
response to a product ergonomics problem.

A regression links to this triage record and preserves the smallest behavior
that reproduces the problem. Representative and holdout benchmarks remain
separate, so historical defects do not inflate or distort the main task
population.

## Security Considerations

Running a coding agent is equivalent to running untrusted automation with shell
access. Authoritative evaluation therefore requires an OS-enforced sandbox. It
should:

- use a dedicated temporary root;
- use a dedicated identity and private temporary filesystem;
- keep the host, prepared base, trusted verifier, capture, baseline, and
  artifacts outside the writable namespace;
- strip unrelated secrets from the environment;
- expose no authenticated Git remote, host socket, device, or credential
  helper;
- enforce filesystem, process, resource, and network policies;
- use trusted absolute executable paths and a minimal fixed `PATH`;
- terminate and verify cleanup of the entire sandbox or platform job; and
- retain an audit trail of commands and external tools.

Local developers should not be required to use Docker for exploratory runs,
but a plain-process run is explicitly unconfined and cannot implement the same
contract. Negative sandbox tests should attempt forbidden host reads and
writes, credential access, cache mutation, network egress, external mutation,
and orphaned-process survival.

## Performance And Cost

Live evaluation cost should be visible. Reports should include wall time, model
usage when available, command count, retained artifact size, configured and
observed concurrency, and preparation-stage timings. Serial performance runs
provide duration baselines; randomized concurrent trials remain appropriate for
outcome comparisons but do not support precise latency claims.

The runner should support:

- scenario and suite selection;
- trial limits;
- maximum concurrent agents;
- per-provider rate limits;
- early termination after required failures when diagnostic value is complete;
- reuse of immutable rendered fixture bases through safe copies; and
- trial-private dependency caches, with verified tool-specific seeds only after
  qualification and measurement.

Parallel trials must never share a writable Project, agent home, Atlas state,
Git index, network namespace, fixture listener, or broker session. Each trial
uses a private network namespace containing only its declared endpoints. If a
backend cannot provide that boundary, fixture endpoints require unguessable,
trial-bound capabilities and cross-trial routing must be denied; otherwise the
runner executes those trials serially. Qualification attempts port discovery,
cross-trial connections and request injection, and broker-session reuse.

## Compatibility

The first implementation is additive.

- Normal Atlas and GoForj commands do not invoke live agents.
- Existing deterministic evaluation APIs remain supported.
- No agent provider becomes a required GoForj dependency.
- GoForj scenario specs and Atlas evaluation manifests include independent
  schema versions.
- Agent adapters should report observation capabilities before execution;
  scenarios requiring unavailable capabilities fail preflight as ineligible.
- Legacy GoForj scenarios remain usable by existing commands and external
  catalogs even when they do not support live preparation.
- GoForj and Atlas exchange versioned evaluation capabilities before Project
  mutation and fail clearly when the pinned library cannot satisfy a contract.
- GoForj Projects with `render.agent_guidance: none` remain valid; the runner
  may apply the `agents` overlay only to its private evaluation fixture.

## Implementation Phases

The first-slice ownership boundary is explicit:

- Scenario preparation: GoForj owns scenario specs, rendering, and
  framework validation. Atlas owns the trusted request, resolved plan, and
  lifecycle.
- Baseline guidance: GoForj persists selection and writes managed native
  projections. Atlas composes canonical content and returns reconciliation
  data.
- Workflow expectations: GoForj exposes executable identities and framework
  facts. Atlas owns typed requirements and conflict validation.
- Live execution: GoForj provides the thin `forj atlas:eval` command. Atlas
  owns adapters, isolation orchestration, capture, and trial state.
- Results: GoForj exposes the product command and Project context. Atlas owns
  independent verification, artifacts, reports, and experiments.

### Pre-implementation: tracked execution plan

- Treat this checked-in design as the normative architecture.
- Replace Atlas's ignored local `IMPLEMENTATION.md` dependency with a tracked
  implementation plan or linked issues that reference this design and state
  its precedence. Do not begin Phase 0 from the stale local Phase 7.
- Record the GoForj/Atlas ownership boundary for scenario contracts, Project
  preparation, guidance composition and writing, adapters, verification, and
  reporting.

### Phase 0: Authoritative adapter feasibility

- Prove one fresh, non-resumed Codex session against a minimal disposable
  fixture before committing to it as the first authoritative adapter.
- Demonstrate brokered provider transport, model identity capture, credential
  and delegated-authority non-exposure to shell children, supervisor-observed
  lifecycle events, and whole-job termination.
- Confirm that provider and adapter constraints can satisfy the authoritative
  contract; otherwise select a different first adapter while retaining the
  provider-neutral library design.

### Phase 1: Evaluation contracts

- Define scenario, guidance, agent, event, verifier, scorecard, and artifact
  types.
- Add the backward-compatible versioned preparation boundary, stable preparation and target
  step IDs, and prepare-before-target API without changing dependency meaning,
  legacy scenario execution, or documentation.
- Compile legacy and v2 specs into one immutable `ScenarioPlan` consumed by
  ordinary execution, live preparation, and documentation; do not add a second
  dependency walker or action interpreter.
- Test legacy external catalogs, migrated catalog execution, target-step
  omission during preparation, oracle isolation, and generated-documentation
  parity.
- Define the Atlas-owned `ProjectPreparer` contract and GoForj implementation,
  including capability negotiation and explicit `PrepareOptions`.
- Prove a candidate cannot substitute its embedded catalog, apply target work
  during preparation, or alter a trusted resolved preparation plan.
- Add and test the promoted `invoice-http-route`, versioned Atlas route
  expectation, and `add-http-controller/v1` verifier mapping needed by the
  first evaluation.
  Loading fails before mutation for missing, stale, ambiguous, or conflicting
  references.
- Keep `WorkflowExpectationID` in Atlas, define its typed requirement grammar
  and parser, and add import/extension conflict rules, cross-repository
  reference validation, and negative tests that cannot pass through substring
  resemblance.
- Add the durable GoForj baseline-guidance setting and compatibility tests for
  every wizard and Atlas update mode.
- Move canonical managed-marker migration, native projection composition, and
  writing for every existing Atlas agent target into GoForj. Replace Atlas's
  direct guideline writes with the versioned reconciliation result.
- Implement guidance-neutral ephemeral Project preparation and private trial
  copying with private dependency caches and no persistent cross-command cache.
- Add the `none` and `agents` guidance profiles.
- Prove profile durability and absence after render, build, and representative
  generator commands.
- Add a fake agent and fake sandbox for runner, lifecycle, terminal-state,
  evidence, cancellation, cleanup, and redaction tests.
- Define attempt-ID artifact roots, authenticated manifests, triage states, and
  the report, verifier-replay, and live-rerun contracts.
- Add positive fixtures and verifier mutants for the controller scenario.
- Define the independent controller outcome contract and at least two valid
  implementation families.

### Phase 2: First live adapter

- Implement the adapter selected by Phase 0, expected to be Codex when its
  feasibility gate passes.
- Define and test its capability matrix and complete lifecycle.
- Guarantee a fresh non-resumed session and hermetic resolved toolchain.
- Capture adapter diagnostics and final tree changes without treating them as
  trusted command or generator evidence.
- Run the controller scenario with `--intent diagnostic --backend
  unconfined-local`.
- Report artifact results only; generator compliance and other observation
  gates remain ineligible until Phase 3.
- Keep live execution behind an opt-in command and test tag.

### Phase 3: Authoritative sandbox

- Add the container or VM-backed authoritative CI backend.
- Add and qualify the first Linux supervisor-owned process monitor for complete
  descendant exec, filesystem observation, and exit capture.
- Broker provider credentials and transport outside agent shell visibility and
  networking.
- Give each concurrent trial private fixture, listener, and broker reachability,
  or run trials serially until that isolation is available.
- Keep credentials, trusted verification, evidence, and artifacts outside the
  agent boundary.
- Add negative isolation tests and sandboxed post-agent verification.

### Phase 4: Core scenarios

- Implement controller, command, job, model, named-App, and Wire-repair
  scenarios.
- Reuse existing GoForj scenario and deterministic Atlas workflow metadata.
- Add calibrated Go AST, ownership, registration, black-box, build, and test
  verifiers.
- Add each missing fixture primitive and mapping immediately before the live
  scenario that needs it.
- Measure preparation stages for the smallest and largest representative
  fixtures.
- Establish the representative, holdout, and regression portfolio across the
  required task families before making comparative claims.
- Add safe-abstention and scripted-clarification scenarios with independent
  clarification and post-answer completion metrics.

### Phase 5: Persistent Project cache

- Promote measured ephemeral preparation into the content-addressed persistent
  cache only when Phase 4 measurements justify it.
- Add canonical keys, authenticated provenance, full-hit hashing, atomic
  publication, reader leases, pruning policy, private materialization, and
  trusted/untrusted namespace separation.
- Add dependency seeds only through separately qualified tool-specific
  contracts.
- Benchmark cached and uncached preparation before enabling it by default.

### Phase 6: Atlas slices and experiments

- Add skill and MCP allowlists with absence tests.
- Enforce MCP discovery and calls through the supervisor-owned interposer;
  candidate server registration never grants authority.
- Add `placebo`, `skills`, `mcp`, `recommended-hermetic`, and custom guidance
  profiles.
- Record exact resolved guidance manifests and add randomized, interleaved
  comparison reports.

### Phase 7: Automation

- Add trusted manual and scheduled GitHub Actions workflows.
- Upload redacted reports and failure artifacts.
- Establish a historical score store or report archive.
- Observe stability and approve the versioned statistical protocol before
  enabling any release threshold.

### Phase 8: Additional agents

- Add adapters only after the common contract works with the first provider.
- Add only new native instruction projection targets; existing Atlas targets
  migrated in Phase 1 remain supported through the canonical GoForj writer.
- Publish capability differences rather than forcing false parity.

## Acceptance Criteria

The first authoritative vertical slice is implemented when:

1. `forj atlas:eval run` can execute a fresh real agent through the Atlas
   evaluation library against a prepared GoForj scenario Project.
2. GoForj persists and preserves baseline-guidance selection, and a run can
   expose `AGENTS.md` without installing Atlas skills or MCP.
3. An authoritative sandbox prevents unselected user-global instructions,
   skills, plugins, MCP servers, host credentials, writable shared caches, and
   host network access from influencing the run.
4. Supervisor-observed execution events prove generator usage without matching
   the agent's explanatory prose.
5. Reports distinguish framework outcome, workflow conformance, and combined
   contract results.
6. An independently authored and calibrated verifier accepts at least two valid
   controller implementation families while rejecting targeted mutants.
7. `none` and `agents` run against the same guidance-neutral prepared base,
   atomically project durable settings and managed files, and remain stable
   after render, build, and generator commands.
8. Failed runs retain redacted, policy-compatible actionable artifacts and an
   explicit triage state; confirmed failures may add a reviewed product-cause
   disposition without presenting automated suspicion as fact.
9. Ordinary Atlas and GoForj tests remain network-free and do not require model
   credentials.
10. A trusted manual CI workflow can run candidate code inside the
   sandbox without granting the agent repository write credentials.
11. Repeated trials within one command reuse one immutable prepared
    Project base, while tests prove that every trial receives an independent
    writable copy.
12. Unconfined local reports are visibly distinguished from authoritative
    results and cannot satisfy release isolation gates.
13. Raw provider credentials and delegated broker authority remain inaccessible
    to agent-issued code, and authoritative cleanup proves no sandbox resources
    survive.
14. Existing v1 scenarios retain byte-identical generated documentation and
    equivalent execution behavior, while v2 ordinary and live modes consume
    the same compiled plan and differ only at the preparation boundary.
15. The Atlas evaluation manifest contains only composition IDs and budgets;
    imported workflow or verifier policy cannot be duplicated, weakened, or
    overridden in YAML.

The broader system is complete when calibrated representative and holdout
scenarios cover scaffolding, feature extension, diagnosis, multi-component
behavior, refactoring, version mismatch, frontend integration, and
clarification across the important framework primitives; guidance profiles can
be compared under the approved randomized protocol; and measured
persistent-cache hits prove provenance, full content consistency, and material
whole-run improvement. Holdout evidence exposes only declassified diagnostics,
and authoritative concurrent trials have private fixture and broker
reachability. Persistent caching is not an MVP acceptance requirement.

## Open Questions

1. Which sandbox implementation should provide the first authoritative CI
   backend: a container runtime, VM runner, or another enforceable boundary?
2. Which Codex execution surface exposes the stable non-interactive behavior
   required by the first adapter?
3. Which event capabilities are required for an adapter to support behavioral
   gates rather than artifact-only scoring?
4. Where should historical scorecards live so model drift can be inspected
   without turning the repository into a large artifact archive?
5. What trial counts and regression thresholds are statistically useful within
   an acceptable release cost?
6. Should `docs-only` use an embedded pinned documentation bundle or a brokered
   local endpoint backed by the same bundle?
7. Which existing GoForj `ScenarioSpec` should provide the closest starting
   state for each initial live evaluation, and where are new scenario IDs
   genuinely required?
8. Which protected store and review process should own holdout prompts,
   fixtures, and hidden oracles without making guidance development opaque?

## Recommended First Slice

Start with one narrow vertical slice. This is an implementation checklist, not
another YAML format:

```text
evaluation: add-http-controller
project_scenario: invoice-http-route
workflow: goforj-add-http-route/v1
verifier: add-http-controller/v1
agent: Codex
guidance: none | agents
project: pinned GoForj ScenarioSpec, HTTP + SQLite, one App
trials: manually selected
diagnostic: --intent diagnostic --backend unconfined-local
authoritative: --intent authoritative --backend container-ci
framework_outcome:
  correct App
  no wire_gen.go edit
  seeded invoice behavior
  thin transport
  established invoice domain and persistence boundaries
  route registration
  tests and build
workflow_conformance:
  generator invocation
  selected App before generation
  generator-owned registration preserved
  proportionate validation
```

The local slice validates orchestration, real-agent execution, diagnostic
adapter events, and calibrated Project verification without claiming trusted
action evidence or security isolation. Generator compliance remains ineligible
there. The same slice must then pass with qualified observation in the
authoritative sandbox before its results can support guidance or release
claims. Persistent caching, additional scenarios, and Atlas capability slices
follow that working vertical path.
