# Framework Upgrade Inspection Spike

Status: experimental, inspection only.

## Purpose

Plan upgrades for the whole generated framework while preserving application
customizations. Auth is a behavioral case study, not the scope of the planner.
Planning works for CLI-only projects without auth or a database, mixed component
compositions, starter kits, and named Apps.

Applications do not need to commit baseline source copies or adopt a render lock
before using the inspector. Unversioned differences remain explicitly uncertain.

## Commands

Run from an existing application using a binary built from this PR:

```sh
forj render --plan
forj render --plan --json
forj render --plan --json \
  --from-renderer /path/to/comparison-forj \
  --target-renderer /path/to/target-forj
```

The target defaults to the user-facing binary. It orchestrates renderer processes
through experimental snapshot protocol v3 (`render --snapshot`). Component-only
`--only` selection is not part of framework upgrade planning.

Renderer processes receive disposable workspaces with projected configuration
and neutral environment layers. They do not receive application source, private
environment values, owner commands, local module replacements, or configuration
extras. The protocol uses bounded output, a timeout, direct executable invocation,
and actual binary version/commit/dirty metadata. Comparison commands use the
existing native process supervisor so managed descendants are stopped on
cancellation and after a renderer leader exits. Child temporary directories stay
inside the comparison workspace so forced termination does not leave nested
snapshot workspaces behind. Global CLI installations and
authentication configuration remain unchanged.

Older released binaries do not implement this protocol. Downloading versions,
historical-release adapters, caching, and original-baseline authentication are
not implemented. A supplied renderer is an unverified comparison reference.

## Authoritative artifact inventory

The snapshot runs the ordinary renderer's scaffold phase inside a fresh temporary
workspace. Its existing template, render-once, raw-file, component gating, App
mapping, and starter-kit paths produce the inventory. There is no second auth
catalog or parallel table of framework template filenames.

Planning stops before dependency synchronization, module tidy, templ compilation,
application generators, Wire, and build tools. A maximal-composition test empties
`PATH` and proves that a snapshot still succeeds. The actual application is never
a render destination.

Managed template output is retained transiently for comparison. Raw assets use
exact digests, avoiding binary baseline copies and UTF-8 conversion. Source
provenance and intended executable bits remain attached to artifacts. Existing
application source and environment values never enter the snapshot protocol.

| Ownership | Planning policy |
| --- | --- |
| Framework templates | Compare current, incoming, and optional reference bytes; report declaration changes for Go |
| Framework raw assets | Compare exact digests; require review for differences or retirement |
| App-owned seeds | Preserve app source, routes, lifecycle hooks, frontend sources, and owner Wire contribution files |
| Database migrations | Preserve existing history, including retired files; missing incoming migrations require review, while absent retired files stay absent |
| Shared semantic files | Review dependency/configuration contracts separately from template replacement |
| Private environment layers | Preserve without reading or fingerprinting values |
| App-derived output | Regenerate from actual reconciled app inputs; do not label app-specific bytes as dirty |
| Conditional derived output | Review operator/runtime ownership policy before deciding to regenerate |
| Unknown neighbors | Preserve without opening their contents |
| Unclassified scaffold output | Report for explicit ownership review |

Derived contracts cover cache, queue, storage, event, and mail managers/accessors;
database connection generation; default and named App Wire and API-index/OpenAPI
output; templ compiler sidecars from actual starter sources; and selected frontend
build directories. Resource and API-index paths come from shared catalogs used by
the generators themselves. Only active output contracts are claimed. Missing
outputs require generation, and retired outputs require review. Metrics discovery
targets have conditional ownership because runtime policy can delegate them to an
operator. A filename ending in `_gen.go` is insufficient evidence of
ownership. Unknown generated-looking files remain preserved owner additions.

Framework-owned differences produce actions such as `review-unversioned`,
`review-missing`, `review-collision`, `review-retirement`, or `reconcile`. Relative
baseline classifications explicitly say `relative-to-baseline`. No action grants
permission to overwrite or delete application files.

Executable-bit differences and unavailable Go syntax hints are visible in both
text and JSON plans. Text paths are quoted to escape filenames containing line
breaks or terminal control characters. Empty Go source remains a comparison input
and is diagnosed rather than mistaken for an omitted payload. Executable-bit differences are
reported on platforms that support them. Group read/write permissions are not
framework drift; Git archives and local umasks can vary them without changing
tracked source semantics.

## Inputs and limits

The projection retains project/module names, component selections, starter-kit
options, help formats, and named App participation/discovery. It deliberately
excludes arbitrary owner configuration and private environment layers. Resource
selection therefore uses component defaults, not inferred production drivers.
Driver-dependent differences and owner extension behavior require review. The
input digest uses a dedicated versioned recipe (contract v1), independent of
incidental configuration schema changes. One frozen recipe is passed to both
renderers, with configuration and App discovery rechecked before returning results.
Every supported persisted component flag participates, including `demo_app`, which
is intentionally absent from the project wizard's component catalog.

