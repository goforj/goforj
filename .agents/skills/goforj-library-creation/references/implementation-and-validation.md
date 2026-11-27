# Implementation and validation

Read this when planning the API, choosing drivers and module boundaries, building tests, or reviewing readiness.

## Choose precedents by responsibility

Use the current sibling implementations as evidence, not a directory tree to clone wholesale.

| Concern | Useful starting points when those checkouts are available |
| --- | --- |
| Framework versus reusable primitive ownership | GoForj `docs/context/repo-boundaries-and-ownership.md` and the relevant design |
| Several interchangeable backends and optional modules | `cache`, `queue`, `storage` |
| Small API and generated API examples | `collection`, `crypt`, `env` |
| Canonical runnable programs, output-checking runner, and generated inline snippets | `search/examples`, `search/docs/gen`, `search/docs/readme` |
| Shared driver contract and real backend lifecycle | `search/searchtest`, `search/integration`, sibling integration harnesses |
| Exact unit coverage and generated test evidence | `search/scripts/test-unit.sh`, `search/docs/report`, `search/scripts/check-unit-coverage.sh` |
| Independently published nested modules | The chosen sibling's module manifest and release scripts |
| Grouped README navigation and independent public-consumer behavior | `notification/docs/gen/generate.go`, `tables.go`, `consumer.go` and their tests |

Inspect at least the closest domain sibling and a complementary implementation. Notice where they differ. Some older example runners only compile programs; the preferred new-library workflow executes them and verifies output. Some siblings generate programs from GoDoc; search instead keeps programs authoritative and projects documentation from them. Preserve an established generator unless changing ownership is part of the task, and avoid introducing another manually maintained workflow copy.

When comparing external libraries, compare complete consumer workflows and failure semantics, not just feature lists. Record whether the comparison examined documentation, public APIs, implementation, or tests. An API/documentation comparison is not a source-code or security audit. Use findings to challenge missing domain essentials and simplify the API without importing unrelated features.

## Domain and driver boundaries

Start from the smallest complete consumer workflow, including its meaningful failure paths. Use domain-neutral exported names and consumer-owned interfaces. Keep framework registries, environment naming, generated accessors, and deployment orchestration outside the primitive.

Follow the workspace's Go comment and import conventions, and run `gofmt` as files are created, including executable examples and generated Go output. Do not defer basic formatting and documentation until the final review.

Specify ownership for caller inputs, returned collections, injected clients, pools, files, workers, and shutdown. Document concurrent-use guarantees. Required injected dependencies should fail visibly when miswired, rather than being hidden by optional-looking nil guards. Use explicit optionality only where it belongs in the design.

For HTTP-backed drivers, prefer domain-neutral `http.RoundTripper` injection over client-library-specific APIs. Document what is borrowed and caller-owned, which timeout, redirect and response bounds remain library-controlled, and which DNS, TLS, proxy or retry policies the injected transport owns. Test interoperability with a sibling HTTP client through that contract. Debug traces are an explicit caller choice and can expose raw credentials or bodies even when normal library errors are redacted.

Reuse an existing sibling library when its responsibility fits a clean adapter boundary. Notification's optional Mail adapter borrows the GoForj Mailer rather than duplicating email-provider transports; the application keeps ownership and unrelated consumers avoid the dependency. Treat this as an example of composition, not a requirement to force sibling dependencies into every library.

Decide which behavior is portable and which remains backend-specific. Test the portable promise against different backend families before expanding the interface. Depending on the domain, important distinctions include atomicity, acknowledgement versus completion, durability, expiration, count precision, retries, and ordering. Do not make every library implement all of these concepts.

Treat backend responses as protocol evidence. Check required fields, identity correlation, counts, completion indicators, native errors, and partial results as appropriate. A missing numeric field is not proof of zero; HTTP success is not proof every item succeeded. For side-effecting operations, transient failure is not proof of rejection: a receiver can commit work before an error response or an incomplete acknowledgement. Preserve uncertainty so retries, fallback and deduplication do not replay possibly accepted work. Check cancellation during required response-body reads as well as before requests and during transport. Retain positive acceptance where the native contract establishes it from headers alone. Keep classifications safe without leaking server prose or application content.

Bound work before allocations and native calls. Check decoded collection storage as well as wire length; deeply nested arrays can reserve much more memory than their headers suggest. Include a boundary success case and malformed or oversized failure cases in regression tests.

## Modules and compatibility

Inventory every module before changing dependencies or making validation claims. `go test ./...` and `go vet ./...` do not cross nested module boundaries. Select commands per module and use the repository's local-module helper where unpublished sibling code requires it.

Keep the root dependency footprint deliberate. A standard-library driver can remain in the root module; a large optional SDK or a higher minimum Go requirement may justify a separate module. Tooling, examples, and integration harnesses may be isolated without being published.

Prepare the applicable repository surfaces together: `go.mod` and package documentation, `README.md` and the family's license, top-level `examples/`, real-driver integration fixtures, generated documentation and artwork, `.gitignore` entries for local artifacts, and CI workflows. Follow the chosen siblings' source and test layout instead of generating empty directories or placeholder packages.

