# Go 1.27 Generic Methods API Design

## Status

- Design status: proposed
- Research date: 2026-07-13
- Language status: accepted for Go 1.27; available in Go 1.27 RC1; not yet stable
- Expected stable release: August 2026, according to the draft release notes
- Scope: GoForj-owned sibling libraries and the generated App surface that consumes them
- Release policy decision: a minimum-Go increase ships in a minor release, never
  a patch; a major is reserved for incompatible API or behavior changes
- Revalidation required: refresh the release and repository facts before implementation

## Decision Summary

GoForj should adopt generic methods after Go 1.27 is stable, but only where a
concrete receiver already owns the operation.

The first adoption wave should migrate the 34 public generic functions that
exist primarily because Go could not previously attach their type parameters to
a method:

- 9 functions in `cache`
- 11 functions in `collection`
- 14 functions in `httpx`

Ship also has one internal receiver-style generic function that is a useful
low-risk canary.

The migrations should be additive:

- add the generic method
- keep the current package function as a forwarding compatibility wrapper
- preserve behavior, error contracts, and encoding policy
- update documentation generators to distinguish package functions from methods

Generic methods should not be forced onto interface-first APIs. In particular:

- keep `events.API` unchanged
- keep `web.Context`, `web.WebSocketConn`, and `web.Router` unchanged
- keep `storage.Storage` byte-oriented and unchanged
- keep cache's interface contracts non-generic

`execx.DecodeChain.As[T]` and queue payload result methods are strong adjacent
additions, but they are new API rather than migrations of existing generic
package functions. They should follow the direct receiver migrations.

Do not publish any production module using the new syntax before Go 1.27 is
stable and the GoForj tooling gates in this design pass.

The Go 1.27 floor does not by itself trigger new major module paths. Public
generic methods are additive, and the existing functions remain compatible.
Affected `v0`, `v1`, and `v2` modules should use their next minor release and
announce the new floor prominently. Patch releases must not raise the minimum
Go version.

## Why This Fits GoForj

GoForj separates generated App policy from reusable sibling primitives:

- sibling repos own reusable cache, collection, HTTP, queue, web, events, and
  storage behavior
- `goforj` owns templates, generated composition, wiring, and developer workflow

Generic methods do not change that ownership. The method belongs in the sibling
repo that owns the concrete type. GoForj should consume the released method in
generated templates only after the sibling API is stable.

This matters most where generated Apps already expose concrete objects:

- cache managers return `*cache.Cache`
- HTTP clients are `*httpx.Client`
- job handlers receive `queue.Message`, an alias of concrete `bus.Context`

It matters much less where generated Apps intentionally depend on abstractions:

- events expose `events.API`
- storage accessors return `storage.Storage`
- HTTP handlers receive `web.Context`

The existing abstraction boundary is more valuable than method-call syntax.

## What Go 1.27 Changes

### New syntax

Before Go 1.27, a method can use type parameters declared by its receiver type,
but it cannot declare new type parameters of its own:

```go
type Collection[T any] struct {
	items []T
}

// This is legal before Go 1.27 because T belongs to Collection.
func (c *Collection[T]) Filter(fn func(T) bool) *Collection[T]
```

A type-changing operation therefore has to be a package function:

```go
func MapTo[T, R any](
	c *Collection[T],
	fn func(T) R,
) *Collection[R]
```

Go 1.27 permits the method to declare `R`:

```go
func (c *Collection[T]) MapTo[R any](
	fn func(T) R,
) *Collection[R]
```

The proposal extends method declarations with an optional type-parameter list.
Calls use the same explicit type arguments and inference rules as generic
functions. Method values and method expressions are generic functions when the
selected method is generic.

The standard library's first documented example is `math/rand/v2.Rand.N`, which
places the existing typed operation in the namespace of a particular random
source.

### What does not change

Generic methods are concrete methods only.

The following remain true:

- interface methods cannot declare type parameters
- a generic concrete method does not implement a same-named non-generic
  interface method
- generic methods are unavailable through reflective method discovery; an
  explicitly instantiated method value can still be reflected as an ordinary
  function value
- methods can only be declared on local receiver types
- type inference does not infer a result-only type parameter from assignment
  context
- a method-specific type parameter cannot strengthen the constraint of a
  receiver type parameter
- Go still has no method overloading

These limits drive most of the decisions in this design.

### Inference examples

An input normally supplies enough information:

```go
c.Set("profile:42", profile, time.Minute) // T inferred as Profile
```

