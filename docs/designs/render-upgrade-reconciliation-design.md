# Render Ownership And Upgrade Reconciliation Design

## Status

- Design status: proposed
- Planning date: 2026-07-20
- Target repository: `goforj`
- Primary scope: project rendering, generated-file ownership, App extension
  points, project upgrades, and transactional publication
- Related planned package boundaries: `internal/projectrender` and
  `internal/projectupgrade`

## Summary

GoForj should treat a project upgrade as a stateful reconciliation, not as a
fresh scaffold written over an existing application.

The recommended model is:

1. Every rendered path has an explicit ownership and reconciliation policy.
2. `forj render` converges a project with the same recorded render engine.
3. `forj upgrade` explicitly crosses an engine or template revision boundary.
4. A checked-in render lock records the last successfully applied framework
   baseline.
5. Upgrade work happens in an isolated staging workspace before the live
   project changes.
6. Managed text can use a real base/current/incoming three-way merge, but an
   unresolved or ambiguous merge fails closed.
7. App-specific behavior belongs in App-owned files, neighboring owner files,
   or typed contribution seams rather than patches to managed framework files.
8. Versioned renderer migration bundles handle structural transitions.
9. The render lock advances only after generation, validation, and publication
   all succeed.

Three-way merging is the safety net. Better ownership boundaries are the
long-term solution.

## Incident That Motivated This Design

A real Ditracker upgrade exposed the difference between render idempotence and
upgrade safety.

An isolated repeat of the upgrade had these properties:

- the first render changed 22 tracked paths;
- the second render produced no diff;
- the first render removed application-specific Discord OAuth, identity, bot
  runtime, environment, logging, and migration behavior;
- the rendered result did not compile;
- a configured Vue starter kit could replace the application frontend tree;
- `.env.local` safety switches were rewritten;
- generated Compose and App-owned Compose behavior had to be separated by hand;
- the original application commit was not a trustworthy pristine baseline, so
  restoring one old tree wholesale was not safe.

The second render being clean only proved that GoForj was deterministic after
it had already replaced the application behavior. It did not prove that the
upgrade was correct.

The recovery required all three views that the renderer currently lacks:

- the framework output before the upgrade;
- the application as customized by its owner;
- the framework output after the upgrade.

It also required classifying each difference:

- a universal framework fix that should be accepted upstream;
- App policy that needs a durable owner seam;
- a safe generated update;
- a stale scaffold artifact that should be removed;
- or a true conflict requiring judgment.

That should be a first-class GoForj workflow rather than a manual forensic
exercise.

## Problem

The current renderer has several useful ownership conventions, but they are not
complete enough to make upgrades safe.

### Rendering publishes in place

`ProjectRenderer.Render` writes template output into the live project as it
walks its render steps. Module synchronization, generators, `go mod tidy`, and
Wire run afterward.

A failure in a late phase can therefore leave a partially upgraded project.
Atomic replacement of an individual file does not make a multi-file render a
transaction.

### Most template output is unconditional

Ordinary template and raw-file writers replace a destination whenever incoming
bytes differ. They do not know:

- whether the existing file still matches the last framework output;
- whether the App owner edited it;
- which framework revision last produced it;
- whether a missing path was intentionally deleted;
- or whether a retired path is still safe to remove.

### Ownership is coarse and partly implicit

The App composition layout already distinguishes:

- overwrite-rendered framework files such as generated Wire injectors;
- render-once App files such as routes, commands, schedules, lifecycle hooks,
  and `inject_*_app.go` files.

That is the right direction, but it covers only part of the output surface.
Auth, runtime, HTTP, logger, environment, database, migration, and other
templates are still overwrite-rendered without a general ownership record.
Many of those files also lack a generated header.

A file cannot safely be described as editable while also being replaced by
ordinary rerender.

### Starter-kit behavior contradicts starter ownership

The completed starter-kit design says the generated frontend becomes App-owned
after creation. The default-App renderer currently removes and recopies the
starter frontend on every full render when a starter remains selected.

Named Apps already behave more conservatively. Requiring a customized default
App to change `starter_kit` to `none` is a workaround, not an ownership model.

### `.env.local` is not currently an owner overlay

Full render rewrites `.env.local`, including values deliberately changed by a
developer. That prevents an App from using it for durable local safety policy,
such as disabling an external bot or message delivery.

`.env`, `.env.host`, and `.env.example` already contain more ownership-aware
behavior. The local overlay should not be the least safe environment surface.

### Cleanup uses inconsistent proof

Some component cleanup paths verify generated markers and refuse unknown files.
Other legacy cleanup paths delete conventional paths or trees without a general
manifest proving that the current content is still framework-owned.

