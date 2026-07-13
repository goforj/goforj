# Forj API Index Design

## Purpose

The Forj API Index turns the HTTP surface selected by one GoForj App into three
build artifacts:

- a canonical, source-oriented API manifest
- a deterministic diagnostics report
- a clean OpenAPI 3.0.3 document

The index is derived from Go source. It does not start the application, execute
route providers, or inspect a running router. Static analysis keeps the build
repeatable and makes indexing available before a deployable binary exists, but
it also means uncertainty must remain visible rather than being filled with
plausible-looking contracts.

The main design rule is:

> Publish only what the selected App and the source can justify. Preserve
> uncertainty as diagnostics and unconstrained schemas instead of inventing a
> route, status, field, or security policy.

## Ownership Boundary

The feature is split between `github.com/goforj/web/webindex`, GoForj's
`internal/apiindex` package, and build lifecycle coordination.

| Owner | Responsibilities |
| --- | --- |
| `web/webindex` | Parse source, resolve the selected route composition, analyze handlers, load focused Go type information, build Manifest v2, report diagnostics, project OpenAPI, and publish caller-selected JSON paths safely. |
| `goforj/internal/apiindex` | Decide whether the active App participates, choose composition and artifact paths, supply project/App OpenAPI metadata and generated auth policy, expose the standalone command, stage and atomically publish candidates, remove stale CLI-only artifacts, and report App-scoped status. |
| `goforj/internal/build` | Resolve the active App for late-bound CLI dispatch, pass mirrored source options to indexing, and publish or discard candidates at the final compile/start boundary. |
| Generated GoForj HTTP code | Serve the active App's OpenAPI document and its version-pinned Scalar reference page. |

During coordinated development, GoForj uses:

```go
replace github.com/goforj/web => ../web
```

That replacement lets GoForj exercise the local `webindex` changes without
publishing an intermediate `web` module release. It is a repository-development
link, not part of a generated application's API-index contract.

## Commands and App Selection

API indexing is a build concern. The supported command surfaces are:

```text
forj build:api-index [--strict] [--tags dev,integration]
forj <app> build:api-index [--strict] [--tags dev,integration]

forj build [--api-index-strict] [-tags dev,integration]
forj <app> build [--api-index-strict] [-tags dev,integration]

forj run [--api-index-strict]
forj <app> run [--api-index-strict]
```

`build:api-index` runs only indexing and publishes immediately. `build` and
`run` use the same indexer but delay publication until their final compile or
process-start boundary succeeds.

An explicit standalone `--tags` selection and a `go build` passthrough
`-tags`/`--tags` selection apply to syntax discovery, generated Auth policy
inspection, and focused `go/packages` type loading. An invocation tag value
overrides a tag value from `GOFLAGS`, matching normal Go command precedence;
callers that need both sets pass their union explicitly. `-overlay` and
`-modfile`, `-race`, `-msan`, `-asan`, and alternate `-compiler` selections are
rejected whether they arrive as passthrough arguments or through `GOFLAGS`.
The indexer cannot currently mirror alternate source/module inputs or the
implicit tags and compiler constraints created by those toolchain modes, and
must not publish a contract for a different build surface.

The unqualified form selects the default App, `app`. A prefixed command selects
that named App through the normal GoForj command dispatcher; API indexing does
not add a separate `--app` selector.

For modern project configuration:

- an App with WebAPI enabled must have its route composition file; its absence
  is an actionable error
- an App known not to have WebAPI is skipped and any stale API artifacts are
  removed
- a legacy project whose component intent is unknown may be skipped when no
  route composition exists

The standalone command prints the App, lifecycle outcome, and operation,
schema, and diagnostic counts. Pipeline timings include the same compact status
when timings are enabled. Outcomes distinguish changed, unchanged, cleaned,
skipped, and rejected work.

## App-Scoped Inputs and Artifacts

The route-composition source and artifact paths are derived from the active App.