A result-only type still needs brackets:

```go
profile, ok, err := c.Get[Profile]("profile:42")
```

That is not a defect in generic methods; it is the same inference rule generic
functions use today.

## Goals

- Move receiver-owned generic operations into the receiver's namespace.
- Restore left-to-right fluent composition where package functions currently
  interrupt it.
- Improve discoverability through completion on concrete values.
- Remove the repeated instance parameter from APIs that already require a
  concrete instance.
- Preserve existing behavior and compatibility during migration.
- Keep GoForj's interface and driver boundaries intact.
- Record explicit no-change decisions for every audited primary GoForj module.

## Non-Goals

- Redesign every API merely because generic methods are available.
- Add generic type parameters to input-only `any` methods without a meaningful
  type relationship.
- Replace interfaces with concrete types to gain method syntax.
- Promise performance improvements; the main benefits are organization,
  composition, discoverability, and compile-time result typing.
- Remove JSON, codec, or reflection work that remains semantically necessary.
- Remove current package functions in the first adoption wave.
- Raise the Go version of sibling modules that receive no useful generic method.

## Adoption Test

A candidate should normally satisfy all of these conditions:

1. Real callers already hold a concrete local type.
2. The operation semantically belongs to that type.
3. A method-specific type parameter varies from call to call.
4. The type parameter relates an input to an output, changes a result type, or
   removes caller-side destination assertions or pointers.
5. The method remains useful without passing through an interface.
6. The name does not collide with a materially different existing method.
7. Reflection is not required to discover or invoke the method.

A generic function whose first parameter is the owning concrete instance is the
strongest migration signal. It is not an automatic rule: foreign receiver types,
receiver-constraint limitations, and symmetric multi-input operations can still
make the package function the correct shape.

## Audit Summary

The table covers the primary public API repositories. Nested driver, core,
testing, integration, docs, and examples modules were also searched for the
receiver-workaround pattern; they produced no additional public candidates.
Examples include the cache, events, storage, and mail auxiliary modules. Their
Go directives and release tags may still need coordinated updates when they
select an affected Go 1.27 dependency.

| Repository | Current Go | Generic receiver-style audit | Decision |
|---|---:|---:|---|
| `cache` | 1.24.4 | 9 | Adopt methods; keep function wrappers. |
| `collection` | 1.24.4 | 11 direct, 10 false positives | Adopt the 11 direct methods; retain constrained functions. |
| `httpx/v2` | 1.24.4 | 14 | Adopt `Client` methods; keep `Do[T]` free. |
| `ship` | 1.26.1 | 1 internal | Use as a post-stable canary. |
| `execx` | 1.24.4 | 0 | Add `DecodeChain.As[T]` as new API. |
| `queue` | 1.24.4 | 0 | Add payload result methods as new API; retain `Bind`. |
| `events` | 1.24.0 | 0 | Do not put generic methods on the interface-led surface. |
| `web` | 1.25.0 | 0 | Prefer generic package helpers, which are legal today. |
| `storage` | 1.24.4 | 0 | No core change; keep raw-byte interface. |
| `str/v2` | 1.24.0 | 0 | No change. |
| `env/v2` | 1.24.4 | 0 | No migration; evaluate any new typed parser separately. |
| `crypt` | 1.24.4 | 0 | No migration; structured-value methods are separate scope. |
| `null/v6` | 1.22.12 | 0 | Constructors stay functions; do not diverge the proxy fork. |
| `atlas` | 1.25.0 | 0 | No change. |
| `godump` | 1.18 | 0 | No change. |
| `goforj` | 1.25.0 | 0 | Tooling and generated-consumer work only. |
| `mail` | 1.25.0 | 0 | No change. |
| `metrics` | 1.25.0 | 0 | No change. |
| `scheduler/v2` | 1.24.4 | 0 | No change. |
| `wgo` | 1.24.0 | 0 | No change. |
| `wire` | 1.19 | 0 | No API migration; parser readiness is a release gate. |

The public receiver-migration count is therefore 34, concentrated in three
repos. That concentration is useful: the rollout should be deliberate, not a
mechanical all-repo Go version bump.

## Direct Migration: Cache

### Current workaround

`cache.Cache` is concrete, but typed operations are package functions because
their `T` changes per key and call:

```go
profile, ok, err := cache.Get[Profile](
	c.WithContext(ctx),
	"profile:42",
)

err = cache.Set(
	c.WithContext(ctx),
	"profile:42",
	profile,
	time.Minute,
)
```