The renderer also copies raw directories without a durable inventory. This can
leave renamed hashed assets behind, while a future cleanup cannot safely know
which old files it owns.

### There is no last-applied render state

`render.goforj_version` is project-creation metadata today, not a transactional
record of the last successfully applied render engine. It is not updated as the
final step of a verified upgrade and does not identify development builds or
template digests.

The module-replace state file proves that small ownership manifests are already
useful, but no equivalent exists for the full render surface.

## Goals

1. Make it impossible for an ordinary upgrade to silently overwrite locally
   changed managed source.
2. Preserve App-owned and seeded files byte-for-byte during ordinary render and
   upgrade.
3. Show the complete upgrade plan before the live project changes.
4. Distinguish safe updates, owner preservation, semantic merges, clean text
   merges, retirements, derived rebuilds, and conflicts.
5. Run generation and validation against the staged candidate.
6. Publish only a conflict-free, validated candidate, with rollback and crash
   recovery.
7. Make a second render or upgrade with unchanged inputs produce no diff.
8. Provide machine-readable plans suitable for CI and coding agents.
9. Give conflicts actionable guidance toward an owner seam or upstream fix.
10. Reduce the number of copied framework implementation files over time.
11. Work without requiring Git, while remaining pleasant to review in Git.
12. Preserve compatibility unless an upgrade bundle identifies a concrete
    source, configuration, runtime, persisted-data, or operational change.

## Non-Goals

1. Silently resolving arbitrary semantic conflicts in Go source.
2. Treating text conflict markers as a successful upgrade.
3. Runtime plugin loading or a service-locator architecture.
4. Allowing arbitrary template forks to become the primary customization API.
5. Automatically installing or updating the `forj` executable in the first
   version of this workflow.
6. Automatically migrating application business logic when an upstream public
   API is intentionally removed.
7. Deleting application data, queues, storage objects, or database state as part
   of source reconciliation.
8. Rewriting already deployed application database migrations.
9. Turning ordinary `forj render` into an implicit starter-kit update command.

## Design Principles

### Dependency-owned runtime, generated composition, App-owned policy

Stable reusable behavior should live in versioned Go packages when practical.
Generated code should compose those packages. Application policy should live in
files the application owns.

Copied framework implementations will still exist, but they should not be the
only place an App can contribute policy.

### Plan before publication

The renderer should know every intended add, update, merge, rebuild, and delete
before it mutates the project.

### Fail closed when ownership is ambiguous

Path convention alone is not sufficient proof that current bytes are safe to
replace or delete.

### Preserve owner files, not just owner directories

A known generated directory can contain owner additions. Cleanup must operate
from a recorded file inventory and must not recursively delete an arbitrary
mixed directory.

### Typed seams before template overrides

If an App repeatedly edits a managed file, either:

- the behavior belongs upstream;
- a neighboring App-owned file can express it;
- or the framework is missing a narrow contribution seam.

### A successful second render is necessary but insufficient

Idempotence proves deterministic convergence. It does not prove that the first
transition preserved application behavior.

## Terminology

### Render engine

The effective renderer implementation, artifact catalog, embedded templates,
generator contracts, and versioned migrations used by one `forj` binary.

Semver alone is not enough to identify a development build, so the engine also
has a deterministic digest.

### Artifact catalog

The authoritative inventory of possible project outputs and their ownership,
scope, source, publication, merge, and removal policies.

### Render lock

The checked-in record of the last successfully applied render engine, normalized
non-secret inputs, applied renderer migrations, and materialized artifact
baselines.

### Baseline

The exact framework output last applied for one managed artifact. It is the
`base` input to later reconciliation.

### Current

The bytes currently present in the application project before an upgrade.

### Incoming

The output proposed by the new render engine for the same artifact.

### Owner file

An application customization point created once and never rewritten or removed
by ordinary render or upgrade.

### Seed tree

An initial scaffold, such as a frontend starter kit, whose ownership transfers
permanently to the application after creation.

### Renderer migration

A versioned, one-shot, owner-safe source or configuration transition. Renderer
migrations are distinct from application database migrations.

## Sources Of Truth

| Concern | Source of truth |
| --- | --- |
| Project and App intent | `.goforj.yml` |
| Renderable artifact inventory and ownership | GoForj artifact catalog |
| Last successfully applied framework output | `.goforj/render.lock.json` and baseline store |
| App policy and business behavior | App-owned source files |
| Go dependency versions | `go.mod` and `go.sum` |
| Applied structural renderer migrations | Render lock migration IDs |
| Application schema history | Immutable application migration files |
| Runtime secrets and machine-local values | Owner environment files and process environment |