| App | Route composition | Manifest | Diagnostics | OpenAPI |
| --- | --- | --- | --- | --- |
| default `app` | `app/routes.go` | `build/api_index.json` | `build/api_index.diagnostics.json` | `build/openapi.json` |
| named `<app>` | `app/<app>/routes.go` | `build/<app>/api_index.json` | `build/<app>/api_index.diagnostics.json` | `build/<app>/openapi.json` |

Only route groups returned by the active composition's `ProvideRoutes` function
define the indexed HTTP surface. A `.Routes()` call that is created but not
returned does not make its provider part of the App.

This is both a deployment boundary and a correctness boundary: a `billing`
artifact must never contain routes exposed only by `reporting`.

## Route Discovery and Composition

`webindex` parses non-test Go files while excluding common generated, vendor,
cache, temporary, template, and nested-module trees. File selection honors the
active Go build context, including `GOOS`, `GOARCH`, filename constraints,
`//go:build`, invocation build tags, and tags supplied through `GOFLAGS` when
the invocation does not override them. The same tags are passed to focused
type loading so syntax and schemas cannot select different files. GoForj
additionally excludes `_data` and `bin`.

The native route vocabulary is:

- `web.NewRoute(method, path, handler, ...middleware)`
- `web.NewRouteGroup(prefix, routes, ...middleware)`
- the historical `http.NewRoute` and `http.NewRouteGroup` spelling used by
  older generated source

WebSocket routes are recognized for route discovery but excluded from the
OpenAPI operation set.

App scoping identifies a route provider by all available semantic parts:

```text
package + import path + receiver + provider method
```

The provider method is significant. `PublicRoutes`, `ProtectedRoutes`, and
`Routes` on the same controller can have different prefixes and middleware and
must not collapse to one receiver-level owner. Full import paths prevent two
packages with the same short name from exchanging routes or handler metadata.

The composition tracer supports the static forms generated and commonly
written by GoForj Apps, including:

- direct returned `[]web.RouteGroup` composite literals
- route and group variables declared with `var`, `:=`, or assignment
- `append` and `slices.Concat` route composition
- parenthesized expressions used by those forms
- the historical `ProvideAppRoutes`/`AppRoutes` registry shape, including its
  generated positive-length guards around public and protected group appends
- empty root prefixes as well as literal non-empty prefixes

Only values that flow into returned groups participate. Outside the bounded
historical positive-length guard above, dynamic prefixes, branch- or
loop-dependent composition, unsupported route expressions, and unsupported
return expressions produce diagnostics. An explicitly returned empty slice is
a valid zero-endpoint App. The indexer does not silently widen scope to every
route provider in the repository when composition cannot be proven.

Prefixes and route paths are concatenated exactly as the runtime router does;
the indexer does not repair missing or duplicate slashes. Group middleware is
followed by route middleware in runtime order. Exact source expressions,
parameterized calls, and duplicates are retained because repeated middleware
can be observable. Statically enumerable variadic middleware actuals passed to
a provider method are substituted as their exact source expressions. An
expansion whose elements cannot be enumerated is diagnosed rather than replaced
by a guessed list.

Handler resolution uses the function name together with import-path, package,
and receiver hints from the route expression. Missing or ambiguous symbols are
reported and selected deterministically; strict mode can reject the result.

## Handler Inference

Inference is limited to parameters declared with a supported HTTP context type.
Methods with similar names on unrelated services or payloads are ignored.

The primary surface is `web.Context`:

| Evidence | Manifest result |
| --- | --- |
| `ctx.Param("id")` | required path parameter |
| `ctx.Query("q")` | optional query parameter |
| `ctx.Header("X-Request-ID")` | optional header parameter |
| `ctx.Cookie("session")` | optional cookie parameter |
| `ctx.Bind(&payload)` | request body and focused typed schema |
| `ctx.JSON(status, value)` | JSON response status and schema |
| `ctx.Text`, `ctx.HTML`, `ctx.Blob` | media-specific response |
| `ctx.NoContent` | status-only response |
| `ctx.Redirect` | redirect response |
| `ctx.File` | binary file response |

Path-template parameters are also derived from literal route paths, so a
`:id` segment remains present even when the handler never reads it explicitly.
A handler read whose name is absent from the route template is diagnosed and
does not create a fictitious path parameter.

