# Search reference implementation and acceptance checks

Use this for a full library build or when comparing a result with the search effort. Locate the current `search` sibling checkout, or inspect the corresponding files in `goforj/search`. These paths identify working implementations; inspect their current contents before adapting them. Do not copy search-specific semantics, versions, test counts, or historical defects into a new domain.

## Working artifact map

| Deliverable | Search reference | Outcome to reproduce |
| --- | --- | --- |
| Portable consumer API | `doc.go`, `index.go`, `document.go`, `errors.go` | A useful standalone workflow, clear ownership and errors, and deliberate portable guarantees |
| Driver implementations | `driver/`, `internal/driverutil/`, `internal/sqlsearch/` | Concrete clients with bounded protocol handling and truthful native behavior |
| Optional dependency isolation | All `go.mod` files, `go.work`, `scripts/module-manifest.txt`, `scripts/local-modfile.sh` | Published and tooling modules are inventoried, and repository tests exercise unpublished source deliberately |
| Isolated application testing | `searchfake/`, `examples/fake/main.go` | An honest scripted fake with owned inputs and responses, rather than a pretend production engine |
| Shared driver conformance | `searchtest/searchtest.go`, `searchtest/searchtest_test.go` | A named required scenario inventory whose assertions are themselves checked against faulty behavior |
| Real backend lifecycle | `integration/main_test.go`, `integration/drivers_test.go`, native fixture files | Shared service containers, isolated test namespaces, bounded startup and cleanup, real embedded engines |
| Pinned backend evidence | `integration/versions.json`, `integration/versions.go` | Version-specific claims verified against the actual running engine |
| Canonical examples | `examples/*/main.go`, `examples/example_test.go` | One workflow implementation per program; tests execute it and check expected output |
| Child-process instrumentation | `examples/race_test.go`, `examples/norace_test.go`, `scripts/test-unit.sh` | Example subprocesses contribute real unit coverage and participate in race runs |
| Documentation generation | `docs/gen/gen.go`, `docs/readme/main.go` and their tests | GoDoc indexing, annotated snippet extraction, curated API entries, and matching inline targets |
| API index and section layout | `docs/readme/main.go`, `docs/readme/main_test.go`, `README.md` | A curated Group/Functions table with matching category sections, API-named headings, and unique resolving anchors |
| Realistic quick start | `examples/quickstart/main.go` | A package-scope `Product` model, consistent schema and data, and readable verified output |
| Feature and driver presentation | `docs/gen/presentation.go`, `docs/assets/icons/`, `docs/assets/badges/` | Locally owned SVG artwork; use the current icon-plus-searchable-text table treatment described in documentation and branding |
| Mascot logo | `docs/assets/logo.png`, `docs/assets/logo-prompt.txt` | A transparent original family-style mascot, readable lowercase wordmark, and retained creation context |
| Executed test and coverage reports | `docs/report/`, `docs/unit-results.json`, `docs/integration-results.json`, `docs/coverage.json` | Machine-derived evidence rather than declaration counts or hand-edited percentages |
| Driver support freshness | `docs/gen/evidence.go`, `docs/driver-results.json`, `SourceFingerprint` in `docs/gen/gen.go` | A support table tied to complete successful scenarios and relevant source identity |
| Local developer commands | `Makefile`, `scripts/test-*.sh`, `scripts/tidy-modules.sh` | Familiar commands that validate every relevant module and support focused driver investigation |
| Generated parity | `scripts/check-generated.sh` | Two generations leave README, examples, and generated badge assets unchanged |
| CI and vulnerability scanning | `.github/workflows/ci.yml`, `.github/workflows/security.yml` | Minimum-version checks, full quality checks, real integrations, and patched-toolchain security verification |
| Strict Codecov policy | `codecov.yml`, `scripts/check-unit-coverage.sh`, coverage-upload steps in CI | Separate unit evidence, exact statement enforcement, project and patch checks, and failed uploads surfaced |
| Driver performance | `integration/performance_test.go`, `scripts/bench-drivers.sh`, `docs/bench/README.md` | The same deterministic corpus and query schedule, validated results, raw samples, and explicit timing boundaries |
| Performance projections | `docs/bench/benchmarks_rows.json`, `docs/bench/render.go`, `docs/bench/render_test.go` | A validated measurement snapshot generates matching SVG charts and README results without rerunning engines |
| Performance investigation | `integration/sql_performance_audit_test.go`, `docs/bench/sql-audit.md` | Suspicious results checked against samples, executed SQL, native plans, and isolated controls before a fresh comparison |
| Independent module release | `scripts/tag-all-modules.sh`, `scripts/check-public-release.sh`, `docs/development.md` | Explicit tag planning and validation of an actual published consumer outside the workspace |