The render lock is resolved state. It must not become a second place to declare
project intent.

## Artifact Ownership Model

Every output must appear exactly once in a central artifact catalog. The current
step-local arrays and special actions can remain as implementation details only
after they are derived from or checked against that catalog.

Ownership and reconciliation are separate properties.

### Ownership classes

| Ownership | Meaning | Typical examples |
| --- | --- | --- |
| `managed` | GoForj maintains the framework baseline | runtime implementation, framework Wire injectors, generated docs |
| `owner` | Created once and permanently controlled by the App | routes, commands, schedules, lifecycle, App provider sets |
| `seed` | Initial tree handed permanently to the App | Vue, React, or Templ frontend source; user SQL migration stubs |
| `shared` | Framework and owner data coexist through a structured contract | `.goforj.yml`, `.env.example`, `.gitignore`, `go.mod` |
| `derived` | Rebuilt from another authoritative source | `wire_gen.go`, generated resource managers, API Index, OpenAPI |
| `runtime-data` | Never source-renderer owned | databases, storage data, queue state, local runtime files |

### Reconciliation strategies

| Strategy | Behavior |
| --- | --- |
| `three-way-text` | Reconcile baseline, current, and incoming text; report every merge |
| `replace-if-pristine` | Replace only while current digest equals the baseline digest |
| `create-once` | Create when absent; otherwise preserve byte-for-byte |
| `semantic` | Use a format-aware reconciler with field or key ownership |
| `regenerate` | Rebuild from source after managed reconciliation succeeds |
| `inventory` | Manage a recorded set of raw or binary paths by digest |
| `never` | Do not read or mutate as renderer-owned state |

The catalog should also carry:

- destination path or path pattern;
- template, generator, or raw-source identity;
- project, component, and App scope;
- file type and expected mode;
- whether baseline bytes may be stored;
- safe-removal policy;
- validation requirements;
- a `customize_via` hint linking to the durable owner seam.

### Header contract

Every managed text file should carry a generated header identifying it as
managed and directing durable customization elsewhere.

Every owner stub should state that GoForj creates it once and preserves it.

There should be no supported category described as both editable and
overwrite-rendered.

### Catalog contract tests

Tests must prove:

- every renderer destination is declared once;
- every declared template or generator source resolves;
- every managed text output carries the managed header;
- every owner and seed output uses create-once publication;
- derived outputs name their authoritative source;
- component cleanup can delete only recorded managed artifacts;
- no runtime-data path is reachable by a source cleanup operation;
- directory inventories preserve unknown owner files.

## Render Lock And Baseline Store

The proposed checked-in layout is:

```text
.goforj/
  render.lock.json
  render-base/
    <sha256>
```

The lock should contain:

- lock schema version;
- last successfully applied GoForj version;
- render-engine digest;
- normalized render-input digest;
- applied renderer migration IDs;
- for each materialized artifact:
  - logical path;
  - component and App scope;
  - source identity;
  - ownership class;
  - reconciliation strategy;
  - framework baseline SHA-256;
  - file type and mode.

It must not contain:

- timestamps;
- absolute project paths;
- local module-replace target paths;
- secret values or secret hashes;
- machine identity;
- runtime data.

### Why hashes alone are insufficient

A baseline digest can detect local drift, but it cannot perform a three-way
merge. Mergeable managed text therefore needs exact baseline bytes.

The baseline store should be content-addressed. Identical content is stored
once, and Git naturally deduplicates an identical blob even when it appears at
both its rendered path and content-addressed baseline path.

Only mergeable, non-secret managed text needs baseline bytes. Large binary or
raw artifacts need only a baseline digest because they cannot be safely merged.

Unreferenced baseline objects should be pruned only when no active lock entry
uses them.

### Engine identity

The engine digest should cover the artifact catalog, relevant embedded template
bytes, generator contract versions, and renderer migration registry. It should
not be the hash of the full executable or of machine-specific build metadata.

`render.goforj_version` can remain human-readable project metadata, but the lock
is the authoritative last-applied baseline. Its meaning should be clarified and
it should advance only with a successful upgrade.

## Reconciliation Rules

Let:

- `B` be the last framework baseline;
- `C` be the current application file;
- `N` be the incoming framework output.

### Managed text

| Condition | Action |
| --- | --- |
| `C == B` and `N == B` | No change |
| `C == B` and `N != B` | Safe managed update to `N` |
| `C != B` and `N == B` | Preserve the local change and report managed drift |
| `C != B` and `N != B` | Attempt a staged three-way merge |
| Three-way merge is clean | Stage the merge and report it explicitly |
| Three-way merge conflicts | Block publication and retain base/current/incoming artifacts |

