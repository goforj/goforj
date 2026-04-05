# Web Abstraction Design Sketch

This document sketches a GoForj-owned web abstraction layer that can sit above Echo while keeping the door open for another adapter in the future.

Echo is an explicit influence on this design. The intent is not to hide that. The abstraction started as an adapter-oriented layer over Echo because Echo already provides a clean, pragmatic, high-quality reference point for route registration, context handling, middleware shape, and overall framework ergonomics.

The long-term goal is to grow more first-class GoForj experiences over time while preserving that clarity and keeping generated apps decoupled from any one engine.

Status:

- exploratory
- not committed to implementation
- intended to define the boundary before refactoring generated app code

## Goal

Decouple generated app code from Echo without recreating an entire web framework.

The abstraction should:

- let app code depend on GoForj-owned handler and context types
- keep Echo as the first concrete implementation
- allow another adapter later if the boundary holds up
- cover the common concerns application code needs every day
- avoid mirroring every feature of Echo or `net/http`

## Non-Goal

This is not an attempt to build a new web framework.

GoForj should not try to own:

- router internals
- raw HTTP server internals
- every request/response feature under the sun
- websocket implementation details
- multipart parsing internals
- transport-specific optimizations

Echo should keep doing the heavy lifting underneath the adapter.

GoForj should not try to expose every Echo feature through the core abstraction.

The right standard is:

- cover the portable application-facing surface cleanly
- keep Echo-specific capabilities in the adapter layer or behind controlled escape hatches

## Layering

This abstraction should exist as a standalone library first, with GoForj leveraging it at the framework layer.

Reason:

- the abstraction itself is useful independent of code generation
- adapters and middleware should be benchmarked and hardened in one place
- generated apps should not each own their own copy of the abstraction contract
- GoForj should build on the library, not redefine it locally per app

Recommended split:

- standalone library:
  - `github.com/goforj/web`
  - `github.com/goforj/web/adapter/echoweb`
- GoForj framework layer:
  - generated route wiring
  - framework conventions
  - bootstrap integration
  - app-level composition

Generated apps should ideally depend on the standalone library rather than a one-off local abstraction package.

## Compatibility Policy

Echo compatibility helpers may exist during migration, but they should be treated as legacy bridges rather than part of the preferred application-facing API.

Preferred application surface:

- `web.Context`
- `web.Handler`
- `web.Middleware`
- `webmiddleware`

Legacy escape hatches that may remain temporarily:

- `echoweb.WrapHandler(...)`
- `echoweb.WrapMiddleware(...)`
- GoForj-side helpers such as `NewEchoRoute(...)`

Policy:

- new generated code should not introduce new Echo-first route or middleware usage
- compatibility helpers should stay explicit and easy to identify
- if a bridge is used, it should be for migration or interop, not as the normal developer path

## Development Workflow

The best implementation path is to build the web abstraction library locally first and integrate it into GoForj with a local `replace` directive during development.

Reason:

- gives a fast feedback loop while the API is still moving
- forces the abstraction to stand on its own as a real library
- makes it easier to benchmark and test the library in isolation
- avoids overfitting the design to generated-app internals too early

Suggested workflow:

1. create the standalone library locally, for example:
   - `github.com/goforj/web`
   - `github.com/goforj/web/adapter/echoweb`
2. add a local `replace` in GoForj during active development
3. iterate on interfaces, adapter behavior, and benchmarks in the library
4. wire GoForj into the local library only after the library surface feels stable
5. then update generators/templates to depend on the library intentionally

Example GoForj `go.mod` development setup:

```go
replace github.com/goforj/web => ../web
```

Recommended discipline:

- keep hot-path benchmarks in the library repo
- keep adapter-focused tests in the library repo
- use GoForj mainly to validate integration, generated app ergonomics, and migration impact

This keeps the design honest:

- the library must be good on its own
- GoForj integration should leverage it, not compensate for weak library boundaries

## Proposed Package Shape

Recommended package split:

- `github.com/goforj/web`
  app-facing abstraction