Echo inference is deliberate compatibility, not the native contract. Existing
handlers declared with `echo.Context` retain support for `Param`, `QueryParam`,
`QueryParams().Get`, `Request().Header.Get`, `Cookie`, `Bind`, `JSON`, `String`,
`HTML`, `Blob`, `XML`, `NoContent`, `Redirect`, and `File`. Native and
compatibility provenance remains distinguishable as `web.*` and `echo.*`.

Status and parameter-key inference accepts integer or string constant
expressions, imported `net/http` constants, and lexically visible local aliases
and arithmetic. Constants shadowed in a nested scope cannot leak into a call
outside that scope. A runtime status expression is retained as an unresolved
response, reported as a warning, and projected as an OpenAPI `default`
response. It is not changed to `200`.

Only calls within the selected handler declaration contribute request and
response evidence; an unrelated function in the same loaded package cannot
change the selected operation. Literal JSON maps, structs, and heterogeneous
arrays retain fields or alternatives evident from syntax. Typed evidence
supersedes heuristic shapes except when a map literal exposes fixed keys more
precisely than its general map type. A helper call or runtime value whose
payload cannot be established becomes `{}`. A handler with no recognized
response receives `handler_no_response`; OpenAPI emits only an unresolved
`default` response.

## Canonical Manifest v2

`api_index.json` is the source-oriented contract. Its version is currently
`"2"`.

Top-level fields are:

- `version`
- `operations[]`
- `schemas[]`
- `diagnostics[]`

Each operation contains:

- internal `id`, currently `METHOD:/router/path`
- `method` and router-native `path`
- `handler`: source expression, package, import path, receiver, function,
  project-relative file, and line
- optional human-authored `metadata`: summary, description, tags, and explicit
  security policy
- collected `middleware[]`
- `inputs`: path, query, header, and cookie parameters plus an optional body
- `outputs.responses[]`: status, type display name, schema, media type, source,
  and confidence

Parameters contain `name`, `in`, `required`, and `confidence`. Request bodies
contain `type_name`, `schema`, `source`, and `confidence`.

Each named schema contains:

- `identity`: canonical Go identity using full import paths
- `name`: readable, codegen-safe OpenAPI component name
- `package` and display `type_name`
- `definition`: the schema graph
- `confidence`

Each diagnostic contains severity, stable code, message, project-relative
source location, and operation ID when applicable. The standalone diagnostics
file is the same sorted diagnostics array embedded in the manifest.

The manifest operation ID is intentionally diagnostic and route-oriented. The
OpenAPI `operationId` is a separate semantic, codegen-oriented identifier.

## Focused Typed Schemas

Route discovery remains a fast syntax pass. Semantic schemas use
`go/packages`/`go/types` only for packages containing handlers selected by the
active route surface. Packages are requested by exact `file=` patterns rather
than by scanning and type-checking the repository as a whole.

Type loading is skipped entirely when the selected handlers do not call
`Bind` or return `JSON`. Checked expressions are mapped back to the syntax pass
by canonical filename and byte offsets, allowing route discovery to stay small
while body and response expressions receive exact types.

The mapper supports:

- named and recursive contracts
- concrete generic instantiations
- aliases and typed constant enums
- pointers and nullability
- slices, fixed arrays, and maps
- JSON-representable primitive types
- exported struct fields and `encoding/json` names, ignored fields, embedding,
  dominance rules, and eligible `,string` wire encoding
- `[]byte` as a base64 string
- unsigned integer minima without dishonest signed formats
- `time.Time`, `encoding/json.Number`, `encoding/json.RawMessage`, and
  explicitly known UUID library types
- safe `validate`/`binding` hints for `required`, UUID, email, and `oneof`

Nil-capable slices, maps, and byte slices are nullable; fixed arrays are not.
Typed map literals with statically known string keys become exact object
contracts with those properties required and `additionalProperties: false`.
Heterogeneous literal arrays use `anyOf` for their item alternatives. Unsafe
map keys stay unconstrained. Named collection types are checked for custom
codec method sets before their underlying slice, array, map, or byte form is
projected.