A clean merge is not described as ordinary generation. The report must show
that the resulting file still contains an application delta over the new
framework baseline.

After a successful merge, the new recorded baseline is `N`, while the published
current file may be the merged result.

### Additions, deletions, and collisions

| Situation | Action |
| --- | --- |
| Incoming path is new and absent | Safe add |
| Incoming path is new but an unrelated current path exists | Conflict |
| Incoming retires a managed path and `C == B` | Safe delete |
| Incoming retires a managed path and `C != B` | Retire/delete conflict; preserve current |
| Current deleted a still-required managed path | Conflict; do not silently recreate |
| Owner or seed path exists | Preserve byte-for-byte |
| Owner or seed path is missing | Report missing owner state; recreate only by explicit owner action |
| Managed binary still matches baseline | Safe replace or delete |
| Managed binary drifted | Conflict; no binary merge |

File modes, symlink identity, and path traversal constraints are part of the
comparison. A symlink must never cause publication outside the project root.

### Semantic artifacts

Shared files must use format-aware ownership rather than whole-file text merge
when a mature reconciler exists.

Examples:

- `.goforj.yml`: typed YAML updates while preserving unknown fields;
- `.env.example`: key and section ownership with lossless owner comments;
- `.gitignore`: exact rule ownership without reordering unrelated rules;
- `go.mod`: module-aware requirements and replace ownership;
- generated Compose: generated base plus owner override.

A semantic reconciler must still emit the same base/current/incoming plan
vocabulary and fail closed when ownership cannot be proven.

## Command Model

`render` and `upgrade` should have different meanings.

### `forj render`

`forj render` converges project intent using the same render-engine identity
recorded in the lock.

It supports:

```text
forj render --plan
forj render --check
```

If the running render engine differs from the lock, ordinary render should stop
before writing and direct the user to the upgrade workflow.

### `forj upgrade`

`forj upgrade` explicitly reconciles the current application with the running
GoForj engine.

The first command surface should support:

```text
forj upgrade --plan
forj upgrade --apply
forj upgrade --status
forj upgrade --continue
forj upgrade --abort
forj upgrade --check
forj upgrade --plan --json
```

An optional `--to` should assert the expected version of the running engine. It
should not download or replace the CLI in the first implementation.

Example:

```text
forj upgrade --plan
```

Expected output should be immediately actionable:

```text
GoForj upgrade plan 0.19.0 -> 0.20.0

  34 safe managed updates
   2 semantic merges
   1 clean text merge
  17 owner or seed paths preserved
   3 derived outputs to rebuild
   2 conflicts

Conflicts
  internal/auth/service.go
    managed source changed in both the App and GoForj
    customize via: app auth hooks or a neighboring internal/auth owner file

  internal/cmd/run_cmd.go
    App runtime contribution is embedded in managed host code
    customize via: app/runtimes.go

No project files were changed.
```

### Plan and check semantics

`--plan` performs no project source writes. It may use an external temporary
workspace and prints or emits the complete proposed transaction.

`--check` is the CI form. It returns success only when the pinned engine,
committed intent, render lock, and project files have no unresolved or
unapplied reconciliation. This is deterministic and independent of machine
performance.

Machine-readable entries should include:

- path;
- ownership and strategy;
- proposed action;
- component and App;
- source identity;
- base, current, and incoming digests;
- reason;
- `customize_via` guidance;
- compatibility note IDs;
- conflict artifact locations when retained.

## Staged Upgrade Transaction

Upgrade must not run the renderer against the live project and hope Git can undo
it later.

The transaction is:

1. Read project intent and the prior render lock.
2. Snapshot the relevant live paths and their digests.
3. Create an isolated staging workspace.
4. Materialize incoming framework output there.
5. Apply ordered renderer migration bundles there.
6. Reconcile managed, owner, seed, shared, and retired artifacts there.
7. Regenerate derived output there.
8. Run mandatory validation there.
9. Print the final plan.
10. If applying, verify every affected live path still matches the snapshot.
11. Publish the explicit add, replace, and delete set with a rollback journal.
12. Publish the new lock and baseline objects last.
13. Remove the journal only after the transaction is complete.

Unrelated dirty worktree files remain untouched. A concurrent change to an
affected path invalidates the plan and stops publication.

### Conflict workflow

Conflict markers should not be written into live application source by default.

`upgrade --apply` may retain a project-local ignored upgrade workspace containing
base, current, incoming, and tentative merged files. `upgrade --status` explains
what remains. The user edits only the staged result or moves the customization
to a durable owner seam, then runs `upgrade --continue`.

`upgrade --abort` removes staged state and leaves the live project and old lock
unchanged.

### Validation

