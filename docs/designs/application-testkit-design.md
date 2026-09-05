# GoForj Application Testkit Design

## Status

- Design status: proposed
- Planning date: 2026-09-05
- Target repository: `goforj`
- Primary package scope: a new public `github.com/goforj/goforj/forjtest` package for lifecycle, HTTP requests, responses, test configuration, resource cleanup, and capability-neutral assertions
- Primary generated scope: an alternate injector in `internal/wire` plus an application-local `internal/testapp` facade that composes the generated App, routes, database, existing sibling-library fakes, Seed scenarios, Auth helpers, and optional Audit capture
- Cross-repository scope: small compatibility additions to sibling-library fakes only where the canonical public interface cannot currently be injected or inspected safely
- Cross-repository source of truth: this checked-in design is normative until implementation documentation supersedes it

## Summary

GoForj should provide a first-class application testkit for tests that need more confidence than an isolated controller test but less infrastructure and process overhead than a full rendered end-to-end environment.

The intended test reads like application behavior:

```go
func TestCreateTicket(t *testing.T) {
	app := testapp.New(t)
	user := app.Auth().CreateUser("agent@example.test")
	app.Auth().AsUser(user)

	response := app.HTTP().PostJSON("/api/v1/tickets", map[string]any{
			"subject": "Unable to sign in",
		})

	response.RequireStatus(http.StatusCreated)
	app.QueueProbe().RequireDispatched(t, "ticket:index")
	app.Events().AssertCount(t, 1)
}
```

The exact assertion names will follow the real sibling APIs and the repository's minimum Go version. The important shape is:

- construct the same generated application graph used in production;
- replace external side effects with canonical fakes deliberately;
- run the real middleware and route stack in-process through `net/http/httptest`;
- isolate database and filesystem state per test;
- establish authenticated users without production backdoors;
- run deterministic Seed scenarios when requested;
- expose fakes for direct, typed assertions; and
- register cleanup immediately with `testing.TB`.

Testkit is not another test framework and should not replace Go's `testing` package, `web/webtest`, Testcontainers, sibling-library contract tests, or browser end-to-end tools. It is the missing composition layer for a generated GoForj application.

## Decision

Implement application testing inside the `goforj` module rather than creating a general-purpose sibling repository.

Adopt these decisions:

1. The public package is `github.com/goforj/goforj/forjtest`; generated application-specific composition lives in `internal/testapp`.
2. Tests continue to use `testing.T`, `testing.B`, and `go test`. Testkit accepts `testing.TB` and registers cleanup; it does not introduce suites, global registries, custom test discovery, or a new runner.
3. Testkit is a project testing capability, not a runtime App component. It adds no production environment key, runtime accessor, service, or process.
4. A generated alternate Wire injector lives in `internal/wire`, where it can reuse the existing unexported provider sets. `internal/testapp` calls that injector and exposes the ergonomic harness facade. Neither layer mutates private fields on a production App after construction or uses a service locator.
5. The alternate composition root reuses production providers wherever behavior should remain real and supplies explicit test providers only for external effects, clocks, paths, and isolated persistence.
6. In-process HTTP uses the production route groups and middleware through an `http.Handler` seam. It does not call controller methods directly or bind a real TCP port by default.
7. Queue, mail, and Audit are capture-only by default. Events use the canonical synchronous recording bus and run production subscribers by default; capture-only events are an explicit mode. Background workers and the scheduler do not start implicitly.
8. Existing canonical sibling fakes remain authoritative. Testkit exposes them; it does not reimplement their semantics or wrap every assertion in a GoForj-specific synonym.
9. A test gets fresh fake instances, temporary paths, cookie jar, logical test context, and database namespace. No mutable package-global test state is shared.
10. Database isolation uses a unique database or schema per harness where supported. It does not rely on one outer rollback transaction because HTTP handlers, workers, and nested transactions may use different connections.
11. SQLite uses a unique temporary file by default rather than a fragile connection-local `:memory:` database.
12. MySQL, PostgreSQL, Redis, and other external services are explicit test resources. The default constructor never discovers Docker and starts containers unexpectedly.
13. An optional Testcontainers adapter can provision declared resources and must register cleanup immediately. Existing project resource metadata remains the source of truth for image and topology choices.
14. The core `forjtest` package keeps a standard-library-only import surface. Container integration lives in `forjtest/containers` in the existing root module, which already pins Testcontainers for Go.
15. Test configuration is passed through an immutable construction snapshot. Testkit does not implement ordinary isolation by repeatedly mutating process environment variables.
16. Explicit environment mutation uses `testing.TB.Setenv` and is documented as incompatible with parallel tests because the Go testing contract says it affects the whole process.
17. `t.Context()` drives application and resource lifetime where the supported Go version provides it. Cleanup uses an independent bounded context because the test context is cancelled before cleanup callbacks run.
18. Authentication helpers either exercise the real login route or use a generated test-only session issuer wired inside `internal/testapp`. They never ship an HTTP backdoor or accept an unsigned actor header.
19. Testkit does not globally freeze `time.Now`. It injects a clock only through code that declares a clock dependency. Seed scenarios receive their existing explicit application time.
20. The harness returns initialization failures through `t.Fatalf` only at the outer ergonomic `New(t)` boundary. Lower-level constructors return errors so package tests and tools can inspect failure behavior.
21. HTTP assertion failures include method, path, status, selected headers, and a bounded body. They redact cookies, authorization headers, tokens, and configured sensitive fields.
22. Testkit never treats recorded side effects as executed effects. A queued job capture proves dispatch intent, not successful job processing.
23. A separate explicit worker mode may run real queue workers against isolated infrastructure. It is not implemented by teaching the queue fake to execute handlers.
24. Seed integration consumes the proposed `github.com/goforj/seed` scenario contracts when available. Testkit does not invent a second fixture or scenario DSL.
25. Audit integration consumes `auditfake` when Audit is selected. Testkit does not store Audit records in its own generic event capture.
26. The minimum useful release supports in-process HTTP, isolated SQLite, Auth sessions, mail/events/queue capture, temporary storage, cleanup, and readable failures before adding container orchestration.

## Why This Belongs In GoForj

### The missing layer is generated composition

Most individual GoForj sibling libraries already have appropriate local drivers or fakes:

- `events` has a recording fake and typed publication assertions;
- `queue` has a recording fake for direct and workflow dispatch;
- `mail` has a fake driver and sent-message inspection;
- `cache` has a fake package;
- `storage` has local and memory-capable behavior; and
- `web` has `webtest` for focused handler tests.

What they cannot know is how a specific generated application wires:

- its App and named primitive managers;
- its repositories and database connections;
- its controllers and middleware;
- its Auth service and cookies;
- its routes and starter UI;
- its lifecycle hooks;
- its Seed scenarios; and
- its application-owned services.

That composition is generated by GoForj, so GoForj must own the testing seam.

### Why not a sibling `testkit` repository

A generic sibling package would either know too little to construct a generated App or would couple its release to GoForj's internal templates. Keeping `forjtest` in the GoForj module provides one version boundary for the public harness vocabulary and the generated code that implements it.

