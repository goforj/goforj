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
through experimental snapshot protocol v2 (`render --snapshot`). Component-only
`--only` selection is not part of framework upgrade planning.

Renderer processes receive disposable workspaces with projected configuration
and neutral environment layers. They do not receive application source, private
environment values, owner commands, local module replacements, or configuration
extras. The protocol uses bounded output, a timeout, direct executable invocation,
and actual binary version/commit/dirty metadata. Global CLI installations and
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
| Database migrations | Preserve existing migration history; missing incoming migrations require review |
| Shared semantic files | Review dependency/configuration contracts separately from template replacement |
| Private environment layers | Preserve without reading or fingerprinting values |
| App-derived output | Regenerate from actual reconciled app inputs; do not label app-specific bytes as dirty |
| Unknown neighbors | Preserve without opening their contents |
| Unclassified scaffold output | Report for explicit ownership review |

Derived contracts cover cache, queue, storage, event, and mail managers/accessors;
database connection generation; default and named App Wire output; and frontend
build directories. A filename ending in `_gen.go` is insufficient evidence of
ownership. Unknown generated-looking files remain preserved owner additions.

Framework-owned differences produce actions such as `review-unversioned`,
`review-missing`, `review-collision`, `review-retirement`, or `reconcile`. Relative
baseline classifications explicitly say `relative-to-baseline`. No action grants
permission to overwrite or delete application files.

Executable-bit differences are reported on platforms that support them. Group
read/write permissions are not framework drift; Git archives and local umasks can
vary them without changing tracked source semantics.

## Inputs and limits

The projection retains project/module names, component selections, starter-kit
options, help formats, and named App participation/discovery. It deliberately
excludes arbitrary owner configuration and private environment layers. Resource
selection therefore uses component defaults, not inferred production drivers.
Driver-dependent differences and owner extension behavior require review.

Both renderers receive the same current projected inputs. This does not recover
historical configuration changes or prove that an app adopted the supplied
baseline. Engine metadata is diagnostic, not cryptographic provenance.

Go declaration hints omit comments and formatting but can still include
semantically equivalent implementation changes. Disjoint declarations can depend
on one another. A test explicitly verifies that non-overlapping declarations do
not authorize a clean automatic upgrade.

Named Apps participate in framework snapshots and Wire classification. The
snapshot serializes its App rendering so inventory capture remains deterministic;
ordinary rendering retains its existing concurrency.

## Reproducible reference inspection

```sh
go build -o /tmp/forj-framework-upgrade ./cmd/forj
python3 scripts/spike-framework-upgrade.py \
  --forj /tmp/forj-framework-upgrade \
  --project ~/code/ship \
  --project ~/code/ditracker
```

The runner copies regular tracked files from committed revisions into `/tmp`,
excluding environment files and symlinks. It invokes the actual CLI, verifies all
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
| Total plan records | 1,143 | 631 |
| Managed templates | 288 | 267 |
| Managed raw assets | 13 | 13 |
| Existing migrations preserved | 42 | 42 |
| Owner seeds preserved | 520 | 12 |
| Derived output contracts present | 16 | 12 |
| Unknown owner additions preserved | 257 | 278 |
| Private environment contracts | 3 | 3 |
| Shared semantic contracts | 4 | 4 |
| Files requiring reconciliation against the supplied reference | 4 | 4 |

These records are transient inspection results, not files added to either app.
The reference variation changes auth templates only, so this experiment does not
exercise upstream changes in every subsystem. Other subsystem differences are
still inventoried and retain unverified-baseline semantics.

## Auth as a behavioral case study

```sh
python3 scripts/spike-auth-reconciliation.py \
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

Only `oauth_state.go` and the added regression test differ in the final auth source
inventory. Impersonation, Discord flow, identity persistence, and other customized
auth source remain byte-for-byte unchanged. The experiment demonstrates one
reviewed reconciliation and one rejected behavior regression, not a general AI
merger or arbitrary overlapping-method compatibility.

Ship's two untagged auth tests pass. Its broader SQLite auth suite encounters an
existing deployment-policy migration dependency before establishing a baseline.
The experiment does not alter migrations to bypass that limitation.

## Validation and remaining work

Tests cover framework planning without auth, named Apps, all starter-kit choices,
the maximal component inventory without tools, private/configuration projection,
raw assets, migrations, owner seeds, derived output, malformed/unsafe snapshots,
unknown additions, and template-output parity against ordinary rendering.

The root Go suite, vet, focused race suite, and relevant nested modules are
validated with workspace resolution disabled. Before generalizing the scope,
the maximal MySQL composition also passed ordinary render, component contracts,
Wire, build, and generated Go tests. The two protocol peers emit framework-wide
snapshots and preserve the real reference projects' copied bytes and modes.

The inspector does not implement AI reconciliation or publication. The next
steps are historical renderer/input recovery, resource-intent capture without
secrets, semantic dependency/configuration planning, generator-specific staging,
and reviewed overlapping behavior changes beyond the auth case study.

Any future apply path needs staged generation/validation, conflict handling,
compare-before-publish protection, and transactional publication. A passing plan,
compilation, or textual merge cannot establish behavioral compatibility.

This explores reconstructed baselines as an alternative to checked-in source
copies in [Render Ownership And Upgrade Reconciliation](render-upgrade-reconciliation-design.md).