The mandatory staged validation floor should include:

- template formatting and parsing;
- renderer migration validation;
- all selected generators;
- Wire generation;
- `go mod tidy` consistency;
- compilation of every relevant Go module with `GOWORK=off`;
- path and ownership invariant checks.

Projects may configure additional upgrade validation, such as:

- `go test ./...` and `go vet ./...` for every relevant module;
- frontend typecheck and build;
- Compose configuration parsing;
- API Index and OpenAPI generation;
- project-specific fixture checks.

Validation output must not be interpreted as permission to publish when an
ownership conflict remains.

### Rollback and recovery

Per-file atomic writes are retained, but a transaction journal provides
multi-file rollback.

Ordinary failures restore the prior live bytes and modes. A crash leaves a
journal that `forj upgrade --status` or a future `upgrade:doctor` command can
inspect and recover.

The API Index staged publication code is an existing in-repository model for
compare-before-publish, explicit removals, and rollback behavior.

## Durable App Customization

The upgrade engine prevents data loss, but it should also teach the App how to
stop carrying a recurring patch.

### Prefer a neighboring owner file first

Go packages do not require all methods and types to live in one file. Additive
App behavior can often live in a separate owner file in the same package.

Examples:

- additive methods on a generated-local auth type can live in
  `internal/auth/discord_app.go`;
- an App-specific Discord controller can live in its own domain package and be
  registered through App-owned routes and controller provider sets;
- application repositories and services belong in their domain package and
  App-owned Wire sets.

This avoids a new framework API when no interception point is required.

### Add typed contribution seams where composition is real

Some customization must participate inside a managed flow. Those cases need a
narrow domain-neutral contract.

#### App runtimes

A long-running Discord gateway is not merely a startup callback. It has runtime
identity, cancellation, failure, and shutdown semantics that should participate
in the aggregate runtime host.

Add render-once surfaces such as:

```text
app/runtimes.go
app/wire/inject_runtimes_app.go
```

The managed `run` host should aggregate built-in runtimes with App-owned runtime
contributions. An empty owner set preserves current behavior.

#### Auth and OAuth policy

App-owned auth seams may be needed for:

- provider registration;
- provider profile normalization;
- OAuth callback and consent-fallback policy;
- identity linking and account resolution;
- post-login policy.

App-specific endpoints should still prefer separate controllers and App-owned
route registration. The hook should cover only behavior that truly intercepts
the managed auth flow.

#### Logger and access policy

Presentation, access observers, and App-specific fields should be injectable or
configuration-driven rather than patches to the generated logger core.

#### Reusable migration execution

If application code needs migration behavior currently trapped inside a CLI
command, extract a stable framework service API. Do not preserve an App patch to
the generated command merely to call it programmatically.

### Triage guidance from Ditracker

| Repeated customization | Durable direction |
| --- | --- |
| Discord bot in aggregate `run` | App-owned runtime contribution |
| Discord-specific routes | App controller plus App-owned routes |
| Additive auth helpers | Neighboring owner file in `internal/auth` |
| OAuth callback or identity policy | Narrow typed auth hooks |
| JSON command silence, child exit codes, timezone, listener behavior | Universal framework fix upstream |
| UTC database defaults | Universal framework policy upstream |
| Logger formatting or observation | Configured or injected logger policy |
| Programmatic migration execution | Framework migration service API |
| Traefik and RapidOCR | Generated Compose base plus owner override |
| Product Vue application | App-owned seed frontend |
| Local Discord safety switches | Owner environment overlay |

The artifact catalog's `customize_via` field should make this guidance available
at the exact conflict where it is useful.

### Avoid template partial forks as the primary model

An arbitrary template override hides upstream changes and becomes a private
framework fork.

If an interim escape hatch is necessary, prefer an explicit patch queue with a
recorded base digest, applied only in the staged workspace. Patch failure blocks
the upgrade. Recurring patches should be migrated to an owner seam or upstream
fix.

This design should align with the generated aggregation points proposed by the
[GoForj Extensions Design](forj-extension-design.md), without requiring the
entire extension system before Apps can own their policy.

## Surface-Specific Decisions

### Starter kits

Starter-kit source is `seed` ownership.

- default and named Apps preserve an existing frontend;
- ordinary render never recursively replaces it;
- changing `starter_kit` affects initial creation or an explicit transition,
  not ownership of an existing product frontend;
- an eventual `forj starter:update --plan` may offer a reviewed three-way
  candidate, but it is separate from ordinary render.

The immediate implementation should remove the default-App overwrite behavior.

### Environment files

`.env.local` should be owner-controlled and must stop being unconditionally
rewritten.