GoForj templates use this shape repeatedly in auth, inspections, application
settings, notifications, and monitoring code.

### Proposed methods

```go
func (c *Cache) GetJSON[T any](key string) (T, bool, error)
func (c *Cache) Get[T any](key string) (T, bool, error)

func (c *Cache) RefreshAhead[T any](
	key string,
	ttl time.Duration,
	refreshAhead time.Duration,
	fn func() (T, error),
) (T, error)

func (c *Cache) RefreshAheadValueWithCodec[T any](
	key string,
	ttl time.Duration,
	refreshAhead time.Duration,
	fn func() (T, error),
	codec ValueCodec[T],
) (T, error)

func (c *Cache) SetJSON[T any](
	key string,
	value T,
	ttl time.Duration,
) error

func (c *Cache) Set[T any](
	key string,
	value T,
	ttl time.Duration,
) error

func (c *Cache) Pull[T any](key string) (T, bool, error)

func (c *Cache) RememberStale[T any](
	key string,
	ttl time.Duration,
	staleTTL time.Duration,
	fn func() (T, error),
) (T, bool, error)

func (c *Cache) Remember[T any](
	key string,
	ttl time.Duration,
	fn func() (T, error),
) (T, error)
```

The custom-codec function currently takes `context.Context` before `*Cache`.
The method should follow the rest of the concrete cache API and use the context
bound by `WithContext`. Its legacy function must preserve its explicit-context
contract through the existing private context-aware implementation.

This helper is therefore the one cache migration that is not a mechanical
first-argument move. The method and compatibility function should enter the
same private implementation with their respective resolved contexts. Add parity
tests for nil-context normalization, cancellation, and the context delivered to
cache observers.

### Resulting usage

```go
profile, ok, err := c.WithContext(ctx).Get[Profile]("profile:42")
err = c.WithContext(ctx).Set("profile:42", profile, time.Minute)

profile, err = c.WithContext(ctx).Remember(
	"profile:42",
	time.Minute,
	loadProfile,
)
```

`Set`, `SetJSON`, `Remember`, `RememberStale`, `RefreshAhead`, and
`RefreshAheadValueWithCodec` normally infer `T` from a value, callback, or
codec. `Get`, `GetJSON`, and `Pull` require an explicit result type.

### Compatibility

Keep all nine current functions and forward them to the methods or their shared
private implementation:

```go
func Get[T any](c *Cache, key string) (T, bool, error) {
	return c.Get[T](key)
}
```

`CacheAPI` and its smaller interfaces remain non-generic. This is consistent
with today's typed functions, which already require `*Cache` rather than
`CacheAPI`.

Do not add new typed batch or codec variants merely to expand the method set.
Evaluate those from real use cases.

## Direct Migration: Collection

### Current workaround

`Collection[T]` already has methods that only use `T`. Operations that introduce
a different result, key, or peer type are package functions:

```go
names := collection.MapTo(
	collection.New(users).Filter(active),
	func(user User) string { return user.Name },
)

groups := collection.GroupBy(
	names,
	func(name string) int { return len(name) },
)
```

### Proposed methods

```go
func (c *Collection[T]) MapTo[R any](
	fn func(T) R,
) *Collection[R]

func (c *Collection[T]) MinBy[K Number | ~string](
	keyFn func(T) K,
) (T, bool)

func (c *Collection[T]) MaxBy[K Number | ~string](
	keyFn func(T) K,
) (T, bool)

func (c *Collection[T]) ToMap[K comparable, V any](
	keyFn func(T) K,
	valueFn func(T) V,
) map[K]V

func (c *Collection[T]) GroupBy[K comparable](
	keyFn func(T) K,
) map[K]*Collection[T]

func (c *Collection[T]) GroupBySlice[K comparable](
	keyFn func(T) K,
) map[K][]T

func (c *Collection[T]) CountBy[K comparable](
	keyFn func(T) K,
) map[K]int

func (c *Collection[T]) UniqueBy[K comparable](
	keyFn func(T) K,
) *Collection[T]

func (c *Collection[T]) Pipe[R any](
	fn func(*Collection[T]) R,
) R

func (c *Collection[T]) Zip[U any](
	other *Collection[U],
) *Collection[Tuple[T, U]]

func (c *Collection[T]) ZipWith[U, R any](
	other *Collection[U],
	fn func(T, U) R,
) *Collection[R]
```

These methods restore the fluent surface that the collection package is trying
to provide:

```go
groups := collection.New(users).
	Filter(active).
	MapTo(func(user User) string { return user.Name }).
	UniqueBy(strings.ToLower).
	GroupBy(func(name string) int { return len(name) })
```

All current functions remain as wrappers.

### Preserve `MapTo`

Do not rename `MapTo` to `Map` during the additive migration. The package
already has a same-type mutable `Collection.Map(func(T) T)`. Go cannot overload
that method with a type-changing `Map[R]`, and changing the existing method's
semantics would be a separate breaking design.

### Adjacent addition

A heterogeneous accumulator becomes possible and would fill a real gap:

```go
func (c *Collection[T]) ReduceTo[R any](
	initial R,
	fn func(R, T) R,
) R
```

Use a new name such as `ReduceTo` or `Fold`. Do not silently change the existing
same-type `Reduce` method in the compatibility release.

### Functions that must remain functions

The following functions require the receiver element itself to be comparable:

- `Contains`
- `CountByValue`
- `Difference`
- `Intersect`
- `SymmetricDifference`
- `TakeUntil`
- `Union`
- `UniqueComparable`

`Collection[T]` declares `T any`. A method-specific parameter cannot restate or
strengthen that receiver parameter as `comparable`, so Go 1.27 does not make
these valid methods.

`ToMapKV` must also remain a function. A method on `Collection[T]` cannot express
that it exists only when `T` is exactly `Pair[K, V]`, nor can it extract fresh
`K` and `V` parameters from that equality.

`Window` can become a method for consistency, but it could already have been a
method before Go 1.27 because it introduces no new type parameter. Track it as
ordinary API cleanup, not as a generic-method migration.

Constructors such as `New`, `NewNumeric`, `FromMap`, and `Times` naturally remain
functions.

## Direct Migration: HTTPX

### Current workaround

`httpx.Client` is concrete and one client intentionally decodes many unrelated
response types. Today all typed verbs therefore take the client as their first
argument:

```go
response, err := httpx.Post[CreateUser, CreateUserResponse](
	client,
	"/users",
	CreateUser{Name: "Ana"},
)
```

There are 14 direct candidates:

- `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, and `Options`
- `GetCtx`, `PostCtx`, `PutCtx`, `PatchCtx`, `DeleteCtx`, `HeadCtx`, and
  `OptionsCtx`

### Proposed methods

```go
func (c *Client) Get[Out any](
	url string,
	opts ...Option,
) (Out, error)

func (c *Client) Post[Out any](
	url string,
	body any,
	opts ...Option,
) (Out, error)

func (c *Client) Put[Out any](
	url string,
	body any,
	opts ...Option,
) (Out, error)

func (c *Client) Patch[Out any](
	url string,
	body any,
	opts ...Option,
) (Out, error)

func (c *Client) Delete[Out any](
	url string,
	opts ...Option,
) (Out, error)

func (c *Client) Head[Out any](
	url string,
	opts ...Option,
) (Out, error)

func (c *Client) Options[Out any](
	url string,
	opts ...Option,
) (Out, error)
```

Add matching context methods with `ctx context.Context` as the first ordinary
argument.

The new body methods need only `Out`. The current `In` parameter is unconstrained
and is immediately passed into an `any` body path, so it establishes no useful
compile-time relationship. Removing it from the new method also fixes calls
that currently need `Post[any, Response](client, url, nil)`.

```go
response, err := client.Post[CreateUserResponse](
	"/users",
	CreateUser{Name: "Ana"},
)
```

The existing two-parameter package function remains compatible and delegates:

```go
func Post[In, Out any](
	client *Client,
	url string,
	body In,
	opts ...Option,
) (Out, error) {
	return client.Post[Out](url, body, opts...)
}
```

Keep `Head[Out]` as an exact migration in this compatibility wave even though a
decoded body is a weak contract for a HEAD response. A different response-metadata
API would be a separate HTTPX design; mixing it into this migration would lose
behavior parity and prevent the legacy function from being a simple wrapper.

### `Do[T]` stays a function

`Do[T]` accepts `*req.Request`, a non-local type, and does not use `httpx.Client`.
HTTPX cannot declare a method on that receiver. Attaching it to `Client` would
invent ownership and potentially change error-mapper behavior, so it remains a
package-level escape hatch.

## Internal Canary: Ship

Ship has one internal instance-passing generic function:

```go
func Decode[T any](cmd Command) (T, error)
```

It has 27 production call sites and can become:

```go
func (cmd Command) Decode[T any]() (T, error)
```

```go
// Before
request, err := lighthouse.Decode[BenchmarkRunRequest](cmd)

