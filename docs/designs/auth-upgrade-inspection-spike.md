# Auth Upgrade Inspection Spike

Status: experimental, inspection only.

## Question and result

Can one user-facing `forj` inspect unversioned application auth, compare renderer
versions, and distinguish app-derived output without checking baseline source
copies into application repositories?

This spike demonstrates that narrow workflow. It reconstructs template output
in memory, reports exact byte differences and changed Go declarations, and can
ask other renderer executables for comparison snapshots. It does not reconcile
or apply changes. It does not prove that AI can preserve arbitrary application
behavior during an upgrade.

The implementation intentionally leaves the current generated auth API, schema,
configuration, runtime behavior, dependencies, and minimum Go version unchanged.

## Commands

Run these from an existing application's root using a binary built from this PR:

```sh
forj render --only auth --plan
forj render --only auth --plan --json
```

The JSON plan contains paths, exact-byte fingerprints, declaration names, and
actions. It does not contain application source or private environment values.
The command performs no render writes, generation, migration, dependency
resolution, Wire invocation, application build, or application startup.

One user-facing binary can orchestrate two protocol-compatible renderer binaries:

```sh
forj render --only auth --plan --json \
  --from-renderer /path/to/comparison-forj \
  --target-renderer /path/to/target-forj
```

The target defaults to the user-facing binary. Executables run in disposable
workspaces containing only projected, non-secret render configuration. They do
not receive app source, private environment files, owner configuration extras,
or arbitrary inherited process variables. Executables are invoked directly,
without a shell, with a timeout and bounded output. No global CLI installation or
authentication configuration changes. The hidden `render --only auth --snapshot`
command is the experimental JSON protocol between executables. Its template
output is transient; applications need not commit it.

Released older binaries do not implement this new protocol. Automatic binary
download, historical-release adapters, caching, and version selection are future
work. An incompatible executable produces an error before the app is changed.

## Ownership and comparison rules

The ordinary renderer and the inspector share the authoritative auth template
catalog. The catalog owns individual paths, never all of `internal/auth`.

| Observed artifact | Plan treatment |
| --- | --- |
| Current bytes equal target output | `matches-target` |
| Unversioned bytes differ from target | `review-unversioned` |
| Expected path is absent | `review-missing`, including possible owner deletion |
| Current equals comparison baseline | `upstream-only-relative-to-baseline` |
| Target equals comparison baseline | `preserve-local-relative-to-baseline` |
| Both differ from comparison baseline | `reconcile` |
| Incoming path collides with existing source but has no prior baseline | `review-collision` |
| Comparison baseline retires a path | `review-retirement`, never automatic deletion |
| Neighboring file absent from both catalogs | `preserve` with unknown ownership |
| Known app-derived output | `regenerate-after-reconciliation` |

The derived context currently covers default-App Wire output and the cache,
queue, and storage manager/accessor generator contracts. It does not infer
ownership from `_gen.go`. A neighboring `custom_gen.go` remains unknown and is
preserved. Derived files are not compared with a blank application's output,
and this inspector does not assert whether their existing bytes are correct.

An eventual upgrade must regenerate derived output from the reconciled app's
actual inputs, then validate the result.

Exact-byte comparisons can identify formatting-only changes. Declaration hints
ignore comments and most formatting, but can still contain non-semantic noise,
such as an equivalent string-normalization implementation. Declaration overlap
is neither a conflict verdict nor a security compatibility verdict. A test
explicitly proves that disjoint declaration changes still require reconciliation.

## Provenance and scope

An unversioned app has unknown provenance. Supplying a comparison renderer does
not authenticate the app's original baseline: the plan labels it
`supplied-unverified`. Both renderers receive current app inputs. Historical
configuration changes remain unknown, and differing input digests are rejected.

The snapshot records actual binary version, commit, and dirty metadata rather
than reusing `version.Semver()`, which identifies the scaffold/config contract.
These fields are diagnostics, not cryptographic proof of reproducibility.

The current scope is a default Auth App's shared auth templates, the mail auth
adapter, neighboring auth files, and selected derived context. Named-only Auth
Apps, wiring reconciliation, routes, frontend contracts, schema migrations,
environment reconciliation, and broader framework ownership remain outside this
spike. The command explicitly reports these boundaries.

Private environment layers are not inspected or included in snapshots. Known
artifact paths reject symlinks, non-regular files, and oversized contents.
Unexpected neighboring files are listed for preservation without reading them.

## Reference-app experiment

Build the CLI, then run the reproducible reference runner:

```sh
go build -o /tmp/forj-auth-upgrade ./cmd/forj
python3 scripts/spike-auth-upgrade.py \
  --forj /tmp/forj-auth-upgrade \
  --project ~/code/ship \
  --project ~/code/ditracker
```

The runner copies regular tracked files from each committed revision into a
fresh workspace under `/tmp`. It excludes environment files and symlinks,
invokes the actual CLI, and verifies that every copied file's bytes and mode
remain unchanged. It stores detailed plans and a small summary in that temporary
workspace. It does not inspect a deployed application or prove live behavior.
Use `--from-renderer /path/to/comparison-forj` for the two-renderer experiment.