If GoForj requires generated low-precedence local defaults, introduce a separate
generated layer or a semantic key-owned contract. Preserve existing environment
precedence until that runtime configuration change is deliberately designed and
migrated.

`.env.example` remains a shared semantic artifact. Framework keys, owner keys,
comments, redaction, and default-value ownership must be explicit in its
reconciler and visible in the plan.

Secret-bearing `.env` and host overlays never enter the baseline store.

### Docker Compose

Continue the existing intended model:

- `docker-compose.yml` is generated and managed;
- `docker-compose.override.yml` is owner-controlled;
- project-specific services and policy live in the override;
- the generated base has a recorded raw-file and service inventory;
- semantic validation parses the effective Compose configuration.

### Raw and binary directories

Lighthouse distribution assets and similar raw trees require a recorded file
inventory.

An old asset can be deleted only when:

- the prior lock records it as managed;
- its current digest still matches the baseline;
- and the incoming inventory retires it.

Unknown files in the same directory are preserved.

### Derived output

Wire output, primitive managers, API indexes, and OpenAPI documents are rebuilt
after source reconciliation. Manual drift is reported rather than silently
treated as an authoritative customization.

### Application migrations

Migration source files created for the App are owner or seed artifacts.
Framework schema changes add new migrations. They do not rewrite an already
deployed migration.

Generated dialect translations may use their own source checksum and generated
ownership contract, as described by the
[Migration Translation Design](migration-translation-design.md).

## Versioned Renderer Migration Bundles

Structural transitions are not ordinary template changes. They should move from
scattered heuristic cleanup into `internal/projectupgrade` as ordered bundles.

Each bundle declares:

- stable migration ID;
- supported source contract or engine range;
- affected paths and ownership classes;
- deterministic preconditions;
- staged operations;
- validation;
- source, configuration, runtime, persisted-data, and operational compatibility
  notes;
- a manual migration path when preconditions do not hold.

Allowed staged operations include:

- exact file moves;
- ownership reclassification;
- seeding a new owner hook;
- AST-aware Go transformations;
- lossless YAML or dotenv transformations;
- removal of a recorded pristine managed artifact.

Bundles must be idempotent and fail closed when customization makes the expected
shape ambiguous. Applied IDs are recorded in the render lock rather than
rediscovered heuristically forever.

Renderer migrations remain distinct from application database migrations and
from reusable Go module version upgrades.

## Legacy Project Adoption

New projects should write a lock and baseline on first successful render.

Existing projects require an explicit adoption workflow:

```text
forj upgrade --adopt --plan
```

Adoption may classify a path automatically when:

- it is a known owner or seed path;
- a managed marker and exact incoming bytes prove the baseline;
- a recorded legacy ownership marker proves it;
- or an explicitly supplied prior render bundle reconstructs the exact base.

Every ambiguous customized managed file is protected as a conflict. Adoption
must not assume that a file is pristine because it has a conventional path.

Git history may help find a blob matching a known baseline digest, but Git is an
optional accelerator rather than a correctness dependency.

If temporary legacy overwrite behavior is retained during rollout, it must
require an explicit flag, print every affected path, and have a removal date. It
must not remain the silent fallback.

## Reporting And Compatibility Notes

The upgrade report should group actions by behavior, not describe every write as
"created."

Required groups:

- safe managed updates;
- clean automatic merges;
- owner and seed preservation;
- semantic changes;
- derived rebuilds;
- safe retirements;
- local managed drift;
- unresolved conflicts;
- renderer migrations;
- dependency changes;
- compatibility and operator notes.

Compatibility notes must classify impact precisely:

- source/API;
- configuration;
- persisted data;
- runtime behavior;
- operational migration;
- minimum Go version.

A dependency version change is not itself a breaking change. The report should
name the concrete consumer behavior that stops working and provide a migration
path when one exists.

## Implementation Phases

### Phase 0: immediate safety

- Stop default-App starter-kit overwrite.
- Stop unconditional `.env.local` replacement.
- Introduce the authoritative artifact catalog.
- Add managed and owner header contract tests.
- Make render reporting distinguish adds, updates, skips, and deletes.
- Refuse deletion of a locally changed retired file.
- Add a non-mutating two-way `render --plan` even before full baseline merging is
  available.

This phase prevents the highest-impact loss while the stateful engine is built.

### Phase 1: render state and drift checks

- Add the render lock and content-addressed baseline store.
- Record engine and normalized non-secret input digests.
- Add `render --check` and engine-mismatch diagnostics.
- Add explicit legacy adoption.
- Fold existing small ownership records into the same state family where that
  can be done without losing their focused semantics.

### Phase 2: staged upgrade and publication