// After
request, err := cmd.Decode[BenchmarkRunRequest]()
```

Because the package is internal, Ship can migrate atomically without a public
compatibility wrapper. This is a useful post-stable canary for compiler, editor,
lint, documentation, and CI readiness before public sibling releases.

## Adjacent Addition: Execx

Execx has no receiver-passing generic function today. Its decode chain instead
ends in a caller-owned pointer:

```go
var payload Payload
err := execx.Command(...).
	DecodeJSON().
	FromStdout().
	Trim().
	Into(&payload)
```

Add:

```go
func (d *DecodeChain) As[T any]() (T, error)
```

```go
payload, err := execx.Command(...).
	DecodeJSON().
	FromStdout().
	Trim().
	As[Payload]()
```

This removes the temporary variable, pointer ceremony, and caller-triggered
non-pointer destination error. It preserves the same decoder, source selection,
trimming, and error behavior.

Keep `Into(any)` for compatibility and for callers that intentionally populate
an existing object. Keep `Decoder.Decode([]byte, any)` non-generic because a
decoder implementation must handle many destination types through an interface.

## Adjacent Addition: Queue

Queue has no receiver-passing generic function today, but its concrete payload
owners expose pointer-based binding:

```go
var payload EmailPayload
if err := msg.Bind(&payload); err != nil {
	return err
}
```

Add methods to both concrete payload values:

```go
// In package queue.
func (j Job) PayloadAs[T any]() (T, error)