Sibling repositories continue to own their own fakes and conformance tests.

### Why not only `webtest`

`web/webtest` is correctly scoped to a handler and a lightweight `web.Context`. Application tests need the full route and middleware path, database migrations, auth cookies, named primitives, and generated dependency graph. Testkit builds upon `web`; it does not expand `webtest` into framework-specific composition.

### Why not only rendered end-to-end tests

Rendered and containerized tests remain essential for generator, migration, driver, and operational confidence. They are too expensive and cumbersome for every application behavior test. Testkit provides a fast in-process middle layer while preserving an escalation path to real services.

## Motivation

### Current application tests reconstruct framework wiring manually

Generated Auth and demo integration tests currently assemble repositories, services, cookies, databases, and managers directly. This provides coverage, but application authors must understand framework internals before writing a realistic feature test.

Manual setup tends to drift from production composition and produces several problems:

- middleware is skipped;
- a fake is injected at the wrong layer;
- cleanup is incomplete after a fatal assertion;
- tests share SQLite files or global environment;
- auth is simulated with trusted values that production would never accept;
- queued work is accidentally executed or silently dropped; and
- test failures dump raw JSON rather than the behavior that differed.

### A coherent test can exercise a coherent scenario

Seed scenarios and Testkit serve different roles:

- Seed creates deterministic application state through application-owned services.
- Testkit constructs and isolates the application around that state.

Together they make product behavior reproducible without requiring every test to know table order or primitive driver configuration.

### Go already supplies the runner and lifecycle hooks

The standard `testing` package already provides subtests, deadlines, contexts, temporary directories, environment restoration, and cleanup callbacks. Testkit should use those facilities rather than obscure them. In particular, Go documents that `Setenv` affects the whole process and cannot be used in parallel tests, while test contexts are cancelled before cleanup callbacks:

- [Go `testing` package](https://pkg.go.dev/testing)
- [Using Subtests and Sub-benchmarks](https://go.dev/blog/subtests)

## Goals

1. Make full generated-application behavior tests concise and readable.
2. Exercise production route, middleware, controller, service, repository, and serialization behavior in-process.
3. Reuse production composition and canonical sibling-library fakes.
4. Isolate mutable state per test and clean it up reliably after failures.
5. Make Auth, Seed, Audit, mail, events, queue, cache, storage, and named instances composable without a universal service locator.
6. Avoid hidden networks, ports, containers, goroutines, workers, and scheduler activity.
7. Support deliberate escalation from local fakes to real service-backed integration.
8. Keep HTTP requests and assertion failures human-readable and secure.
9. Preserve context cancellation, request metadata, metrics, and Lighthouse behavior where the selected test mode enables them.
10. Support default and named Apps.
11. Allow parallel tests when they use isolated resources and avoid process-global environment mutation.
12. Make generated test code rerender-safe and ownership-aware.
13. Keep the public package useful to generated applications without exposing GoForj internals.

## Non-goals

1. Replacing Go's `testing` package or introducing a suite runner.
2. A general assertion library competing with `testify`, `go-cmp`, or standard testing helpers.
3. Browser DOM, JavaScript, visual regression, accessibility, or Playwright testing.
4. Load testing, fuzz orchestration, chaos testing, or production probes.
5. Replacing sibling-library unit tests and driver conformance suites.
6. Automatically starting every service from `docker-compose.yml` during ordinary `go test`.
7. Mock generation or reflection-based dependency replacement.
8. Runtime monkey patching, unsafe field mutation, or package-global dependency overrides.
9. Running captured queued jobs as though a real worker completed them.
10. Automatically executing scheduled work.
11. Globally replacing `time.Now`, cryptographic entropy, DNS, or the filesystem.
12. Creating domain models through reflection or direct generic table insertion.
13. Guessing credentials or bypassing authorization middleware.
14. Keeping one mutable shared database and cleaning tables heuristically between tests.
15. Hiding migration failures or silently falling back to SQLite when a requested dialect is unavailable.
16. Making private application types importable outside the generated module.
17. Shipping test-only HTTP endpoints in the production route graph.
18. Treating a fake assertion as end-to-end provider evidence.

## Test Layers

GoForj should document four distinct layers.

### Unit test

Tests one service or helper with explicit collaborators. It does not need Testkit.

### Handler test

Uses `web/webtest` to exercise one handler's binding and response behavior without route or middleware composition.

### Application test

Uses `internal/testapp` plus `forjtest` to exercise the generated application graph in-process with isolated persistence and selected fakes. This design owns that layer.

### Infrastructure or browser test

Runs real drivers, processes, containers, or a browser. Existing rendered integration and frontend tooling own this layer.

Passing an application test does not replace infrastructure-driver or browser evidence.

## Terminology

### Harness

One test-scoped running application composition plus its isolated resources, HTTP client, and probes.

### Test App

The generated application-local composition package that knows concrete App types and providers.

### Provider

A construction function for one production or test dependency. Providers remain compile-time-visible to Wire.

### Override

An explicit selection made before composition, such as capture mail, use a temporary storage root, or bind a supplied clock. It is not a mutable map of arbitrary dependencies.

### Probe

A typed handle used to inspect a fake, recorder, or captured output after behavior runs.

### Resource

A test-scoped database, directory, server, container, or other lifecycle-owned dependency.

### Local mode

The default mode using in-process handlers, temporary paths, isolated SQLite where available, and capture fakes.

### Service mode

An explicit mode that provisions or connects to real declared dependencies such as PostgreSQL or Redis.

### Execution mode

Whether background queue workers, scheduler tasks, or other runtime hosts remain stopped or are explicitly started.

## Repository Ownership

### `goforj/forjtest`

The public package owns:

- `testing.TB` lifecycle integration;
- immutable harness configuration contracts;
- cleanup ordering and bounded cleanup contexts;
- in-process HTTP request building;
- cookie-jar handling;
- response capture, decoding, and safe assertions;
- temporary path and resource vocabulary;
- provider-neutral clock and entropy contracts where needed;
- capability detection and informative unsupported errors;
- safe failure formatting and redaction;
- common probe interfaces that do not duplicate sibling APIs; and
- public documentation and examples that do not import a particular generated application.

### Generated `internal/testapp`

The generated package owns:

- the ergonomic `New` and `Open` constructors;
- conversion of generated test options into the injector's typed configuration;
- wrapping the returned composition as a typed harness;
- application-specific capabilities;
- Auth, Seed, and probe convenience adapters; and
- startup selection;
- resource planning, acquisition, namespace creation, and generated migration execution; and
- passing typed resource handles into the alternate injector.

### Generated `internal/wire` test injector

The generated Wire package owns:

- the concrete alternate injector source and generated result;
- access to, and reuse of, production provider sets;
- explicit fake providers;
- generated App and HTTP handler access;
- named primitive fake maps;
- construction of the App graph from already-acquired typed resources;
- the returned typed `TestComposition` containing the App, HTTP server, probes, and graph-owned cleanup; and
- adapter code from generated internals to public `forjtest` contracts.

### Sibling libraries

Each sibling owns:

- its canonical fake and assertion semantics;
- safe record snapshots;
- reset and concurrent access behavior;
- driver-specific integration tests; and
- any interface compatibility required for intentional dependency injection.

Testkit should request narrow upstream changes when necessary rather than maintaining forks in generated templates.

### Generated application

The application owns:

- domain-specific factories and assertions;
- which Seed scenarios tests invoke;
- application credentials used only in test data;
- explicit real-service tests;
- browser tests;
- test-specific external API servers;
- whether selected background workers run; and
- any custom provider exposed through the App's normal injection extension points.

## Core Invariants

1. Every ergonomic harness returned by `New` belongs to exactly one `testing.TB` and registers cleanup before resource acquisition; the lower-level `Composition` returned by `Open` is explicitly caller-owned.
2. Construction either returns a complete harness or cleans every resource acquired before failure.
3. Cleanup executes in reverse dependency order and aggregates errors without skipping later cleanup.
4. Cleanup uses a new bounded context after the test context is cancelled.
5. A local harness binds no externally reachable TCP port unless the test explicitly requests an `httptest.Server` URL.
6. Default construction starts no queue worker, scheduler loop, or unrelated runtime host.
7. Every mutable fake and temporary path is instance-scoped.
8. Named primitive instances remain distinct and are exposed by their generated names.
9. HTTP requests traverse the same route and middleware graph as production.
10. Test-only authentication never adds a production route or accepts a client-controlled trusted identity.
11. Database migrations complete before Seed or HTTP execution begins.
12. Database isolation never depends solely on an outer transaction invisible to nested connections.
13. A requested real driver never silently falls back to a fake or different dialect.
14. A captured queue dispatch is never reported as a processed job.
15. A captured event publication retains the sibling fake's actual dispatch/error semantics.
16. A failed assertion emits a bounded, redacted diagnostic.
17. No secret from headers, cookies, environment, DSNs, mail, Audit, or Auth appears in ordinary failure output.
18. Testkit does not mutate process environment unless the test explicitly requests `WithEnv`.
19. Parallel-safe mode rejects process-global overrides and non-isolated shared resources during validation.
20. App startup and shutdown are idempotent under the same guarantees as production.
21. The production initializer, runtime graph, binary behavior, and component defaults are unchanged when no test package is imported.
22. Component-off generated applications omit typed capability methods and their imports entirely; option requests for absent capabilities fail with an informative unsupported-capability error before acquisition.

## Architecture

```text
testing.T
    |
    v
generated internal/testapp.New
    |
    +-- immutable test Config
    +-- internal/wire alternate injector
    |      +-- production routes/controllers/services/repos
    |      +-- isolated database and paths
    |      +-- canonical sibling fakes
    |      +-- test HTTP handler seam
    |
    v
forjtest.Harness
    +-- HTTP client / cookie jar
    +-- generated App handle
    +-- typed probes
    +-- resource cleanup stack
```

`forjtest` controls generic lifecycle and HTTP ergonomics. Generated code controls application-specific construction and capability access.

## Public API Shape

### Generated constructor

```go
func TestCreateTicket(t *testing.T) {
	app := testapp.New(t,
		testapp.WithTime(time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)),
	)
	// Cleanup is already registered.
}
```

`testapp.New` calls `t.Helper()`, validates options, registers its cleanup owner, constructs resources, runs migrations, builds the App, and creates the in-process handler. It fails the test immediately on construction error because no useful test can continue with a partial App.

Lower-level usage remains available:

```go
composition, err := testapp.Open(t.Context(), testapp.Config{})
if err != nil {
	t.Fatalf("open test app: %v", err)
}
defer func() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := composition.Close(ctx); err != nil {
		t.Errorf("close test app: %v", err)
	}
}()
```

`Open` is a non-asserting constructor for Testkit's own tests and tooling. It accepts no `testing.TB`, performs no `Fatalf`, `Errorf`, `Setenv`, or implicit cleanup registration, and returns a caller-owned `*Composition`. If `Config.Root` is empty, it creates an owned root with `os.MkdirTemp`; an explicit root must arrive with a matching ownership lease. `New(t, ...)` creates an empty mutable cleanup stack and registers one closure over it with `t.Cleanup` before the construction core acquires anything. Successful ownership transfer changes what that already-registered closure owns; it does not register a later replacement.

The generated package can return a concrete application-specific `*Harness` embedding or wrapping `forjtest.Harness`. The public package never imports generated internal types.

### HTTP client

```go
response := app.HTTP().
	Header("X-Request-ID", "test-create-ticket").
	PostJSON("/api/v1/tickets", CreateTicketRequest{
		Subject: "Unable to sign in",
	})

response.RequireStatus(http.StatusCreated)

var body CreateTicketResponse
response.RequireJSON(&body)
```

Request builders are immutable or single-use. Reusing a consumed builder fails visibly instead of retaining bodies or headers unexpectedly.

Supported v1 helpers:

- `Get`
- `PostJSON`
- `PutJSON`
- `PatchJSON`
- `Delete`
- raw body and content type;
- query parameters;
- headers;
- cookies through the harness jar; and
- multipart only after bounded streaming behavior is designed.

Testkit should not clone all of `httpx`. The application client is intentionally narrow and backed by the in-process `http.Handler`/`httptest` contract. Applications can use `httpx` against an explicitly started `httptest.Server` when they need its complete client surface.

### Response assertions

Suggested methods:

```go
response.RequireStatus(http.StatusCreated)
response.RequireHeader("Content-Type", "application/json")
response.RequireJSON(&body)
response.RequireNoContent()
```

Assertions retain the originating `testing.TB` because the response cannot outlive its harness. They call `Helper()` and stop the current test through `Fatalf` only for requirement-style failures.

The core should also expose non-fatal inspection:

```go
status := response.StatusCode()
body := response.BodyBytes()
err := response.DecodeJSON(&target)
```

Bodies are read once with a hard maximum. Oversized bodies produce a clear truncation marker and do not exhaust memory.

### Application and capability access

Generated harness methods are component-gated:

```go
func (h *Harness) App() *wire.App
func (h *Harness) HTTP() *forjtest.HTTPClient
func (h *Harness) Database() *gorm.DB
func (h *Harness) Events() *eventsfake.Fake
func (h *Harness) Queue() *queue.Queue
func (h *Harness) QueueProbe() *queuefake.Probe
func (h *Harness) Mail() *mailfake.Driver
func (h *Harness) Audit() *auditfake.Fake
func (h *Harness) Seed() *SeedHarness
```

Only selected components generate their typed methods and imports. A capability-neutral method such as `Capabilities()` may report availability for generic helpers, but it cannot manufacture access to an absent capability and there is no `map[string]any` dependency lookup.

### Generic assertions and the Go version

Go versions before generic methods require free functions for typed assertions:

```go
eventsfake.RequirePublished[TicketCreated](t, app.Events())
```

Testkit must not increase GoForj's minimum version merely to turn these into generic methods. The separate Go 1.27 design can reevaluate ergonomics when the repository deliberately changes its baseline.

## Alternate Composition Root

### Why runtime mutation is rejected

The production App stores concrete generated managers and private fields. Constructing it and then replacing dependencies would:

- leave controllers holding the original collaborators;
- bypass provider validation;
- create split graphs where accessors and services see different instances;
- race with startup hooks; and
- require reflection or unsafe access.

Testkit instead generates a second Wire injector whose provider graph is selected before construction.

### Production-provider reuse

The test injector should reuse:

- controllers;
- services;
- repositories;
- route groups;
- middleware;
- serialization;
- validation;
- Auth policy;
- database query behavior;
- application lifecycle hooks that are safe in the selected mode; and
- generated App construction.

It replaces only explicitly classified edges:

- environment/config snapshot;
- database connection/namespace;
- storage roots;
- outbound mail;
- event transport;
- queue dispatch transport;
- Audit store when capture mode is selected;
- external API clients supplied through App-owned providers; and
- clock/entropy contracts that already support injection.

### Required provider refactoring

Before implementation, inventory every generated manager constructor and classify whether its public sibling API can accept the canonical fake. Where a manager stores a concrete implementation, prefer a compatibility-preserving internal refactor to the smallest established public interface.

Do not:

- change application-facing manager methods solely for testing;
- add nil guards around required dependencies;
- create fake-only public production variants;
- expose generated private fields; or
- let the test graph bypass metrics/inspection decorators accidentally.

Instrumentation wrappers should wrap the selected fake exactly as they wrap a real driver when the test enables observation.

### Generated source location

The alternate injector source must live beside the production injector in `internal/wire`, because the existing provider sets are intentionally package-private. Its Wire declaration uses the repository's normal `wireinject` build tag and generates a regular checked-in implementation such as `wire/testkit_gen.go`. It accepts a typed test configuration and returns a `TestComposition` containing the constructed App, HTTP server, fake probes, and one idempotent cleanup callback with explicit ownership-transfer semantics. Returning this bundle avoids adding test-only fields or accessors to the production `App`.

`internal/testapp` is a regular facade package imported only by tests. Go does not automatically define a custom “test” build tag, and a package's `_test.go` files cannot serve as a reusable dependency for arbitrary other packages. The design should not rely on either misconception.

Because the generated injector result and facade are regular packages:

- it may compile during `go test ./...` and `go build ./...` package discovery;
- it is not linked into the production binary unless imported by production code;
- its imports remain visible to `go mod tidy`; and
- generated dependency weight must stay intentional.

Container-heavy adapters therefore belong behind a separate opt-in package/module rather than the default generated package.

## HTTP Handler Seam

The generated HTTP server currently builds the web adapter inside its serve path. Testkit needs a reusable construction method that returns the same adapter as an `http.Handler` without listening.

The authoritative generated source should add a method shaped like:

```go
func (s *Server) Handler() (http.Handler, error)
```

It must use the same:

- route groups;
- framework routes;
- runtime middleware;
- CORS behavior where applicable;
- Auth middleware;
- maintenance behavior;
- metrics hooks;
- Lighthouse inspection hooks; and
- error handler.

Production `Serve` and test `Handler` both call one internal adapter builder so they cannot drift.

The returned handler is wrapped by a harness gate that rejects requests after closing begins and tracks in-flight calls for bounded draining. Requests use `httptest.NewRequest` and a Testkit-owned bounded `http.ResponseWriter`. The writer enforces the capture ceiling while the handler writes; wrapping an unbounded `httptest.ResponseRecorder` and truncating afterward would still permit memory exhaustion. Tests requiring redirect/cookie behavior use the harness cookie jar. Tests requiring streaming, WebSocket, HTTP/2, TLS, or a real client explicitly request `httptest.NewServer` or `httptest.NewTLSServer` and accept a loopback port.

## Lifecycle Model

### Construction sequence

1. Validate all options and capabilities.
2. Create a cleanup stack and register its owner immediately, before any resource provider runs.
3. Allocate an owned temporary root with `os.MkdirTemp` and create resource identities beneath it.
4. Provision explicit external resources.
5. Build immutable configuration.
6. Open the isolated database.
7. Apply generated migrations.
8. Construct canonical fakes and named maps.
9. Pass the typed resource handles into the alternate Wire injector, which constructs the graph and returns an idempotent graph-cleanup callback.
10. Build the HTTP handler.
11. Start only explicitly enabled lifecycle subsets.
12. Transfer graph ownership to the complete harness; `New`'s early cleanup closure observes that transfer, while `Open` returns ownership to its caller.

If any step fails or panics, already-acquired resources are closed in reverse order before the error returns or the panic resumes. Providers push cleanup as soon as acquisition succeeds. Once the App graph takes ownership of a manager, the construction stack removes its separate closer so the same resource cannot be closed twice.

### Default execution mode

Default local mode does not call the production `run` host because that can start HTTP listeners, queue workers, schedulers, Lighthouse connections, or custom long-lived hooks.

The generated graph exposes lifecycle groups:

- core resources;
- in-process HTTP readiness;
- workers;
- scheduler;
- Lighthouse/control plane; and
- App-owned custom hooks.

Testkit starts core resources and the in-process HTTP surface. Other groups require explicit options. Existing unclassified App hooks map to a `custom` group: production continues to start them in its established order, while Testkit excludes them unless `WithLifecycle(Custom)` is requested. This deliberate test-mode difference must be visible in generated documentation.

The current lifecycle implementation must first gain typed grouping and transactional start bookkeeping. Each hook is recorded immediately after it starts successfully. If a later hook returns an error or panics, lifecycle unwinds only the successfully started hooks in reverse order, joins shutdown errors without hiding the start failure, and remains safe for the final cleanup callback. Direct migration tests must prove legacy unclassified production order is unchanged.

### Cleanup

Cleanup first quiesces request producers, then follows dependency ownership rather than closing managers twice:

1. reject new direct-handler requests, close any explicit test server listener, and drain in-flight HTTP requests within the cleanup deadline;
2. stop explicitly started scheduler/workers and other producers;
3. stop the App lifecycle, which closes graph-owned managers and its database pool;
4. close residual handler resources;
5. drop the harness database/schema through a still-live administrative service lease;
6. release that administrative lease, allowing the parent-owned service pool to terminate after its final user;
7. terminate harness-owned containers or external resources; and
8. remove temporary directories.

Wire cleanup callbacks and App lifecycle ownership have one transfer point. Before transfer, the injector callback owns partial construction; after transfer, the harness closes the App once and invokes only residual non-graph cleanup. Tests must fail on duplicate close attempts instead of silently tolerating ambiguous ownership.

The Go testing package cancels its test context before running cleanup callbacks. Testkit therefore creates a short, independent cleanup context rather than passing an already-cancelled context to `Shutdown`.

Cleanup failures use `t.Errorf` so every cleanup still runs and the test reports the resource that leaked.

## Configuration And Environment

### Immutable construction snapshot

Testkit must not race on ambient environment while multiple harnesses construct. Generated test providers accept an immutable typed snapshot containing the relevant resolved settings.

This aligns with the broader GoForj direction of resolving runtime policy near the root and passing it down. It may require internal constructor overloads or generated config providers, but it must preserve production configuration behavior.

Typed configuration is a release gate, not an aspiration. The composition inventory must identify every reused provider that reads ambient environment during or after construction and refactor it to accept the resolved snapshot before parallel-safe Testkit ships. A contract test constructs concurrent harnesses with different snapshots and proves each graph retains only its own values.

### Explicit environment escape hatch

Some application code will still read process environment directly. Testkit may provide:

```go
testapp.WithEnv("FEATURE_MODE", "preview")
```

This delegates to `testing.TB.Setenv`, validates the key, redacts values in diagnostics, and is named/documented as process-global compatibility behavior. `testing.TB` exposes no reliable query for whether a test or ancestor is parallel, so Testkit must not claim it can detect every invalid ordering. Go itself enforces the restriction by panicking when `Setenv` is used in a parallel test or one with a parallel ancestor. An explicit parallel-safe Testkit mode rejects `WithEnv` before calling `Setenv`; otherwise the Go testing contract is authoritative.

The default harness does not call `os.Clearenv`, load a different dotenv file globally, or copy `.env.testing` into the project.

### Secret handling

Configuration diagnostics show key names and source classifications, not secret values. DSNs are parsed and summarized by dialect/host/database without credentials.

## Database Isolation

### Why outer transaction rollback is insufficient

Wrapping a test in one transaction only isolates code that receives that exact transaction handle. Real HTTP handlers may acquire another pooled connection, start nested transactions, or dispatch asynchronous work. Testkit therefore uses resource-level isolation.

### SQLite

Default SQLite behavior:

- create a unique file under the harness-owned temporary root;
- configure the same pragmas and GORM behavior as production;
- apply all generated migrations;
- use the ordinary connection pool; and
- close the pool before temporary directory cleanup.

Connection-local `:memory:` is not the default because multiple pooled connections can observe different databases unless configured with special shared-cache semantics.

### PostgreSQL and MySQL

Service mode chooses one explicit strategy:

- unique database per harness; or
- unique schema per harness only when all generated SQL, migrations, and DSN behavior are proven schema-safe.

Names use a validated random suffix and never interpolate untrusted test names into SQL. Administrative create/drop statements quote identifiers through dialect-specific helpers and validate the resolved target before deletion.

A container may be shared only through an explicit parent- or `TestMain`-owned service pool passed to each harness as an option. Testkit has no mutable package-global pool. Each harness retains a unique logical database and ownership lease; the parent closes the pool only after child harness leases have been released. Shared service lifecycle and per-test database ownership are separate.

### Migrations

Tests run the same generated migrations as production. They do not call GORM `AutoMigrate`, skip failed migrations, or construct a reduced schema from model reflection.

Migration completion is a prerequisite for Seed and HTTP use. Failure output identifies the migration and dialect without printing credentials.

### Database access

`app.Database()` returns the harness's default `*gorm.DB` for deliberate domain assertions. Named connections receive generated accessors or a name-based method with validated generated names.

Direct database assertions are allowed, but documentation should prefer observable application behavior when practical.

## External Resource Provisioning

### Default posture

`testapp.New(t)` never contacts Docker, downloads images, binds fixed ports, or starts services based only on a Compose file.

Tests request service mode explicitly:

```go
app := testapp.New(t,
	testapp.WithDatabase(testapp.PostgreSQL()),
	testapp.WithRedis(),
)
```

### Resource plan

Generated test code derives supported resource kinds, image pins, readiness, and environment mapping from the same project resource catalog used by render and integration tests. It must not maintain a second hardcoded database/Redis matrix.

### Testcontainers adapter

Testcontainers for Go is appropriate for explicit integration mode because it programmatically creates and cleans container dependencies and supports test cleanup helpers:

- [Testcontainers for Go](https://golang.testcontainers.org/)
- [Creating and cleaning up containers](https://golang.testcontainers.org/features/creating_container/)

The adapter should:

- use repository-pinned images;
- use random mapped ports;
- register cleanup as soon as a resource exists;
- attach concise test logging only after failure or verbose selection;
- return structured endpoints rather than mutate global env;
- support CI image substitution; and
- never disable cleanup/reaper behavior silently.

The adapter lives at `forjtest/containers` in GoForj's existing root module. That module already owns the Testcontainers for Go dependency, so v1 does not add a nested module or a second release surface. The core `forjtest` package does not import the adapter.

Network-requiring tests remain explicit integration tests and may skip under `testing.Short()` or a project-defined integration gate.

## Primitive Modes And Probes

### Events

Default: the canonical `eventsfake` implementation wrapping the synchronous in-memory bus, recording publications while production subscribers run.

These are distinct modes:

- `CaptureEvents`: records publications without invoking application subscribers.
- `RunEventSubscribers`: uses the synchronous bus and records publications while invoking registered subscribers.

`RunEventSubscribers` is the v1 default because subscribers are part of application behavior, while external distributed transport remains replaced. `CaptureEvents` is explicit and the alternate injector excludes subscriber registration in that mode rather than pretending handlers ran.

The capture wrapper must preserve handler errors and publication semantics rather than returning success unconditionally.

The events repository currently exposes overlapping fake entry points. Before Testkit pins one, that repository must designate `eventsfake` as canonical, keep the other entry point as a compatibility wrapper or deprecate it on the repository's normal schedule, and run direct parity tests. Testkit imports only the canonical package.

### Queue

Default: a real `*queue.Queue` configured with a queue-owned recording backend, with captured dispatches exposed through a separate `queuefake.Probe`.

Assertions come from the queue-owned probe:

```go
app.QueueProbe().RequireDispatched(t, "ticket:index")
```

The current `queue.NewFake` has different dispatch signatures and is not substitutable for the concrete manager used by generated applications. Testkit must not inject it and claim API parity. As a prerequisite, the queue repository should add the narrow recording backend/driver seam needed to construct the ordinary `*queue.Queue`, preserve named managers, workflow dispatch, handler registration, validation, and instrumentation, and expose immutable recordings through the probe. Existing `queue.NewFake` remains available for its established unit-test use. Compatibility tests must execute the same public dispatch calls against real and recording backends.

An explicit real-worker mode uses a real local or service-backed queue, registered production handlers, bounded workers, and deterministic shutdown. Testkit does not add `RunNext` to the recording fake merely for convenience.

### Mail

Default: canonical `mailfake.Driver`.

Testkit exposes the driver and may provide only generated domain helpers for common Auth mail lookup. Subject/body/recipient assertions remain owned by `mailfake`.

Failure diagnostics never dump full HTML, tokens, or magic links automatically. Tests inspect those fields explicitly when that is their purpose.

### Cache

Default: isolated memory or canonical cache fake according to the behavior being tested.

A pure recorder that does not implement real get/set semantics cannot substitute where application behavior depends on cache hits. The inventory phase must select the canonical functional fake/local driver and expose operation records separately.

Named caches receive distinct instances and prefixes.

### Storage

Default: a unique local directory under the harness-owned temporary root or a functional memory driver if it has identical semantics for the tested capability.

Tests must be able to inspect stored paths and bytes through the normal `storage` API. The harness validates resolved paths stay within its owned root before cleanup.

Named disks receive distinct roots unless the application explicitly configures a shared disk.

### Audit

Default when selected: `auditfake` capture for service behavior, with an option to use the isolated SQL store for query and transaction tests.

Capture mode proves append intent. SQL mode proves durable ordering, query, and transaction behavior. The harness reports which mode is active so a fake assertion cannot be mistaken for persistence evidence.

### HTTP clients and external APIs

Application-owned outbound client providers should accept a test `http.RoundTripper`, base URL, or interface through normal App injection. Testkit can create `httptest.Server` resources, but it does not intercept all network traffic globally.

An optional deny-network transport can fail unexpected HTTP requests made through injected `http.Client` instances. It cannot claim to sandbox arbitrary sockets opened by third-party code.

## Authentication

### Real-flow helper

The highest-confidence path exercises public routes:

```go
app.Auth().Register("agent@example.test", "correct horse battery staple")
app.Auth().Login("agent@example.test", "correct horse battery staple")
```

These helpers issue real HTTP requests, retain cookies in the harness jar, and fail with bounded diagnostics.

### Test session helper

Many tests need an authenticated actor without retesting registration and password hashing. Generated `internal/testapp` may expose:

```go
app.Auth().AsUser(userRef)
```

It calls a generated test-only credential issuer directly, then passes the issued credential through the normal post-credential eligibility, tenant-membership, account-state, and session-creation policy. User references are opaque and bound to the current harness/database and tenant; a reference from another harness is rejected. The helper writes a real server-side session using the isolated database and places correctly signed cookies in the harness jar. It does not:

- register an HTTP endpoint;
- weaken production middleware;
- accept arbitrary identity headers;
- skip persisted session state; or
- expose signing secrets in failure output.

An internal package path is not by itself a production boundary because other packages in the same generated module can import it. Generation must therefore fail if a non-test application package imports `internal/testapp`, production-binary tests must prove no test-session issuer symbols or providers are linked, and issued credentials must carry test-issuer provenance rejected by the production composition. The helper is never wired into the production route graph.

### User creation

Testkit should not invent generic Auth table rows. A user comes from:

- an Auth-owned test factory that preserves password/session invariants;
- a Seed scenario; or
- an application-owned factory.

### Authorization

Authentication helpers do not grant domain roles automatically. Tests create roles and tenant memberships through application-owned services or Seed scenarios.

## Seed Integration

When the proposed Seed component is selected, generated Testkit exposes the App's actual scenario registry.

```go
report := app.Seed().Run("ticket-overdue",
	seed.WithTime(fixedTime),
	seed.WithEntropy(42),
)
```

Testkit supplies:

- the harness context;
- fixed logical time when configured;
- isolated resources;
- local/test environment classification;
- structured failure presentation; and
- cleanup through the harness.

Seed remains responsible for scenario validation, deterministic steps, parameters, and results. Testkit does not parse fixture YAML, reset arbitrary tables, or infer cleanup from scenario output.

Because every harness owns fresh persistence, most application tests do not need scenario-specific teardown.

## Named Apps

Each generated App gets its own `internal/testapp` constructor or a generated selector whose return type preserves the chosen App's concrete capabilities.

Preferred shape:

```go
admin := admintest.New(t)
api := apitest.New(t)
```

This is clearer than one dynamic `New(t, "admin")` returning a lowest-common-denominator harness.

When multiple Apps must share one database or event bus in a test, the application explicitly creates a shared resource lease and passes it to both constructors. Ownership identifies which harness closes the resource. Double cleanup is rejected.

## Time And Entropy

### Clock contract

`forjtest.Clock` provides deterministic `Now` and timer behavior only to collaborators designed to accept it. It does not patch the Go runtime.

A controllable clock should define:

- current time;
- advancing time;
- timer/ticker delivery ordering;
- cancellation;
- concurrent safety; and
- cleanup of blocked waiters.

If only `Now()` is required in v1, call the interface `Clock` and defer fake timer semantics rather than shipping an incomplete scheduler simulation.

### Seed time

Seed receives the harness time explicitly under its own design. Adding random calls in one Seed step must not perturb unrelated steps.

### Cryptographic entropy

Auth tokens, session secrets, and cryptographic operations should use real secure entropy by default. Deterministic entropy is allowed only through an explicit test provider whose outputs never leave isolated test state.

Failure output never prints those values.

## Parallelism

A harness is parallel-safe only when:

- it uses no `WithEnv` option;
- database names/schemas are unique;
- temporary paths are unique;
- ports are randomly allocated;
- fakes are instance-scoped;
- package-global library state is not mutated;
- shared containers allocate separate logical state; and
- application code under test does not use uncontrolled globals.

Testkit validates the conditions it owns. It cannot prove application code has no globals.

Parallel-safe mode rejects `WithEnv` regardless of option order before applying it. Outside that mode, Testkit cannot pre-detect all `testing` parallel states and relies on Go's `Setenv` enforcement. Documentation shows `t.Parallel()` only in examples that avoid process environment mutation.

Shared service pools use leases and remain safe when one subtest completes before another. A parent test may own a shared pool and close it after all parallel subtests return, following Go's subtest lifecycle semantics.

## Failure Diagnostics

HTTP failures show:

```text
POST /api/v1/tickets
status: got 422, want 201
content-type: application/json
body:
  {"error":"subject is required"}
```

Diagnostics are bounded and redact:

- `Authorization`;
- `Cookie` and `Set-Cookie`;
- CSRF tokens;
- known secret-shaped JSON fields;
- DSN credentials;
- mail tokens and links;
- Audit changes/attributes; and
- configured application-specific sensitive fields.

Displayed request and redirect URLs use the route template when known. Otherwise diagnostics strip user information, redact all query values while retaining safe key names, sanitize token-shaped path segments, and bound redirect targets and history. Raw URLs are never included through wrapped client errors.

Binary and invalid UTF-8 bodies are summarized by content type, byte count, and safe prefix encoding. Redirect histories are bounded. JSON formatting errors retain the raw bounded safe body after redaction.

Fake assertion failures should defer to canonical sibling formatting. Testkit does not dump every probe when one HTTP assertion fails because that creates noise and leakage.

## Observability

### Logging

Default local harness uses a test logger that writes concise output through `testing.TB.Logf` only when:

- the application emits warning/error output;
- the test fails and buffered debug output is flushed; or
- verbose mode is selected.

It preserves structured fields for inspection while redacting secrets.

### Metrics

Metrics may run in-memory when selected. Testkit should expose a gatherer for direct assertions rather than starting a metrics listener.

Assertions should prefer stable metric families and labels, not serialized exposition text, unless formatting itself is under test.

### Lighthouse

Default application tests do not connect to an external Lighthouse process. Local inspect capture may remain enabled in-memory when the test asks for execution timelines.

Testkit can expose recent inspect summaries after failure, bounded and redacted. It must not make test behavior depend on an available Lighthouse server.

## Generated Configuration

Project configuration should distinguish testkit generation from runtime components:

```yaml
testing:
  application: true
  defaults:
    events: subscribers
    queue: capture
    mail: capture
    storage: temporary
```

The exact schema should be added only after current project configuration ownership and reconciliation are inventoried.

Rules:

- adding Testkit generates framework-owned files and dependencies;
- removing it deletes only framework-owned test composition files;
- application-owned tests and factories are never deleted;
- rerender preserves App-owned test options and providers through documented extension files; and
- runtime `.env` and `.env.example` are not modified merely to enable Testkit.

`forj make:test <name>` may generate an App-owned test file that imports the correct test App package. It is a convenience after the core harness works, not a prerequisite.

## Security Model

1. Test-only Auth behavior creates no runtime route or header bypass.
2. Test credentials and secrets remain inside isolated state and redacted diagnostics.
3. Resource cleanup requires an unforgeable harness-created ownership lease for the exact database, schema, path, container, or server identity before deletion; endpoint allowlisting alone never authorizes destructive migration, Seed reset, or cleanup.
4. Temporary storage never uses the repository root, home directory, `/`, or unresolved environment variables.
5. External service mode is explicit and uses bounded, pinned resource definitions.
6. Unexpected external HTTP calls can be denied only through injected transports; Testkit does not overstate network sandboxing.
7. HTTP response bodies and logs have hard capture limits.
8. Symlinks inside owned storage roots are not followed during cleanup without validated containment.
9. Database administrative statements use validated generated identifiers and least-privilege credentials where practical.
10. Failure output never prints cookies, tokens, DSNs, private keys, full mail bodies, or Audit values by default.
11. Seed production/destructive gates cannot be disabled by Testkit outside the isolated test environment classification.
12. A test cannot attach to a production database merely because a process environment variable is present; non-destructive external access requires explicit configuration, while migrations, Seed resets, and drops additionally require a Testkit-created resource ownership lease.

## Performance Model

The minimum useful local harness should be cheap enough for ordinary package tests:

- no container startup;
- no real TCP listener;
- one temporary SQLite database when selected;
- in-process fakes;
- migrations cached only when safe and copied into unique per-test state; and
- lazy optional capabilities.

Measure separately:

- composition time;
- migration time;
- first request and steady request latency;
- cleanup time;
- allocations per request helper; and
- service-mode provisioning time.

Potential optimizations:

- prepare a migrated SQLite template file keyed by exact migration fingerprint, then copy it per harness;
- share container processes while allocating unique databases/schemas;
- cache immutable route metadata; and
- lazily create probes not selected by the App.

Caching must use complete fingerprints and atomic publication. A stale migrated template is worse than a slower test and must fail closed or rebuild.

The design should establish an informal target, not a hard noisy CI gate: a minimal SQLite harness should normally construct in well under a second on a warm developer machine, while service mode reports its provisioning cost separately.

## Error Model

Lower-level constructors return typed errors for:

- unsupported capability;
- invalid option combination;
- non-parallel-safe configuration;
- resource unavailable;
- migration failure;
- composition failure;
- handler construction failure;
- startup failure;
- cleanup failure; and
- unsafe cleanup target.

Errors support `errors.Is`/`errors.As`, retain safe causes, and identify the failing phase/resource without credentials.

`testapp.New(t)` converts returned construction errors into one concise fatal test message. Provider and lifecycle panics are cleaned up and resumed, and invalid `Setenv`/parallel ordering retains the Go testing package's documented panic.

Cleanup errors are joined and reported non-fatally so later cleanups execute.

## Compatibility

### Source and API compatibility

Testkit is additive. Existing generated Apps and tests continue to compile when the capability is not enabled.

Internal manager refactors needed for fake injection must preserve existing public App and manager APIs.

### Configuration compatibility

Testkit adds project testing configuration, not runtime environment configuration. Existing `.goforj.yml` files default to disabled until the team decides generation should become universal.

### Persisted data

Test databases are disposable. Production migration files remain authoritative and are not rewritten for Testkit.

### Runtime behavior

Production binaries do not import `internal/testapp`, start test resources, or register test routes. Adding Testkit must produce no runtime behavior change.

### Operational behavior

Service-mode tests may require Docker or an explicit external endpoint and can consume images, ports, CPU, and disk. They remain opt-in and clearly reported.

### Minimum Go version

The initial implementation should match GoForj's existing minimum version. It may use `testing.T.Context` only if that method is present at the supported baseline. Generic methods are not required.

## Testing The Testkit

### Public package tests

Directly cover:

- cleanup registration and reverse order;
- partial-construction cleanup;
- cleanup after provider and lifecycle panics, with the original panic resumed;
- cleanup racing an in-flight direct request or test-server request, including drain timeout behavior;
- cancelled test versus independent cleanup context;
- option validation;
- request builder reuse;
- body limits;
- response capture that remains bounded while a handler writes an oversized body;
- JSON decode errors;
- cookie persistence;
- redirects;
- URL, redirect, credential, and body redaction;
- concurrent clients; and
- safe error formatting.

### Generated composition tests

Render disposable Apps under `/tmp` covering:

- minimal CLI App;
- Web API without database;
- SQLite API;
- MySQL API;
- PostgreSQL API;
- Auth;
- all fake-capable primitives;
- Audit;
- Seed;
- default and named Apps; and
- largest supported composition.

Verify component-off code contains no unsupported imports or accessors.

### Behavioral contract tests

One representative generated application proves:

- real middleware runs;
- route parameters and JSON binding work;
- Auth login and test session helpers both produce valid server-authoritative sessions;
- test-session helpers enforce normal eligibility and tenant policy, reject cross-harness references, and are absent from production artifacts;
- database state is isolated between harnesses;
- parallel local harnesses do not collide;
- queue capture does not run handlers;
- event subscriber and capture-only modes differ honestly;
- mail capture retains messages;
- temporary storage is contained;
- Seed creates deterministic state;
- Audit capture and SQL modes differ honestly;
- lifecycle defaults start no workers/scheduler; and
- cleanup closes all goroutines, pools, files, servers, and resource leases.

The generated suite also constructs two harnesses concurrently with different typed configuration snapshots and proves the values cannot cross. It covers Go's `Setenv`/`Parallel` restriction in both call orders and through a parallel ancestor, using subprocesses so the expected testing panic cannot corrupt the parent suite.

### Service-mode tests

With explicit integration gates, prove:

- random port allocation;
- pinned images;
- readiness;
- cleanup after construction failure;
- unique MySQL/PostgreSQL state per harness;
- Redis namespace isolation;
- concurrent harnesses;
- process cancellation; and
- no local replacement hides a required published module.

### Race and leak testing

Run focused race tests for:

- probe capture;
- HTTP cookie jars;
- shared service leases;
- cleanup registration;
- concurrent requests; and
- controllable clocks.

Use bounded goroutine/resource leak assertions around representative harness construction and closure. Account explicitly for Testcontainers/reaper goroutines in service mode rather than globally ignoring leaks.

## Documentation

Generated application documentation should explain:

- when to use unit, handler, application, and infrastructure tests;
- how to construct a Test App;
- default fake versus real behavior for every selected primitive;
- how database isolation works;
- how to authenticate through real login and the test session helper;
- how to run Seed scenarios;
- how to make typed sibling assertions;
- how to request real services;
- why `WithEnv` and parallel tests are incompatible;
- how cleanup works; and
- what passing the test does not prove.

Examples must keep expected output adjacent to the call that produces it. Failure examples use one `//` comment per output line and redact invisible or sensitive values unless those bytes are the subject of the example.

## Delivery Plan

### Phase 0: composition inventory

1. Inventory every generated App/provider edge and canonical sibling fake.
2. Classify concrete manager coupling that prevents safe injection.
3. Inventory lifecycle registrations by core, HTTP, workers, scheduler, Lighthouse, and custom hooks.
4. Define the minimum immutable configuration snapshot.
5. Prove one hand-written alternate injector in a disposable rendered App before changing templates.

### Phase 1: public foundation

1. Add `forjtest` lifecycle and cleanup stack.
2. Add in-process HTTP client and response assertions.
3. Add redaction and body limits.
4. Add resource/capability vocabulary.
5. Add complete package documentation and examples.

### Phase 2: generated local harness

1. Add testing project configuration.
2. Add generated `internal/testapp` and alternate Wire injector.
3. Add the shared HTTP handler builder used by production and Testkit.
4. Add temporary SQLite and storage isolation.
5. Add events, queue, and mail fakes.
6. Add Auth real-login and test-session helpers.
7. Prove lifecycle and cleanup behavior.

### Phase 3: Seed, Audit, and named resources

1. Integrate the released Seed scenario API.
2. Integrate Audit capture and SQL modes.
3. Add cache and storage probes based on canonical sibling behavior.
4. Add named primitive maps and named App constructors.
5. Add optional in-memory inspect and metrics access.

### Phase 4: service mode

1. Add the opt-in `forjtest/containers` adapter in the existing root module.
2. Derive resources from the existing catalog and pinned versions.
3. Support PostgreSQL, MySQL, and Redis explicitly.
4. Add shared-service/per-harness-database leases.
5. Add real-worker mode without changing queue fake semantics.

### Phase 5: developer workflow

1. Add `forj make:test` only if examples show repetitive file setup.
2. Add concise testkit documentation to generated project README/AGENTS guidance.
3. Integrate application tests with `forj test` command selection without replacing `go test`.
4. Add Atlas scenarios that require observable behavior and canonical fake assertions.
5. Run published-module validation with `GOWORK=off` for every changed sibling surface.

## Acceptance Criteria

The design is implemented only when:

- a generated test constructs the real App graph through an alternate compile-time injector;
- no dependency is replaced through reflection, unsafe access, package-global mutation, or post-construction field edits;
- an in-process request traverses the exact production route/middleware builder;
- default construction binds no real port and starts no worker or scheduler;
- SQLite state is unique per harness and uses production migrations;
- PostgreSQL/MySQL requests either use the requested isolated dialect or fail without fallback;
- canonical mail, event, queue, cache, storage, Seed, and Audit packages retain ownership of their semantics;
- fake handles are distinct per named instance and per test;
- Auth helpers create real persisted session state without runtime backdoors;
- Seed integration uses scenario time, entropy, parameters, and safety gates honestly;
- queue capture never reports handler completion;
- all acquired resources clean up after success, fatal assertion, construction error, cancellation, and panic unwinding;
- cleanup cannot delete an unresolved, broad, merely allowlisted, or non-owned path/database target;
- HTTP and probe diagnostics are bounded, readable, and redacted;
- default harness construction does not mutate ambient environment;
- explicit environment mutation is rejected from parallel-safe mode and otherwise follows Go's enforced `Setenv`/parallel restriction;
- production App source/API, runtime behavior, and dependency selection remain compatible;
- component-off and minimal generated Apps compile without irrelevant test imports or typed accessors, and absent-capability options fail before acquisition;
- default, named, and largest App compositions pass render tests under `/tmp`;
- service-mode tests use catalog-derived pinned resources and random ports; and
- generated documentation clearly distinguishes application-test evidence from provider, browser, and end-to-end evidence.

## Risks And Mitigations

### Risk: The alternate graph drifts from production

Mitigation: share provider sets and one HTTP adapter builder; maintain an explicit short list of test replacements and parity tests.

### Risk: Fakes do not satisfy concrete generated managers

Mitigation: inventory first and make narrow compatibility-preserving interface refactors in owning repositories rather than adding generated fake clones.

### Risk: Testkit becomes a service locator

Mitigation: generate typed capability methods and explicit option types; reject arbitrary `map[type]any` overrides.

### Risk: Tests pass against behavior unlike production

Mitigation: document fake/real modes, reuse production middleware and migrations, and require driver/browser tests for claims beyond application behavior.

### Risk: Global env prevents parallel tests

Mitigation: pass immutable construction snapshots and isolate explicit `Setenv` usage as non-parallel-safe.

### Risk: Cleanup damages unrelated state

Mitigation: resource leases, validated exact targets, owned temp roots, random generated database names, and cleanup safety tests.

### Risk: Background work makes tests nondeterministic

Mitigation: capture by default; start workers/scheduler only through explicit bounded modes.

### Risk: Auth convenience creates a production bypass

Mitigation: no test routes or trusted headers; generate a direct internal session issuer used only by the alternate test composition.

### Risk: Container startup makes ordinary tests slow

Mitigation: no implicit containers, an opt-in adapter package, explicit parent-owned shared process leases, and local-mode performance measurement.

### Risk: Test failures leak secrets

Mitigation: centralized bounded redaction, explicit safe diagnostics, and adversarial tests covering headers, cookies, JSON, DSNs, mail, Audit, and logs.

## Deferred Questions

These require implementation evidence after the minimum local harness:

1. Whether testing configuration should eventually be generated for every project by default.
2. Whether a migrated SQLite template cache provides meaningful speedup without excessive invalidation complexity.
3. Which sibling fakes need public interface adjustments after the composition inventory.
4. Whether later demand justifies moving `forjtest/containers` into a separately released module; v1 keeps it in the root module.
5. Whether WebSocket and SSE helpers belong in Testkit or remain direct `web` tests.
6. Whether a browser driver should consume Testkit resource leases without becoming part of the core.
7. Whether application-defined fake providers need a typed extension manifest.
8. Whether full subprocess mode is useful for code that cannot avoid process-global environment.

The v1 composition, isolation, HTTP, fake ownership, Auth, lifecycle, and cleanup contracts should not wait on these questions.