Both renderers receive the same current projected inputs. This does not recover
historical configuration changes or prove that an app adopted the supplied
baseline. Engine metadata is diagnostic, not cryptographic provenance.

Go declaration hints retain package names, build constraints (including effective
legacy placement), and Go compiler directives associated with their next or enclosing
declaration while omitting ordinary comments and formatting. Package-variable
declaration order remains visible because initializer side effects can depend on
it. Hints can still include semantically equivalent implementation changes. Disjoint
declarations can depend on one another. A test explicitly verifies that non-overlapping declarations do
not authorize a clean automatic upgrade.

Named Apps participate in framework snapshots and Wire classification. The
snapshot serializes its App rendering so inventory capture remains deterministic;
ordinary rendering retains its existing concurrency.

Managed files are opened through a rooted filesystem handle. Managed-file reads
reject symlinks, bounds source reads to 4 MiB, streams raw-asset digests up to 64 MiB,
and verifies file identity and compared state before returning a plan. Opaque
private, seeded, and owner files are not opened. These checks detect common
concurrent edits; inspection is not a filesystem transaction or an executable
sandbox for supplied renderer binaries.

App marker discovery shares the ordinary renderer's marker catalog through a
rooted filesystem. Discovery-root or marker symlinks cannot select Apps outside
the inspected project; ordinary discovery retains its existing behavior.

Peer responses require strict, bounded JSON: canonical field names, no duplicate
keys, valid UTF-8, limited nesting, and exactly one document. Accepted source strings
and engine metadata must survive JSON serialization without losing bytes; malformed
UTF-8 is rejected before acceptance. Artifacts require explicit ownership, portable
paths without case collisions or file/directory overlap, and matching source digests.
Ambiguous or incompatible peers fail closed.

## Reproducible reference inspection

```sh
go build -o /tmp/forj-framework-upgrade ./cmd/forj
go run ./tools/upgradespike reference \
  --forj /tmp/forj-framework-upgrade \
  --project ~/code/ship \
  --project ~/code/ditracker
```

The Go experiment command lives in `tools/upgradespike`; its regression tests run
with the ordinary `go test ./...` suite. It copies regular tracked files from
committed revisions into `/tmp`, excluding environment files and symlinks. It
invokes the actual CLI, verifies all
copied bytes and modes remain unchanged, and retains plans plus a summary outside
source repositories. Add `--from-renderer` for a two-renderer comparison.

Reference revisions:

- Ship: `f3eb4bae25cbc811025a706c17af1a0f287f33b6`.
- Ditracker: `6fe4b4842a08b131ba6723d5ce16fa3dcc431e49`.

The comparison experiment uses the framework-wide emitter with historical auth
templates from `68f1de1463e2379f20d63c3329380f0116587074` as an input variation.
Other templates and emitter/configuration behavior remain current. This is a
historical-template replay, not execution of that revision's original released
renderer. Both binaries plan the full framework rather than only auth.

The framework-wide run preserved every copied byte and file mode:

| Inventory | Ship | Ditracker |
| --- | ---: | ---: |
| Total plan records | 1,145 | 636 |
| Managed templates | 287 | 266 |
| Managed raw assets | 13 | 13 |
| Existing migrations preserved | 42 | 42 |
| Owner seeds preserved | 520 | 12 |
| Derived output contracts present | 19 | 15 |
| Missing active derived outputs | 0 | 1 |
| Conditional derived output policies | 1 | 1 |
| Unknown owner additions preserved | 255 | 278 |
| Private environment contracts | 3 | 3 |
| Shared semantic contracts | 5 | 5 |
| Files requiring reconciliation against the supplied reference | 4 | 4 |

These records are transient inspection results, not files added to either app.
The reference variation changes auth templates only, so this experiment does not
exercise upstream changes in every subsystem. Other subsystem differences are
still inventoried and retain unverified-baseline semantics.

## Auth as a behavioral case study

```sh
go run ./tools/upgradespike auth \
  --forj /tmp/forj-framework-upgrade \
  --ditracker ~/code/ditracker
```

This controlled experiment applies the conditional OAuth-state consumption
behavior already present in Ship to a disposable Ditracker copy. It uses SQLite
and a public test-only JWT key. The candidate fix remains confined to the
experiment; it is not a production auth change in this PR.

| Phase | Observed result |
| --- | --- |
| Original Ditracker auth suite | 103 tests/subtests pass |
| Add replay and expiry regression tests before the fix | Both regressions fail behaviorally |
| Apply conditional state consumption | All three regression test/subtest results pass |
| Replace request authentication with the framework default | Compiles, but fails the impersonation round-trip test |
| Restore the app request method and retain the state fix | Full suite passes with 106 tests/subtests |

The runner requires the final passing test identities to include every baseline
identity plus all three new regression identities, with no skipped tests. Both
replay and expiry must fail before the fix; compiler failures and a failing parent
test alone cannot satisfy that negative control.