Requiredness is conservative. A property is required only when a supported
validation or binding tag explicitly contains `required`; Go value type,
pointer shape, and the absence of `omitempty` do not imply API policy.
Validation options are interpreted only in the field-level segment before
`dive`, `omitempty`, or `omitnil`; element validation cannot accidentally make
the container property required. Quoted `oneof` values remain single enum
members.

Unsupported interfaces, channels, functions, unions, uninstantiated type
parameters, unsafe map keys, ambiguous embedded JSON fields, incompatible
validation hints, and other uncertain values produce warnings and an
unconstrained `{}` where needed. User-defined JSON/text marshalers and
unmarshalers are detected by their actual method sets; because their wire shape
cannot be derived safely, they remain unconstrained with a diagnostic instead
of exposing their underlying Go representation.

Named contracts are not shape-deduplicated. Identity includes import path,
defined origin type, and concrete generic arguments. True Go aliases collapse
into their semantic target; pointer nullability is preserved at each use site,
and a pointer used as a generic argument remains part of that instantiation's
identity, rather than creating duplicate components for ordinary `*T` uses.
Component names use readable package/type semantics and generic argument
labels. Names are allocated against a global case-folded, sanitized namespace;
every member of a real collision receives a stable identity hash so discovery
order cannot privilege one owner. A type name that contains no portable ASCII
identifier characters falls back to `Contract`. Traversal order and unrelated
packages do not allocate `SchemaN`-style names or rename existing contracts.
Only exported constants whose exact declared type matches a named contract
define its enum; private sentinels and merely convertible constants are not
public wire vocabulary.

## Handler Metadata and Overrides

Ordinary handler documentation supplies useful OpenAPI prose without a second
annotation language. The first prose sentence becomes the summary, with Go's
required declaration-name prefix removed from that human-facing text, and
remaining prose becomes the description. An explicit `@openapi.summary` is
preserved verbatim. `@group` remains a tag source. In the absence of an explicit
tag, the handler package supplies a stable default tag.

Supported directives are:

```go
// @openapi.summary Register an account
// @openapi.description Creates the primary account record.
// @openapi.tag Accounts
// @openapi.security forjSession
// @openapi.security oauth read:accounts write:accounts
```

Repeated security directives are alternatives. Use
`@openapi.security none` to mark an operation explicitly public. Missing values,
unknown directives, and incompatible security declarations are diagnostics;
directive text is not copied into operation prose.

For contracts that source inference cannot express, the `webindex` library
offers programmatic overrides through `IndexOptions.OpenAPI`. General-purpose
overrides are currently a Go API, not a `.goforj.yml` schema. GoForj uses this
API for a narrower framework-owned bridge: project/App document identity and
the exact cookie security conventions generated by its Auth component.

`OpenAPIOptions` can provide:

- document title, version, and description
- declared security schemes
- global middleware-expression mappings for compatibility and preferred
  source-scoped middleware rules keyed by project-relative file, enclosing
  function, optional receiver, and exact expression
- operation overrides selected by handler import path, package, receiver,
  function, HTTP method, and path
- summary, description, and tags
- parameter requiredness, schema, and example
- request-body requiredness, media type, schema, and example
- response replacement, removal, description, media type, schema, and example
- per-operation security, including an explicit empty public policy

Every operation-override selector must match exactly one indexed operation. A
source-scoped middleware rule may match several operations, but all matches must
come from the one selected source declaration. Unknown schemes, stale or
overlapping middleware rules, ambiguous selectors, nonexistent inferred
parameters, invalid response keys, and dangling component references fail
projection before any artifact is published.

Projection overrides refine only `openapi.json`. Components not transitively
reachable from the final projected operations are pruned after overrides and
route omissions. Overrides do not erase uncertainty
from Manifest v2 or suppress strict-mode diagnostics. A lenient build can retain
the canonical warning while publishing an explicitly corrected OpenAPI
operation; strict acceptance will require a future manifest/type-level override.

## OpenAPI Projection