- `github.com/goforj/web/adapter/echoweb`
  Echo adapter
- generated app local packages such as `internal/http`
  transport/bootstrap/server setup only, if still needed

Reasoning:

- `web` is clearer than `http` for the app-facing abstraction
- `http` is already overloaded with stdlib and transport concerns
- handlers/controllers should depend on `github.com/goforj/web`, not directly on Echo

## Core Principle

GoForj should abstract the application-facing surface, not the engine internals.

That means owning:

- handler signature
- middleware signature
- request context
- route registration
- request parameter access
- binding
- response helpers
- request-scoped values
- error propagation

That should be enough for:

- controllers
- middleware
- auth/session flows
- JSON APIs
- normal web responses

It should not be interpreted as:

- “everything Echo can do must appear in `goforj/web`”

Instead it should mean:

- “everything application code commonly needs should be expressible through `goforj/web`”
- “Echo-specific capabilities remain available through the adapter when they do not generalize cleanly”

## Minimal App-Facing Types

### Handler and Middleware

```go
type Handler func(Context) error

type Middleware func(Handler) Handler
```

### Router

```go
type Router interface {
	Use(...Middleware)

	Get(path string, handler Handler, middleware ...Middleware)
	Post(path string, handler Handler, middleware ...Middleware)
	Put(path string, handler Handler, middleware ...Middleware)
	Delete(path string, handler Handler, middleware ...Middleware)
	Patch(path string, handler Handler, middleware ...Middleware)

	Group(prefix string, middleware ...Middleware) Router
}
```

This keeps route declaration portable without forcing GoForj to own routing internals.

### Context

```go
type Context interface {
	Context() context.Context

	Method() string
	Path() string
	RequestURI() string

	Param(name string) string
	Query(name string) string
	Header(name string) string
	Cookie(name string) (Cookie, bool)

	Bind(v any) error

	Set(key string, value any)
	Get(key string) any

	Status(code int)
	JSON(code int, v any) error
	Text(code int, body string) error
	HTML(code int, body string) error
	NoContent(code int) error
	Redirect(code int, url string) error

	Native() any
}
```

This is intentionally narrow.

The abstraction should only cover the set of operations app code should rely on regularly.

## Why `Native() any` Exists

There should be a deliberate escape hatch.

Reason:

- sometimes app code or migration code will need access to framework-specific features
- without an escape hatch, framework leakage tends to happen in worse ways

Rule:

- app/business code should avoid `Native()`
- adapter or transitional code may use it when necessary

This is a pressure valve, not the primary API.

## Request and Response Concerns

The abstraction should support:

- path params
- query params
- headers
- cookies
- request body binding
- request-scoped values
- common response types

It should not initially try to solve:

- streaming APIs
- SSE
- websockets
- every flavor of file handling
- every transport-specific optimization

Those can be added later only if the abstraction proves it can carry them cleanly.

## Feature Coverage Boundary

Echo supports many features beyond the core app-facing request lifecycle, including:

- static file serving
- transport-specific middleware options
- proxy behavior
- websocket-oriented integration points
- template/rendering integrations
- other low-level server concerns

GoForj should distinguish between:

### Features the abstraction should own

- handler and middleware contracts
- route registration and grouping
- path/query/header/cookie access
- request binding
- common responses
- request-scoped values
- normal error propagation

### Features the adapter may expose without promoting into the abstraction

- static file engine details
- specialized Echo middleware options
- reverse proxy features
- websocket-specific behavior
- advanced framework-specific hooks

This keeps the abstraction honest.

If a feature does not generalize cleanly across frameworks, it should not be forced into `goforj/web` just because Echo happens to provide it.

## Error Model

The abstraction should preserve the normal handler contract:

```go
type Handler func(Context) error
```

That keeps middleware composition simple and fits Echo naturally.

Potentially useful framework-owned helpers later:

- `web.NotFound(...)`
- `web.Unauthorized(...)`
- `web.BadRequest(...)`
- `web.JSONError(...)`

But those should be helpers layered on top, not required for the first version.