- Add `forj upgrade --plan` and `--apply`.
- Build candidates outside the live project.
- Implement the base/current/incoming action matrix.
- Add clean text merge and conflict artifacts.
- Run generators, Wire, module validation, and compilation in staging.
- Add compare-before-publish, rollback journal, continue, and abort behavior.
- Publish the lock last.

### Phase 3: recurring-conflict removal

- Add App-owned runtime contributions.
- Add only the auth and OAuth policy seams proven necessary by the reference
  application.
- Add logger/access policy seams where configuration is insufficient.
- Extract stable migration execution from CLI commands.
- Add `customize_via` diagnostics.
- Ensure every managed file that real Apps commonly edit has a durable
  alternative.

### Phase 4: versioned renderer migrations

- Move existing owner migrations and legacy cleanup into an ordered registry.
- Record applied IDs.
- Add compatibility and operator metadata.
- Prove direct `N-2 -> N` upgrades are equivalent to sequential upgrades where
  the documented contract permits both.

### Phase 5: shrink copied framework implementation

- Move stable runtime behavior from copied auth, HTTP, logger, runtime, and
  primitive templates into versioned packages when that produces a narrow real
  API.
- Keep generated code focused on App-specific adapters and composition.
- Do not create a broad framework SDK solely to reduce template line count.

## Validation Strategy

### Reconciliation truth table

Unit tests must cover every meaningful base/current/incoming combination for:

- add;
- update;
- current deletion;
- incoming deletion;
- path collision;
- clean text merge;
- conflicting text merge;
- binary drift;
- file mode changes;
- symlink and project-boundary safety.

### Long-lived upgrade fixture

Fresh render combinations are not enough. Add a Ditracker-shaped fixture that
starts on an older render engine and contains:

- a custom OAuth provider and identity policy;
- an App runtime;
- owner routes, commands, and Wire providers;
- an owner environment safety overlay;
- a Compose override;
- a custom migration;
- logger policy;
- a heavily modified frontend;
- a neighboring owner file inside a generated-local package.

Upgrade it across at least two render revisions and prove:

- owner and seed bytes remain unchanged;
- pristine managed files update;
- managed drift is reported;
- clean merges are explicit;
- conflicts cause no live mutation;
- stale pristine managed files are removed;
- stale customized files are preserved as conflicts;
- derived output is rebuilt;
- failed validation publishes nothing;
- the second upgrade produces no diff.

### Transaction failure injection

Tests should fail deliberately:

- while building the candidate;
- during a renderer migration;
- during generation;
- during Wire;
- during module validation;
- immediately before publication;
- during publication;
- after source publication but before lock publication.

Every ordinary failure must restore the prior project. Crash cases must leave a
recoverable journal.

### Security and privacy

Tests must prove:

- secrets and secret hashes never enter the lock, baseline, plan, or diagnostic
  output;
- absolute machine paths are absent;
- symlinks cannot escape the project;
- a malicious lock cannot write outside the project;
- file modes are preserved intentionally;
- staged tools cannot publish unexpected paths.

### Repository validation

The acceptance fixture must inventory every nested `go.mod` and validate every
relevant module independently. At least one pass uses `GOWORK=off`.

The largest supported generated composition should run:

- render and generate;
- Wire;
- Go build, tests, and vet;
- frontend typecheck and build when present;
- Compose parsing;
- API Index and OpenAPI generation;
- a second render and generation diff check.

All GoForj test renders remain under `/tmp`.

## Compatibility

### Source and API

The safety and state phases require no public application API break. New owner
contribution contracts should be additive, with empty owner sets preserving
current runtime behavior.

### Configuration

The render lock is a new checked-in generated contract. Clarifying
`render.goforj_version` as the last successfully applied human-readable version
is a configuration-behavior change and requires migration documentation.

Environment-layer changes must preserve existing precedence until a deliberate
migration says otherwise.

### Persisted data

The source upgrade engine does not delete or migrate application data. Database
changes continue through ordinary additive application migrations.

### Runtime behavior

Phase 0 changes starter and `.env.local` ownership behavior. Those changes make
the implementation match existing ownership documentation, but they should
still be called out in release notes.

### Operational migration

Existing projects must adopt the lock explicitly. CI should pin the GoForj
engine described by the lock and run the objective check command.

### Minimum Go version

No minimum Go version increase is required by this design. Any implementation
dependency that changes that must identify the exact constraint separately.

## Risks And Mitigations

### Baseline store size

Exact text baselines duplicate working-tree bytes on disk.

Mitigation:

- store only mergeable managed non-secret text;
- use content-addressed objects;
- rely on Git blob deduplication;
- prune unreferenced objects;
- keep large binaries digest-only.

### False confidence from automatic merge