`openapi.json` is OpenAPI 3.0.3 and is intentionally cleaner than the canonical
manifest:

- router `:id` segments become OpenAPI `{id}` templates
- catch-all routes, `CONNECT`, and invalid route templates remain in Manifest
  v2 but are omitted because OpenAPI 3.0 has no faithful operation shape for
  them
- named components come only from Manifest v2 schemas
- anonymous response shapes remain inline
- raw Go syntax, `x-forj-type`, internal `METHOD:path` IDs, and anonymous
  `SchemaN` components are removed
- operation IDs derive from package, receiver, and function semantics; readable
  route roles disambiguate reuse and a short hash is the final collision fallback
- observed status codes use standard HTTP descriptions
- unresolved statuses and response-less handlers use `default`, never a
  fabricated success response
- distinct media types for one status are retained, and distinct contracts for
  one media type are represented with `anyOf`
- multiple distinct `Bind` targets become a request `anyOf` when every branch
  is typed, or an unconstrained request with a diagnostic otherwise
- an inferred `Bind` body is optional and exposes its typed JSON contract under
  `application/json` plus an unconstrained `*/*` fallback, because the runtime
  binder can accept non-JSON input; an explicit override can require, refine,
  or remove the body and its media contract
- a Blob with a dynamic media-type expression has no invented
  `application/octet-stream` content contract
- path parameters are required; other inferred scalar parameters remain string
  schemas unless explicitly overridden
- `Accept`, `Content-Type`, and `Authorization` reads are diagnosed and omitted
  as ordinary parameters because OpenAPI represents them through content
  negotiation or security
- invalid observed response statuses become `default`, and invalid observed
  media types are diagnosed and omitted instead of invalidating lenient output

Before publication, projection validates complete Operation Objects, path
templates, case-insensitive header uniqueness, response keys and payload rules,
media types, examples, security schemes and scopes, recursive OpenAPI 3.0 Schema
Objects, type-compatible schema keywords, discriminator mappings, and local
RFC 6901 JSON Pointer references. Explicitly malformed override configuration
is a hard projection error in both modes.

The `webindex` package never guesses security from a middleware name such as
`RequireAuth`. Callers must declare a scheme and either map captured middleware
provenance or supply operation metadata/override policy. Source-scoped rules
are preferred: the same expression in another file or declaration cannot
inherit a policy. Expression-only global mappings remain available for callers
whose spelling is globally unique. API keys support headers, queries, and
cookies; HTTP, OAuth2, and OpenID Connect schemes are also validated. Multiple
schemes in one requirement are ANDed, while separate requirements are OR
alternatives. Policies from multiple mapped middleware are combined as
requirements that must all pass.

GoForj supplies a small explicit convention when Auth is enabled for the active
App. It first proves that the active project module agrees with `.goforj.yml`
and that the selected `internal/auth` package contains the concrete generated
`Controller` and `Service` contract. The proof includes the controller's
`auth *Service` field, the framework middleware signature, the generated
authentication/error/success control flow, and the exact `auth_access` and
`auth_refresh` cookie constants. Aliases, no-op lookalikes, ignored/test files,
inactive build-constraint files, and files with another package clause cannot
provide that trust boundary.

After that proof, GoForj declares `authAccess` and `authRefresh` cookie API-key
schemes only for generated policy that flows into the active returned route
surface:

- group policy is a source-scoped rule for `authService.RequireAuth` in the
  selected composition declaration, after proving that identifier is the exact
  project-owned `*internal/auth.Service` parameter and rejecting lexical
  shadows
- controller policy is a source-scoped rule for `c.auth.RequireAuth` in the
  active `internal/auth.Controller.Routes` declaration, with receiver identity
  pinned to `Controller`, emitted only when the composition selects that exact
  controller provider
- either access or refresh cookie satisfies generated `RequireAuth`
- the middleware-free `POST /auth/refresh` operation receives a refresh-only
  override only after GoForj proves the exact generated route plus
  `RefreshSession` assignment, unauthorized error guard, and successful
  response flow