Provide familiar Make targets where applicable: `test`, `test-race`, `vet`, `tidy`, `modules-check`, `test-integration`, `test-coverage`, `generate`, and `check-generated`. A focused integration target should select a driver without changing the meaning of the full run. Add release planning and verification targets when independent modules need them. Inspect existing commands before naming or invoking them; do not document imaginary targets.

Preserve intentional relative `replace` directives used by repository tests. Distinguish testing unpublished source with local replacements from validating a released consumer with `GOWORK=off` and no local replacements. A workspace can mask invalid published requirements. Test both a combined consumer and each independently published module with its declared transitive minimums; explicitly upgrading the root in the combined case can hide an outdated adapter requirement. Exercise a small meaningful behavior contract as well as compilation where newer root behavior is required. See [public-consumer verification](repository-and-release.md#public-consumer-verification).

Do not raise Go versions merely to match the framework or a sibling's latest line. Identify the exact language or dependency constraint and keep higher requirements scoped to the modules that need them. Use patched toolchains for security verification without increasing the module's minimum language version unnecessarily.

Assess source/API, configuration, persisted format, runtime behavior, operational migration, and minimum Go compatibility separately. Adding a variadic constructor argument can preserve ordinary calls while breaking assignments to its original function type. Account for that before claiming full source compatibility. New security capabilities must not silently reinterpret or rewrite existing plaintext or encrypted files.

## Real integration tests

For a library with drivers, provide both shared semantic fixtures and native backend tests:

- Run the same mandatory conformance inventory for every supported driver. Cover the operations promised in the driver table, including failure and lifecycle behavior that the domain exposes.
- Use Testcontainers for disposable services where supported. Own containers at package-run scope, typically through `TestMain` and a shared fixture pool. Start one container per service or genuinely distinct configuration, not one per subtest.
- Give tests isolated namespaces, tables, indexes, buckets, or accounts inside shared services. Keep repeated and concurrent test runs isolated too.
- Register cleanup when resources become available, including partial startup. Use bounded readiness and cleanup contexts and propagate cleanup failures.
- Pin backend versions and preferably image digests. Verify the actual running version before generating version-specific support claims.
- Test authentication and permissions, native failures, malformed protocol responses, cancellation, shutdown, and data ownership where applicable. Mocks complement these tests; they do not replace real engine coverage.
- Missing Docker or required credentials fails the required verification or leaves a clearly pending result. Do not turn infrastructure absence into a silent skip and then claim support.
- A hosted driver needs a usable dedicated test environment. If it is unavailable, resolve scope with the user. Respect exclusions; do not add the provider anyway or silently substitute an emulator for hosted verification. When the user already authorized native drivers with local protocol tests and hosted verification pending, implement and label that scope without repeatedly requesting test accounts. A pending hosted check must remain visible and must not be reported as live verification.

SQL and embedded drivers should exercise their actual engine and relevant extensions. For injected resources, prove that library shutdown preserves caller ownership. For native schema management, prove validation detects actual drift and that ordinary request paths do not perform hidden migration.

Run offline examples by default. Credentialed provider programs can compile in normal tests and run explicitly when configured; document that distinction. Do not load developer credentials during offline tests or fabricate verified hosted output. Emulator success does not establish hosted IAM, KMS, regional, or retention behavior. Cross-compilation proves build compatibility; runtime claims need observed execution for the exact source or CI commit.

## Examples, coverage, and final checks

The runner must execute the programs users see, compare meaningful output, and fail for a missing output expectation. Format structs and nested configuration across readable lines. Keep expected output immediately after the producing call for readers; a final machine-readable output block may serve the runner. Use one comment per output line. Do not use quoted strings solely to make ordinary output look testable.

When programs run as subprocesses, ordinary parent `go test -cover` does not capture their library execution. Instrument the built or run programs, collect `GOCOVERDIR` data, and merge it using Go's coverage tools. Pass race instrumentation to the child programs during race runs as well. Keep unit and integration coverage separate even when also publishing a combined report.

Require the unit target using exact covered and total statement counts. A display rounded to `100%` is insufficient. Include every relevant first-party driver module; do not use package exclusions or integration execution to hide unit gaps. Diagnose toolchain and OS differences explicitly. If combining profiles from several platforms, use compatible instrumentation and preserve the provenance of each execution.

Count executed tests from `go test -json`, including subtests. Generate evidence and badges from passing events and actual coverage, not declarations or a prior run. Bind driver support evidence to relevant source and pinned backend versions when maintaining a generated matrix. A focused driver run must not overwrite complete all-driver evidence.

Make cancellation and concurrency regressions deterministic. A goroutine announcing that it started does not prove it reached a blocked acquisition or publication checkpoint. Coordinate the relevant checkpoint when behavior or coverage depends on reaching it. For persistent security-sensitive storage, use the additional criteria in [security and persistence](security-and-persistence.md).

Run the meaningful module tests, race checks, vet, tidy parity, module-manifest checks, generated parity, and vulnerability checks. Add benchmarks when performance goals or claims need evidence. Verify CI uses the same commands and inventories. Broaden testing after new changes or unresolved failures, rather than repeatedly rerunning passed checks without a reason.