## Echo Adapter Shape

Echo should be the first adapter only.

Possible adapter structure:

```go
package echo

type Adapter struct {
	engine *echo.Echo
}

func New(engine *echo.Echo) *Adapter
func (a *Adapter) Router() web.Router
```

And an Echo-backed context:

```go
type contextAdapter struct {
	echo echo.Context
}
```

Responsibilities of the adapter:

- translate `web.Handler` into Echo handler functions
- translate `web.Middleware` into Echo middleware
- implement `web.Context` on top of `echo.Context`
- keep Echo-specific behavior contained here

## Middleware Interop

The abstraction should support a clean path for using existing Echo middleware inside the GoForj `web` layer.

This matters because:

- the framework already depends on Echo today
- generated apps may already rely on Echo middleware
- migration will be easier if middleware can be adapted instead of rewritten immediately

### Goal

Allow these two worlds to coexist:

- GoForj-owned `web.Middleware`
- native Echo middleware

without forcing app code to choose one or the other everywhere all at once.

### GoForj Middleware Shape

```go
type Middleware func(Handler) Handler
```

### Echo Middleware Shape

Echo uses:

```go
type MiddlewareFunc func(echo.HandlerFunc) echo.HandlerFunc
```

### Adapter Strategy

The Echo adapter should expose a helper that wraps Echo middleware into GoForj middleware.

Possible shape:

```go
func WrapEchoMiddleware(mw echo.MiddlewareFunc) web.Middleware
```

Conceptually, the adapter would:

1. adapt `web.Handler` into `echo.HandlerFunc`
2. pass that through the Echo middleware
3. adapt the resulting Echo handler back into a `web.Handler`

Sketch:

```go
func WrapEchoMiddleware(mw echo.MiddlewareFunc) web.Middleware {
	return func(next web.Handler) web.Handler {
		return func(ctx web.Context) error {
			native, ok := ctx.Native().(echo.Context)
			if !ok {
				return fmt.Errorf("web/echo: native context is not echo.Context")
			}

			echoNext := func(ec echo.Context) error {
				return next(wrapContext(ec))
			}

			return mw(echoNext)(native)
		}
	}
}
```

The exact implementation can vary, but the key point is:

- middleware interop belongs in the adapter package
- app/business code should not hand-roll these conversions

### Recommended Usage

Inside Echo-backed apps, route setup should be able to do things like:

```go
router.Use(
	webecho.WrapEchoMiddleware(middleware.RequestID()),
	webecho.WrapEchoMiddleware(middleware.Recover()),
)
```

That gives a clean migration path:

- existing Echo middleware can stay in use
- new GoForj middleware can be authored in `web.Middleware` form
- app code can gradually move toward the GoForj surface

### Important Constraint

Interop is a bridge, not the end state.

The preferred long-term model should still be:

- application middleware written against `web.Middleware`
- Echo middleware used only where there is no real value in rewriting or where the feature is truly Echo-specific

If too much critical behavior remains trapped in Echo-native middleware, the abstraction will not actually decouple app code.

### Testing Expectations

Middleware interop should be covered by tests that verify:

- request-scoped values survive the adapter boundary
- short-circuit middleware still works correctly
- errors propagate correctly
- wrapping Echo middleware does not introduce surprising allocations or extra handler invocations

Suggested cases:

- Request ID middleware
- auth middleware that aborts the request
- recovery middleware
- logging middleware

## Migration Strategy

This should be done in phases.

### Phase 1

Define the `internal/web` interfaces and the Echo adapter.

Do not try to migrate the whole generated app immediately.

### Phase 2

Update generated controllers and route registration to depend on `web` instead of Echo.

That means:

- controller methods take `web.Context`
- middleware uses `web.Handler`
- route registration uses `web.Router`

### Phase 3

Move any remaining Echo-specific helpers into the adapter or replace them with `web` helpers.

### Phase 4

Only after the boundary feels stable should a second adapter even be considered.

The point is to create a clean contract first, not to rush into multiple implementations.

## Design Constraints

The abstraction will fail if it becomes:

- a 1:1 mirror of Echo
- a giant lowest-common-denominator API
- too small to express the common app use cases

The right target is:

- enough for ordinary generated app work
- small enough to reason about
- strong enough that app code can stop importing Echo directly

## Performance And Allocation Budget

This abstraction only makes sense if the request lifecycle stays thin.

If GoForj adds a layer on top of Echo, that layer should not introduce meaningful per-request allocation overhead beyond what Echo already does.

This should be treated as a design constraint, not a cleanup item.

### Performance Goals

- avoid unnecessary per-request heap allocations
- avoid wrapping request state in multiple short-lived objects when one adapter object is enough
- avoid reflection-heavy helper layers beyond what binding already requires
- avoid converting back and forth between intermediate request/response representations
- keep middleware composition straightforward and allocation-light

### Practical Design Implications

Echo already reuses its native request context objects through a context pool.

That means the GoForj abstraction should be designed to preserve that advantage, not accidentally erase it.

The adapter should prefer:

- a thin `web.Context` wrapper over `echo.Context`
- direct delegation to Echo for request/response operations
- minimal copying of headers, params, query values, and request-scoped data
- no generic request envelope allocation per request unless there is a clear reason
- no additional wrapper churn that defeats Echo's pooled request lifecycle

The abstraction should avoid:

- building large request wrapper structs eagerly
- copying response payloads into intermediate framework-owned buffers
- separate routing metadata objects when Echo already owns that state
- “clean architecture” layers that allocate just to repackage the same request data

Concrete implication:

- if Echo gives GoForj one pooled native context per request, the adapter should ideally add at most one thin wrapper object on top of it
- repeated wrapper creation through middleware hops or helper layers should be treated as a regression

### Testing Strategy

Performance should be verified with automated tests, not treated as a vague aspiration.

The web abstraction should have:

- focused benchmarks for hot-path handler execution
- allocation assertions for representative request lifecycles
- direct comparisons between:
  - raw Echo handler path
  - GoForj `web` handler path over the Echo adapter

### Benchmark Examples

Suggested benchmarks:

- simple GET route with no bind
- JSON POST route with body bind
- middleware chain with request-scoped values
- JSON response path

Examples:

```go
func BenchmarkEchoDirectJSON(b *testing.B) {}
func BenchmarkWebAdapterJSON(b *testing.B) {}
func BenchmarkEchoDirectBind(b *testing.B) {}
func BenchmarkWebAdapterBind(b *testing.B) {}
```

### Allocation Assertions

Use Go benchmark tooling to measure:

- `ns/op`
- `B/op`
- `allocs/op`

The important constraint is not “zero allocations at all costs.” It is:

- no surprising new allocations caused by the abstraction layer itself
- no large regression compared to direct Echo usage

The acceptable threshold should be explicit.

For example:

- equal allocations on the simplest routes is ideal
- a very small fixed overhead may be acceptable
- multi-allocation regressions on every request are not

### Regression Policy

If the abstraction introduces measurable request-path overhead, that should block the design until the source is understood.

The wrong outcome would be:

- a cleaner abstraction on paper
- but a fatter request lifecycle in practice

The correct standard is:

- app-facing decoupling
- while preserving Echo-like thinness on the hot path

### Recommendation

Before migrating generated handlers broadly:

1. build the Echo adapter
2. benchmark direct Echo vs adapted handlers
3. add allocation-focused regression tests
4. only proceed once the overhead is proven acceptably small

That gives the abstraction a performance contract, not just an API contract.

## Recommendation

Use:

- `github.com/goforj/web` for the abstraction
- `github.com/goforj/web/echo` for the adapter
- `internal/http` only for server/bootstrap concerns

Start with:

- handlers
- middleware
- router
- context
- binding
- JSON/text/HTML/no-content/redirect responses
- request-scoped values

Do not start with:

- websockets
- streaming
- advanced file/multipart APIs
- every Echo feature

That is the smallest design that has a real chance of decoupling app code from Echo without turning GoForj into its own web framework.