GitHub repository description, topics, app installation access, repository secrets, and native webhooks are external state. Their presence cannot be reproduced by copying these files. Use the authorized setup workflow and verify their current live state separately.

## Acceptance checks for the completed library

Use the applicable rows as a completion record. Mark an item passed only after executing its check; distinguish pending external setup from local work. A pure utility library can mark driver-only rows inapplicable with a concrete reason.

| Surface | Evidence required |
| --- | --- |
| Scope | Implementation meets the intended domain workflow as well as the agreed boundary and exclusions; missing core guarantees are resolved, and framework wiring is separately scoped |
| Primary workflow | The visible quick start runs from its stated location and prints its documented result |
| Examples | Advertised offline programs execute with checked output; credentialed programs compile and have explicit run prerequisites and verification status; no duplicate workflow implementations are maintained |
| Coverage | Exact unit statement coverage meets policy across all relevant first-party modules; integration execution does not supply missing unit coverage |
| Race behavior | Race tests include the real example subprocesses and relevant library modules |
| Driver support | Every supported driver passes the shared inventory and applicable native tests; missing infrastructure and skipped tests are not advertised as support |
| Service lifecycle | Each service or required configuration is started once per package run, all its tests reuse it safely, and cleanup is verified |
| Failure handling | Regression tests cover malformed protocol evidence, cancellation, native rejection, resource boundaries, and ownership relevant to the domain |
| Generated content | Regenerating twice produces no diff; changing relevant source invalidates stale support evidence where fingerprints are used |
| API navigation | Grouped navigation matches the category sections below; each API links to a plain-text API-named heading whose own section contains the demonstrating snippet |
| Visuals | Transparent logo, wordmark, SVG badges, alignment, and light/dark contrast have been inspected at actual display size |
| Asset delivery | README image and badge paths load in GitHub rendering, with truthful unreleased/version status |
| Performance, when requested | Comparable runs use the same corpus and requests, results are validated, measurements retain provenance, and recorded data regenerates all charts and README figures |
| Security | Relevant modules pass the pinned scanner on a patched supported toolchain; credentials and server prose stay out of output; applicable persistence, encryption, and key-ownership guarantees are reviewed separately from coverage |
| Repository integration | Authorized Codecov access, uploads and checks, description/topics, and notification routes are verified against actual external state |
| Public installation | A fresh public-proxy consumer resolves the pushed revision with normal checksums; combined and independent runtime modules pass without replacements or rewritten requirements |
| Publication | Only when requested: every published module version resolves independently, and a clean consumer passes with `GOWORK=off` and no replacements |
| Release notes, when publishing | A concise benefit-led announcement has readable sections, installation instructions, actionable upgrade guidance, and a link to detailed verification |
| Handoff | Focused Conventional Commits and exact local, pushed, CI, and publication status are reported accurately |

## Lessons from the review passes

Use these as examples of the kinds of defects a review should find, not as assumptions that every new library contains them:

- A numeric score or count missing from a response must not silently become a proved zero. Meilisearch and Typesense regression tests demonstrate presence checks and malformed-response failures.
- Response byte limits alone do not bound decoded memory. Redis's RESP tests demonstrate reserving aggregate element storage against the allocation budget before creating a slice.
- A working README anchor can still point to a snippet that does not use the named API. The API generator tests demonstrate checking the association rather than just the target's existence.
- A syntactically valid Go example without a recognized output contract may compile without being executed. The canonical program runner makes execution and output verification explicit.
- Toolchain and OS differences can expose different coverage paths. Verify the claimed platform and exact statement coverage rather than changing a percentage or excluding an inconvenient branch.

The skill has structural validation and source-backed guidance. Those checks do not prove an agent can reproduce the whole result unaided. A meaningful behavioral evaluation is to build a small fresh library in an isolated temporary workspace, exercise its applicable acceptance checks, and record omissions. Run such an evaluation when requested or otherwise authorized; do not publish repositories or configure services as a side effect of evaluating the skill.