No generated middleware spelling is mapped globally. An identically spelled
middleware in another controller, provider, or composition declaration cannot
inherit cookie security. This also lets starter kits retain a generated shared
auth controller without documenting routes they do not expose. Unused schemes
are omitted, a similarly named custom middleware is not secured automatically,
and project-level Auth does not leak into a named App that did not select it.

GoForj also supplies OpenAPI info from `.goforj.yml`: the default App uses the
project name as its title, while a named App uses `<project> / <app>`. The
initial document version is `1.0.0`, and the description names the owning App.
When no project config exists, the zero-value options preserve `webindex`'s
fallback title behavior.

## Diagnostics and Strict Mode

Recoverable uncertainty is a deterministic warning rather than a silent drop.
Examples include:

- Go parse and selected-package type errors
- unsupported or dynamic composition expressions
- missing or ambiguous handlers
- dynamic parameter keys
- unresolved response statuses or media types
- invalid OpenAPI annotations
- unsupported schema types or ambiguous JSON fields
- handlers with no statically recognized response
- catch-all routes, `CONNECT`, invalid templates, or middleware expansions that
  cannot be projected faithfully
- reserved or duplicate headers, invalid observed response statuses, and
  invalid observed media types

Duplicate method/path operations are errors because OpenAPI can represent only
one operation for that key.

Normal mode rejects errors but permits warnings. Strict mode rejects warnings
and errors. A rejection returns the candidate manifest and an actionable
`DiagnosticsError`, but it does not publish any member of the artifact set.
OpenAPI projection/configuration errors have the same no-publication property.

The indexer accepts a real `context.Context`, checks cancellation throughout
filesystem parsing and before expensive or publishing work, and returns context
cancellation without exposing partial output.

## Artifact Lifecycle

There are two publication layers because the library and the framework have
different boundaries.

At the `webindex` layer, every JSON value is encoded before the filesystem is
touched. Changed files are written and synced to same-directory temporary files,
then renamed while holding process-local keyed locks layered over persistent
`.webindex-artifacts.lock` operating-system advisory locks for the destination
directories. The process-local layer is required on platforms whose advisory
locks do not serialize goroutines in one process. Lock directories are acquired
in canonical order and waits honor cancellation. Unchanged bytes are left in
place, preserving modification times. If a later rename fails, earlier files
are restored and rollback errors are joined with the original failure.

At the GoForj layer, `build` and `run` generate all three artifacts in a sibling
staging directory. GoForj then:

1. validates that every candidate exists and contains valid JSON
2. retains snapshots of both active and staged generations
3. runs the final build or run boundary
4. calls `webindex.AcquireArtifactPublicationLock` so it acquires the exact
   process-local and persistent sibling `.webindex-artifacts.lock` protocol
   used by direct `webindex` publishers
5. verifies under that lock that the active files did not change concurrently
6. publishes only changed files with same-filesystem renames
7. rolls back a partially published set on an error while still holding the lock
8. releases the lock and removes the staging directory on every exit path

Two writers that prepared the same generation converge successfully after lock
handoff; a writer is rejected only when the current complete set differs from
both its preparation snapshot and its validated candidate. Identical cleanup
writers likewise treat an already absent complete set as success.

For `build`, publication follows successful `go build`. For `run`, GoForj first
compiles the selected package into an exact temporary binary, starts that binary
with the requested app arguments and environment, and only then publishes the
candidate. A publication failure after start terminates and reaps that exact
process. The temporary executable is removed when the process exits.

Known CLI-only App cleanup is subject to the same final-step and lock boundary
in build and run pipelines. Stale files are first renamed to unique sibling
tombstones; a mid-set failure restores every moved file and joins rollback
errors with the triggering failure. Tombstones are deleted only after all three
active paths have been cleared. The standalone `build:api-index` command
publishes or cleans immediately because indexing is its final operation.

The three destination paths are still separate files, so no filesystem offers
a single crash-atomic transaction spanning the whole set. Same-file renames,
validation, compare-before-publish checks, and rollback cover ordinary errors
and concurrent writers; a process or machine crash between separate renames is
a remaining limitation.