// In package bus.
func (c Context) PayloadAs[T any]() (T, error)
```

`queue.Message` is an alias of `bus.Context`, so generated handlers receive the
method through the existing root API:

```go
payload, err := msg.PayloadAs[EmailPayload]()
if err != nil {
	return err
}
```

`PayloadAs` avoids collisions with the existing `Job.Payload(any)` setter and
the existing `Bind(any)` methods.

Queue's generated documentation must account for the alias: the declaration is
physically `bus.Context.PayloadAs`, while users encounter it as
`queue.Message.PayloadAs`. Add an alias-aware root entry or an explicit root
example so the preferred API is discoverable.

Keep `Bind(any)` permanently. Lower-level queue interfaces and adapters rely on
it, and a generic method cannot replace an interface method.

Do not genericize `Payload(any)`, `PayloadJSON(any)`, or dispatch setters. They
are input-only APIs and an unconstrained type parameter would add no safety.

A later queue-specific design may consider typed handler registration:

```go
func (q *Queue) RegisterPayload[T any](
	jobType string,
	handler func(context.Context, Message, T) error,
)
```

That work needs decisions about binding-error wrapping, job metadata, GoForj's
generated queue manager, and whether a typed job descriptor should connect
dispatch to registration. Pilot `PayloadAs` first.

## Interface-First Repositories

### Events

The common event surface is `events.API`, including `WithContext`, `Publish`, and
`Subscribe`. Generic interface methods are illegal, and `WithContext` returns
the interface, so a generic concrete method on `Bus` would disappear in normal
injected usage.

Do not replace `Subscribe(any)` or add a privileged concrete-only subscription
tier. If compile-time validation of a canonical handler shape is useful, use a
generic package helper:

```go
func Subscribe[T any](
	bus API,
	handler func(context.Context, T) error,
) (Subscription, error)
```

That helper is legal before Go 1.27 and works with real and fake implementations.
`Publish(any)` is input-only and gains no useful invariant from `Publish[T]`.

### Web

The attractive typed operations live on `web.Context` and `web.WebSocketConn`,
which are interfaces implemented by adapters. Generic methods cannot be added to
those contracts.

Use package helpers instead:

```go
func Bind[T any](ctx Context) (T, error)
func Value[T any](ctx Context, key string) (T, bool)
func ReadJSON[T any](conn WebSocketConn) (T, error)
```

These helpers are also legal before Go 1.27. They can improve generated handler
code without weakening the web abstraction, but they need the `webindex`
readiness work described below.

Do not add a `web.Typed(ctx)` wrapper solely to recover method syntax.

### Storage

`storage.Storage` intentionally moves bytes across driver boundaries. Its
manager and GoForj accessors return that interface.

Do not add `Storage[T]`, change `Manager.Disk` to return a concrete facade, or
conflate byte access with serialization merely to create a generic method.
Generic JSON helpers could be package functions today if structured object
storage becomes an explicit product requirement. That is a codec-scope decision,
not a Go 1.27 migration.

## Explicit No-Change Decisions

- `str`: `String` is already fluent and method-based. `Parse[T]` would be less
  discoverable than `Int`, `Float64`, and `Bool` and would require new runtime
  parsing policy.
- `env`: `Scope` already has typed, named getters. A generic `Value[T]` would be
  a new scalar parsing abstraction, not a receiver-function migration.
- `crypt`: `Cipher` already owns encryption methods. Typed JSON encryption may
  be useful later but adds serialization policy and is separate scope.
- `null`: `NewValue`, `ValueFrom`, and `ValueFromPtr` are constructors and have
  no existing receiver. `Value[T].Map[U]` is a valid language example but would
  be new API in a proxy fork that should track upstream.
- `atlas`: its production generic helper is private and stateless; attaching it
  to the server would invent a receiver relationship.
- `godump`: `Dumper` already mirrors the package-level default-instance API.
- `mail`: builders, mailers, and drivers already own their operations.
- `metrics`: `Registry` already owns fixed metric constructors.
- `scheduler`: its fluent surface is already method-based.
- `wgo`: no typed transformation or receiver-function workaround was found.
- `wire`: no API migration candidate exists; it is tooling that must understand
  the new syntax.
- `goforj`: its only generic function is a private map-key sorter. Built-in maps
  cannot receive local methods. GoForj's role is generator and consumer rollout.

Unaffected repos should retain their existing Go version floors.

## Compatibility Policy

### Keep package functions

For public direct migrations, the old and new forms should coexist:

```go
httpx.Get[User](client, url)
client.Get[User](url)
```

The method becomes the preferred documentation form. The function becomes a
thin compatibility wrapper.

Do not immediately deprecate the functions. The method form is an ergonomic
improvement, not a correctness fix. Reconsider deprecation only after real usage
and migration data, and only under each module's compatibility policy.

### Minimum Go version

Source that declares a generic method requires Go 1.27 language support. Each
adopting module must set a clear Go 1.27 minimum rather than expose a
toolchain-dependent public method set through version build tags.

This language change is backward-compatible with existing Go source, but a
module's minimum-toolchain increase is still a consumer compatibility event.
The GoForj release policy is:

- a patch release must not raise the minimum Go version
- a minor release may raise the minimum Go version when the change is prominent
  in release notes and installation documentation
- a minimum-Go increase alone does not require a new major module path
- a major release remains reserved for incompatible source API, contract, or
  behavior changes
- consumers that cannot move to the new toolchain can remain pinned to the
  previous minor line

This policy must be published in the affected sibling repos before, or as part
of, the first Go 1.27 release. README Go-version badges and CI matrices must be
updated at the same time; several currently disagree with their `go.mod` files.

Do not bump every sibling module in lockstep. Modules that declare the new
methods, plus modules that select dependencies requiring Go 1.27, need the new
floor. Repositories that do neither should retain their current floor.

GoForj itself imports several affected modules. Consuming their new releases and
emitting calls to the methods therefore requires a coordinated GoForj minimum-Go
update and updated generated project metadata.

`collection` has a separate release blocker: its module path is currently
unsuffixed while the repository has `v2` tags. Resolve that semantic import
versioning mismatch before choosing or publishing the generic-method release
tag. Do not create another major solely to work around the existing mismatch.

### No conditional public API

Do not keep a lower module version and expose methods only from `go1.27`-tagged
files. Even if a build-tag arrangement can be made to compile, users would see a
different public method set based on toolchain version, and documentation,
completion, examples, and support would become ambiguous.

## Tooling Readiness

### Documentation generators

The additive strategy creates a package function and method with the same bare
name. Documentation tooling must use receiver-qualified identities such as
`Get` and `Client.Get`.

Fix before adding same-named compatibility functions and methods:

- `collection/docs/readme`
- `collection/docs/gen`
- `httpx/docs/readme`
- `httpx/docs/examplegen`

The current generators key declarations by the bare function name and can merge,
overwrite, or generate colliding paths. `cache`, `queue`, `web`, `storage`,
`mail`, and `crypt` already have receiver-aware identity logic and are useful
references.

`execx/docs/readme` and `execx/docs/examplegen` also key by bare name, but the
proposed `DecodeChain.As` has no current name collision. Treat receiver-qualified
identity there as tooling hardening rather than a blocker for `As[T]`.

After every public API change, regenerate and verify README, examples, and any
API indexes as part of the release—not as follow-up work.

### Webindex and Forj API indexing

An instantiated generic call wraps its selector in `ast.IndexExpr` or
`ast.IndexListExpr`. An analyzer that assumes `CallExpr.Fun` is directly an
`ast.SelectorExpr` misses calls such as:

```go
web.Bind[CreateMonitorInput](ctx)
controller.Handle[Payload]
controller.Routes[Config]()
```

Before generic helpers or generic handler values appear in generated web code:

- centralize unwrapping of parentheses, `IndexExpr`, and `IndexListExpr`
- normalize the declared selector name separately from its type arguments
- teach request-body inference about `web.Bind[T](ctx)`
- test instantiated handlers, middleware, and route providers
- test runtime route names produced by `runtime.FuncForPC`
- preserve type arguments separately when they help schema inference

`webindex` is the source-indexing engine; GoForj's API index runner delegates to
it. Repair the callable handling in `webindex`, then validate it through
GoForj's integration and API-index tests. Generic instantiation must not make a
route or request body disappear from generated metadata.

### GoForj Wire fork

The GoForj Wire fork is a release gate:

- provider parsing currently recognizes method expressions only as plain
  `ast.SelectorExpr`
- instantiated functions and methods introduce `IndexExpr` or `IndexListExpr`
- the AST copier omits function and type declaration type parameters
- the copier handles `IndexExpr` but not multi-argument `IndexListExpr`
- the fork still targets Go 1.19 and uses an older `x/tools`

The Go 1.19 directive is inventory, not by itself the parser failure when Wire
runs under a Go 1.27 toolchain. The concrete blockers are the outdated tooling
dependency, narrow provider-expression handling, and incomplete AST copying. A
directive bump may follow the dependency update but is not the fix on its own.

Add coverage for:

- an instantiated generic function provider
- an instantiated generic method expression provider
- one and multiple type arguments
- copied generic function and type declarations
- packages that declare generic methods even when the provider itself is
  non-generic

Release the repaired Wire fork and update GoForj's pinned tool before generated
Apps consume generic-method sibling releases.

### Generated set rewriters

GoForj's generated `make:model` analyzer, mirrored in Ship, preserves Wire
providers only when they are plain selectors. A provider such as
`pkg.Provider[T]` or an instantiated generic method expression may be silently
dropped when a set is rewritten.

Teach those rewriters to unwrap and preserve indexed callables before the
feature is allowed in provider sets.

### Reflection

Do not add any discovery path that relies on `reflect.Type.MethodByName` or method
indexes for generic methods. The accepted design intentionally excludes them
from reflection.

Existing reflection that decodes a concrete value, invokes legacy event
handlers, or inspects input shapes does not automatically disappear when a
method becomes generic. Remove reflection only where the new API actually makes
the runtime check unnecessary, as with `execx.As[T]` caller destinations.

## Resume Checklist

This design captures decisions as of 2026-07-13. Before implementation resumes:

1. Replace the draft/RC language status with the final Go 1.27 release notes and
   verify that syntax, interface, inference, and reflection rules are unchanged.
2. Refresh every affected repo's latest tag, module path, `go` directive, README
   Go badge, and CI toolchain matrix.
3. Re-run the receiver-style API scan in case sibling surfaces changed after the
   research date.
4. Confirm the minimum-Go minor-release policy is present in each affected repo.
5. Resolve `collection`'s module-path/tag mismatch.
6. Re-audit Wire, `webindex`, generated set rewriters, documentation generators,
   gopls, lint, and CI against the stable Go 1.27 toolchain.
7. Reconfirm that no generated interface boundary was replaced merely to obtain
   generic method syntax.

If any final language rule or repository API differs from this document, update
the design before implementation rather than silently adapting during rollout.

## Rollout Plan

### Phase 0: readiness before stable Go 1.27

1. Keep this design as the cross-repo contract.
2. Add Go 1.27 RC CI experiments without publishing production modules.
3. Repair Wire, source analyzers, set rewriters, and documentation identities.
4. Publish the minimum-Go minor-release policy and correct README/CI version
   claims that disagree with `go.mod`.
5. Resolve `collection`'s module-path/tag mismatch.
6. Add compile-only prototypes in temporary branches or `/tmp` modules where
   useful; do not put RC-only syntax in released branches.

### Phase 1: stable compiler canary

After stable Go 1.27:

1. Convert Ship's internal `lighthouse.Decode[T](cmd)` to `cmd.Decode[T]()`.
2. Run its full CI, lint, docs, and editor workflows.
3. Exercise method calls, method values, and method expressions.
4. Resolve tool failures before any public module release.

### Phase 2: direct public migrations

Implement in this order:

1. `collection`
2. `cache`
3. `httpx`

For each repo:

1. make docs tooling receiver-aware first
2. add methods over the existing shared implementation
3. retain package-function wrappers
4. add behavior-parity tests for both call forms
5. regenerate documentation and examples
6. run unit, contract, integration, vet, and lint checks appropriate to the repo
7. publish according to the repo's minimum-Go release policy

The order starts with the broadest language/API exercise, then validates the
concrete generated-App cache surface, then the larger HTTP verb family.

### Phase 3: focused new APIs

1. Add `execx.DecodeChain.As[T]`.
2. Add queue `Job.PayloadAs[T]` and `Message.PayloadAs[T]`.
3. Update generated job templates only after the queue release is consumed.
4. Evaluate `collection.ReduceTo` or `Fold` independently.
5. Consider `web.Bind[T]` and related helpers independently of Go 1.27, after
   `webindex` is ready.

### Phase 4: GoForj generated surface

After sibling releases are available:

1. bump GoForj dependencies and its Go version together
2. replace generated cache helper calls with receiver methods
3. prefer HTTP client methods in tests and examples
4. prefer queue payload result methods in generated handlers
5. update starter kits and demo templates
6. render and test disposable Apps only under `/tmp`
7. verify source indexing, Wire generation, docs, build, and runtime smoke

Do not change generated event, storage, or web receiver types merely to expose
generic methods.

## Validation Matrix

Every adopting repo should validate:

- explicit result type arguments
- type inference from values and callbacks
- method values and method expressions
- pointer and value receivers as applicable
- nil-receiver behavior where the existing function accepted a nil pointer
- exact parity between method and compatibility function results/errors
- documentation identity for same-named functions and methods
- reflection-dependent code paths do not expect the generic method
- lint, vet, static analysis, and generated examples

Use the GoForj cache defaults for Go commands:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
```