Only `oauth_state.go` and the added regression test differ in the final auth source
inventory. Impersonation, Discord flow, identity persistence, and other customized
auth source remain byte-for-byte unchanged. The experiment demonstrates one
reviewed reconciliation and one rejected behavior regression, not a general AI
merger or arbitrary overlapping-method compatibility.

Ship's two untagged auth tests pass. Its broader SQLite auth suite encounters an
existing deployment-policy migration dependency before establishing a baseline.
The experiment does not alter migrations to bypass that limitation.

## Independent agent trial

A separate Codex session attempted a full unversioned Ditracker upgrade using the
plan and snapshot, without a prescribed reconciliation patch. Independent checks
preserved all 1,453 baseline unit and 103 auth passing identities, and verified
new trace correlation behavior plus a real app build. The result remains partial:
existing app test failures prevent full-suite acceptance. The app retained its
database version as infrastructure policy; adopting every scaffold default is not
an upgrade requirement.
A second agent found no actionable source regression. The experiment also
reproduced a help/build/generation sequence that prunes driver pins and later
selects incompatible dependencies. See the [agent trial report](agent-framework-upgrade-trial.md)
for controls, evidence, corrections, and the requirements for a repeatable upgrade.

## Validation and remaining work

Tests cover framework planning without auth, named Apps, all starter-kit choices,
the maximal component inventory without tools, private/configuration projection,
raw assets, migrations, owner seeds, derived output, malformed/unsafe snapshots,
unknown additions, and template-output parity against ordinary rendering.

The root Go suite, vet, focused race suite, and relevant nested modules are
validated with workspace resolution disabled. The reviewed build also passed the maximal MySQL composition through ordinary
render, component contracts, Wire, build, and generated Go tests. The two protocol peers emit framework-wide
snapshots and preserve the real reference projects' copied bytes and modes.

The review included separate passes over input/compatibility preservation,
artifact ownership and generation, filesystem/concurrency boundaries, and the
cross-renderer protocol. Regression tests cover discovered-App inheritance,
local-only module names, inactive component outputs, conditional metrics targets,
missing/retired generation contracts, symlink replacement, concurrent managed-file
changes, compiler directives, and ambiguous JSON. Three bounded fuzz campaigns
exercised snapshot validation, JSON framing, and Go declaration hints for more
than 770,000 combined inputs without failures. This is additional evidence, not a
proof of arbitrary app compatibility.

A further review reproduced and corrected two misleading classifications: absent
retired migrations were reported as new work, and unknown baseline mode metadata
could hide a current executable-bit change. Existing retired migrations now have
an explicit preservation action, absent retired migrations stay absent, and
unproven mode drift requires reconciliation. Another regression covers moving a
compiler directive between declarations without changing the file-wide directive
list. Repeated initializer order remains visible in declaration hints.

The same pass removed quadratic concatenation for repeated declaration names.
On a local Apple M4 Max benchmark with 2,500 initializers, two runs decreased from
85.7 MB allocated per operation and 13.3 to 13.5 ms to 4.0 MB and 4.4 to 6.3 ms.
These measurements describe this synthetic inspection workload only. The auth
experiment's evidence checks also have direct Go tests for skipped tests,
missing baseline identities, compiler failures, and incomplete negative controls.
The reference copier also tests case-insensitive environment-layer exclusion and
symlink exclusion so those private paths never enter a disposable app copy.

The reference, controlled-auth, and independent-agent experiment runners are all
implemented in Go. The Go runners reproduced the reference inventory counts and
the controlled auth red/green results. Re-evaluating the retained agent candidate
preserved the original test identities, changed-file list, and residual differences.
Tests also cover malformed evidence, normal-exit descendant cleanup, tracked
dependency preservation, and nested modules hiding failing or empty packages.

An independent core review also reproduced three cases now covered by direct
regressions: upstream executable changes beside local content edits, discovery
through external directory/marker symlinks, and renderer descendants surviving
timeout or normal leader exit. Mode classification now considers upstream
executable bits, discovery uses the confined shared marker policy, and renderer
orchestration reuses native process-tree supervision. A maximal normal render
and both committed reference inspections passed after these corrections.

The Windows CI failure on the earlier revision came from a MySQL readiness test
that indexed beyond its three shell responses after an unexpected extra probe.
The sequencing test now injects deterministic responses; the separate shell
contract test still verifies the probe script. No production readiness behavior
changed.

The inspector does not implement AI reconciliation or publication. The next
steps are historical renderer/input recovery, resource-intent capture without
secrets, semantic dependency/configuration planning, generator-specific staging,
and reviewed overlapping behavior changes beyond the auth case study.

Any future apply path needs staged generation/validation, conflict handling,
compare-before-publish protection, and transactional publication. A passing plan,
compilation, or textual merge cannot establish behavioral compatibility.

This explores reconstructed baselines as an alternative to checked-in source
copies in [Render Ownership And Upgrade Reconciliation](render-upgrade-reconciliation-design.md).