A textual merge can compile while changing behavior.

Mitigation:

- report every merge;
- validate the staged result;
- keep recurring deltas visible;
- direct them toward owner seams or upstream fixes;
- never treat an unresolved merge as success.

### Too many owner interfaces

Creating a hook for every application request would produce an unstable SDK.

Mitigation:

- prefer neighboring owner files and existing composition files;
- require a real reference use case;
- keep contracts domain-neutral;
- add only true interception or aggregation seams.

### Slow upgrade plans

Full staged validation can be expensive.

Mitigation:

- separate mandatory structural validation from configured project checks;
- cache dependencies without sharing mutable project output;
- expose phase timings;
- retain correctness as the default for `--apply`.

### Legacy ambiguity

Older Apps do not have exact baselines.

Mitigation:

- explicit adoption;
- generated-marker and exact-byte proof;
- optional prior render bundles;
- conservative conflicts for everything ambiguous.

### Concurrent project edits

An upgrade plan may become stale while a developer or agent changes files.

Mitigation:

- fingerprint affected paths;
- compare immediately before publication;
- invalidate rather than merge against a moving target.

## Open Decisions

1. Whether baseline objects should be stored as raw exact bytes or a standard
   compressed representation. Raw bytes are easier to inspect and allow Git to
   reuse identical blobs; compressed bytes reduce checkout size.
2. Whether a clean automatic text merge should be applied automatically by
   `upgrade --apply` or require explicit confirmation. It must always be visible
   in the plan.
3. The exact mandatory validation floor versus project-configured validation.
4. How development builds assert engine identity when several builds share one
   semver.
5. Whether `upgrade:doctor` should ship with the first transactional slice or
   whether `--status`, `--continue`, and `--abort` are sufficient initially.
6. Whether module-replace ownership remains a focused companion file or moves
   into a common state envelope.
7. The exact generated environment layer, if one is needed before
   owner-controlled `.env.local`.
8. The first auth hook contract proven by the long-lived fixture.

## Acceptance Criteria

This design is successfully implemented when:

1. Ordinary render cannot overwrite or delete a locally changed managed file
   without an explicit reconciliation action.
2. Owner and seed files are preserved byte-for-byte.
3. Default starter-kit rerender never removes a customized frontend.
4. `.env.local` is no longer unconditionally replaced.
5. A complete non-mutating plan is available before upgrade.
6. The plan distinguishes every action category defined here.
7. A failed render, migration, generation, Wire, tidy, compile, or project check
   leaves the live project and old lock unchanged.
8. Managed text uses the exact last framework base for three-way decisions.
9. Retired managed files are deleted only with baseline proof.
10. Conflict markers never appear in live source unless the user explicitly
    publishes a resolved file containing them.
11. App runtimes can join the aggregate host without editing managed run or Wire
    files.
12. The Ditracker-shaped fixture upgrades without losing Discord, environment,
    Compose, migration, logger, or frontend behavior.
13. A second upgrade with unchanged inputs produces no diff.
14. The lock and baseline contain no secrets, secret hashes, timestamps,
    absolute paths, or machine state.
15. Every relevant nested Go module and the largest supported generated
    composition pass the required validation matrix.

## Related Designs And Context

- [Generated App Extension Points](../context/generated-app-extension-points.md)
- [Rendering And Smoke Workflow](../context/rendering-and-smoke-workflow.md)
- [App Composition Layout Design](completed/app-composition-layout-design.md)
- [Starter Kits Design](completed/starter-kits-design.md)
- [App Primitive Component Gating Plan](completed/app-primitive-component-gating-plan.md)
- [Comprehensive Code Quality Pass Plan](comprehensive-code-quality-pass-plan.md)
- [GoForj Extensions Design](forj-extension-design.md)
- [Environment Contract Generation Design](env-contract-generation-design.md)
- [Migration Translation Design](migration-translation-design.md)
- [Docker Compose Developer Service Catalog Design](completed/docker-compose-developer-service-catalog-design.md)

## Recommendation

Proceed in two tracks:

1. Land the immediate safety corrections first: truthful starter ownership,
   owner-controlled `.env.local`, an exhaustive artifact catalog, and
   non-mutating plan output.
2. Build the render lock, staged transaction, and three-way reconciliation in
   `internal/projectupgrade`, then remove recurring conflicts through the
   smallest proven App-owned seams.

Do not wait for perfect auto-merge before stopping destructive overwrite
behavior. A precise plan that preserves the live tree and blocks ambiguous
managed drift is already a major improvement. Add three-way merging as the
recorded baseline becomes available, and treat it as a bridge toward clean
ownership rather than permission to keep application policy in managed files.