For GoForj integration:

- release or locally replace the sibling module intentionally
- render into `/tmp`, never into the GoForj repository
- build and test the emitted App
- verify Wire generation
- verify API indexing and OpenAPI artifacts
- inspect generated README/example output in every changed sibling

## Risks

### Tool lag

The accepted proposal warns that third-party tooling may take one or two release
cycles to catch up. GoForj owns enough AST, code generation, and Wire machinery
that compiler success alone is not a release signal.

### Interface confusion

Adding a generic method only to a concrete implementation can create two unequal
API tiers. Avoid that in event, web, storage, and driver contracts.

### Baseline churn

The API additions are source-additive, but the Go 1.27 module floor may exclude
consumers on older toolchains. Treat the floor change as an explicit release
decision.

### Documentation collisions

Keeping functions and methods with the same name is the right compatibility
strategy, but bare-name generators can silently merge their docs. Receiver-aware
identity is a hard prerequisite.

### Generic novelty

An unconstrained type parameter on an input-only method often provides no more
safety than `any`. Require a real type relationship or result benefit.

### Accidental semantic changes

Moving a function into a namespace must not quietly change context binding,
default-client behavior, codecs, observation names, nil handling, or errors.
Methods and wrappers should share one implementation.

## Final Recommendation

Adopt generic methods as a focused API correction for the receiver-first
workarounds already present in `cache`, `collection`, and `httpx`.

The feature's best GoForj outcome is not “put generics everywhere.” It is:

- concrete resources read naturally from left to right
- type-changing collection pipelines remain fluent
- HTTP calls live on the client that performs them
- old function calls keep working
- interface-led primitives keep their abstraction strength
- generated Apps gain the new syntax only after the compiler and GoForj toolchain
  are ready

That gives GoForj a materially cleaner public surface without turning a language
release into an ecosystem-wide redesign.

## Sources

- [Draft Go 1.27 release notes](https://go.dev/doc/go1.27)
- [Accepted generic methods proposal, golang/go#77273](https://github.com/golang/go/issues/77273)
- [Official Go downloads, including Go 1.27 RC1](https://go.dev/dl/)
- [Go module version numbering](https://go.dev/doc/modules/version-numbers)
- [Go major-version guidance](https://go.dev/doc/modules/major-version)
- [Go modules reference: minimum Go version](https://go.dev/ref/mod#go-mod-file-go)
- Local API audit of GoForj-owned modules under `/workspace/code`, 2026-07-13