## Generated Scalar Serving

Generated WebAPI Apps register:

```text
GET /swagger
GET /swagger/
GET /swagger/doc.json
```

The HTML routes use Scalar's API reference bundle pinned to
`@scalar/api-reference@1.62.5` and configure it to fetch
`/swagger/doc.json`. The pin prevents an unrelated CDN release from changing a
generated application's behavior.

The JSON handler serves only the current process App's artifact. `FORJ_APP`
selects the default or named path shown above only when it is a conventional
safe App slug; unsafe path-like values fall back to `app`.
`OPENAPI_SPEC_PATH` is the explicit arbitrary-path override. A missing document
returns JSON `404` with the exact
`forj [<app>] build:api-index` command to run. A configured directory returns
JSON `500`; other filesystem failures remain errors. There is no fallback from
a missing named-App document to the default App's document.

`API_SWAGGER_ENABLED` can disable the routes. The legacy `SWAGGER_ENABLED`
value remains a migration fallback.

## Reproducibility and Performance

Operations, schemas, diagnostics, security requirements, and generated maps are
normalized into deterministic order before JSON encoding. Source paths are
project-relative, JSON uses stable indentation and a final newline, and
component identities do not depend on checkout roots or discovery order.
Tests compare artifact bytes across different temporary roots and after adding
unrelated packages.

Performance relies on a cheap whole-tree syntax pass followed by type loading
only for selected contract-bearing handler packages. A syntax-focused warm
budget test covers 240 routes with a two-second ceiling, and medium/large
benchmarks track repeated indexing. Route-runtime parity coverage compares the
indexed method/path surface with the actual returned route groups for a static
fixture.

Changed-only publication avoids watcher loops and downstream rebuilds when the
contract bytes have not changed.

## Validation and Generated-Client Workflow

GoForj includes a hidden framework integration command:

```text
forj test:openapi
```

It creates a native `web.Context` fixture under `/tmp`, builds all three
artifacts, validates OpenAPI with the pinned
`openapitools/openapi-generator-cli:v7.6.0` Docker image, generates a Go client
with the same image, and compiles that generated client package with isolated
Go caches. `--silent`, `--keep`, and `--image` support CI output, investigation,
and an explicit tool-image override.

This command is an internal end-to-end guard for the framework projection; it
does not yet validate an arbitrary active project. Unit, golden, typed-schema,
route-parity, stability, lifecycle, and generated-template tests cover the
individual boundaries.

## Current Limitations

- Route discovery understands a conservative static composition vocabulary; it
  does not execute helpers, interpret arbitrary control flow, reflection, or
  runtime feature flags.
- Route paths, group prefixes, parameter keys, statuses, and media types that
  cannot be resolved statically remain diagnostics.
- WebSocket and `CONNECT` routes are not projected into OpenAPI. Router
  catch-all paths remain in Manifest v2 but are omitted from OpenAPI because a
  normal path-template variable would not preserve their multi-segment match
  semantics.
- Query, header, cookie, and path values default to string schemas; richer
  scalar policy requires an override.
- Semantic schemas depend on selected packages being loadable enough for
  `go/packages` to return type information. Partial type failures are visible.
- Custom JSON/text codecs and arbitrary runtime validation logic are not
  interpreted; detected codecs and incompatible declarative hints remain
  diagnostics until their contracts are supplied explicitly.
- Inferred `Bind` media remains conservative: JSON receives the typed schema,
  while `*/*` stays unconstrained until project policy supplies a narrower
  override.
- Programmatic OpenAPI overrides are available in `webindex`, but GoForj does
  not yet expose a general equivalent project-config model beyond its generated
  project/App metadata and Auth convention.
- Security requires explicit schemes and policy. GoForj supplies these for its
  generated Auth component only; other unmapped middleware remains present in
  the manifest without becoming OpenAPI security.
- The projection targets OpenAPI 3.0.3, not JSON Schema/OpenAPI 3.1.
- Publication is rollback-safe for ordinary errors but cannot make three fixed
  paths one crash-atomic filesystem object.