Reference revisions used during the spike:

- Ship: `f3eb4bae25cbc811025a706c17af1a0f287f33b6`.
- Ditracker: `6fe4b4842a08b131ba6723d5ce16fa3dcc431e49`.
- Framework source before spike changes: `5ab78874`.

Unversioned inspection produced these results, with unchanged copied source:

| Action | Ship | Ditracker |
| --- | ---: | ---: |
| Matches target | 4 | 12 |
| Review unversioned difference | 17 | 10 |
| Review missing file | 1 | 0 |
| Preserve unknown neighbor | 3 | 4 |
| Regenerate derived output later | 7 | 7 |

The Ship plan identifies session sign-in fields, their cache serialization,
session issuance, and organization bootstrap. It preserves
`session_sign_in.go`. Ditracker's session template matches exactly, while the
service plan identifies impersonation integration, Discord guild snapshots,
silent-login fallback, and its session idle policy. The Discord provider and
impersonation implementation/query/test files remain preserved neighbors.

For a second executable, the experiment built the new snapshot emitter with
historical auth templates from `68f1de1463e2379f20d63c3329380f0116587074`.
This is explicitly a historical-template replay using the current emitter,
not execution of that revision's original released renderer. All templates in
the scoped catalog were replayed; emitter and configuration handling remained
current. The binary was labeled as a dirty experimental replay.

With that comparison renderer, both apps produced four `reconcile` files.
Ship's service overlap includes `beginOAuth` and `CompleteOAuthCallback`;
Ditracker's includes those methods and other provider/state helpers. Each app
still had seven derived outputs marked separately, and both copied trees
remained unchanged in bytes and modes. This proves the subprocess protocol and
comparison plumbing, not original baseline discovery or semantic reconciliation.

## Controlled behavioral reconciliation

The second runner probes a real persistence behavior change already present in
Ship: consuming an OAuth state conditionally, checking expiration and requiring
exactly one affected row. The change is applied only to a disposable Ditracker
copy. It is not included as a production auth change in this PR.

```sh
python3 scripts/spike-auth-reconciliation.py \
  --forj /tmp/forj-auth-upgrade \
  --ditracker ~/code/ditracker
```

The runner uses SQLite and an explicit public test-only JWT key. Results:

| Phase | Result |
| --- | --- |
| Existing auth suite before changes | 103 tests/subtests passed |
| Add replay and expiration regression tests before fix | Both regressions fail behaviorally |
| Apply conditional OAuth-state consumption | All three regression test/subtest results pass |
| Replace only `AuthenticateRequest` with the default framework implementation | Compilation succeeds; the impersonation round-trip test fails |
| Restore app-owned request authentication, retain the state fix | Full suite passes with 106 tests/subtests |

Only `oauth_state.go` and the added regression test differ in the final auth
source inventory. Discord/provider logic, identity persistence, impersonation,
session identity tracking, and the app's other auth source remain byte-for-byte
unchanged. The intentionally destructive replacement is restored in the
disposable copy; the original repository is never modified.

This is a scripted, reviewed single-change experiment. It proves that a useful
shared fix can coexist with this app's custom flows, and that its existing tests
can catch one behaviorally incorrect replacement that still compiles. It does
not prove a general AI reconciler, arbitrary overlapping method merges, or
complete security coverage.

Ship's two untagged auth tests also pass. Its broader auth suite could not
establish a SQLite baseline: an existing generic deployment-policy migration
requires a table unavailable in that dialect's migration stream. This experiment
does not modify migrations to bypass that limitation and makes no claim about
Ship's full runtime suite.

Framework validation passes the root module's test suite and vet, the focused
race suite, and relevant nested-module compile/test checks with workspace
resolution disabled. The pure comparison package has 97.1% statement coverage.
The `sentinel_max_mysql` render case also passes with Auth, OAuth, Web API/UI,
mail, metrics/observability/Grafana, scheduler, cache, events, storage, and jobs
enabled: render, component contracts, Wire, build, and generated Go tests.
Concurrently loaded full test runs hit CLI-help timeouts; the CLI package and
complete runs with reduced package concurrency passed, including a serial
package run after rebasing onto current main.

## Required next proof

Extend the behavioral experiment to an actual overlapping security/workflow
change and to Ship's supported integration database. Add staging regeneration,
frontend contract checks, and a deliberately incompatible schema/configuration
transition. The two-renderer replay does not prove historical input recovery.

The AI should inspect full current and incoming source, surrounding callers,
migrations, frontend contracts, and regression tests. It must produce a
customization inventory, a proposed reconciliation, and behavioral evidence.
This PR supplies inspection inputs; it supplies no automatic AI judgment.

Do not add `--apply` until there is staged generation/validation,
compare-before-publish protection, explicit unresolved-change handling, and
transactional publication. No successful plan or clean textual merge is proof
that an auth upgrade preserves behavior.

This explores an alternative baseline strategy to the checked-in source store
in [Render Ownership And Upgrade Reconciliation](render-upgrade-reconciliation-design.md).
