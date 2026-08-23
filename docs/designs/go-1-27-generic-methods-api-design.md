# Go 1.27 Generic Methods API Design

## Status

- Design status: implemented in the library adoption branches
- Research date: 2026-07-13; stable revalidation: 2026-08-23
- Language status: shipped in stable Go 1.27.0 on 2026-08-19
- Scope: GoForj-owned sibling libraries with receiver-owned generic operations
- Release policy decision: a minimum-Go increase ships in a minor release, never
  a patch; a major is reserved for incompatible API or behavior changes
- Generated-application adoption is deferred consumer work, not a release gate
  for the library APIs

Implementation pull requests: [ship#3](https://github.com/goforj/ship/pull/3),
[collection#5](https://github.com/goforj/collection/pull/5),
[cache#9](https://github.com/goforj/cache/pull/9),
[httpx#10](https://github.com/goforj/httpx/pull/10),
[execx#10](https://github.com/goforj/execx/pull/10), and
[queue#13](https://github.com/goforj/queue/pull/13).

## Decision Summary

GoForj should adopt generic methods only where a
concrete receiver already owns the operation.

The first adoption wave migrates the 33 public generic functions that
exist primarily because Go could not previously attach their type parameters to
a method:

- 9 functions in `cache`
- 10 functions in `collection`
- 14 functions in `httpx`

Ship also has one internal receiver-style generic function that is a useful
low-risk canary.

The migrations are additive:

- add the generic method
- keep the current package function as a compatibility entry point
- preserve behavior, error contracts, and encoding policy
- update documentation generators to distinguish package functions from methods

Generic methods should not be forced onto interface-first APIs. In particular:

- keep `events.API` unchanged
- keep `web.Context`, `web.WebSocketConn`, and `web.Router` unchanged
- keep `storage.Storage` byte-oriented and unchanged
- keep cache's interface contracts non-generic

`execx.DecodeChain.As[T]` and queue payload result methods are adjacent additions,
not migrations of existing generic package functions. They use the same
result-owned method capability to remove caller-side destination pointers.

The Go 1.27 floor does not by itself trigger new major module paths. Public
generic methods are additive, and the existing functions remain compatible.
Affected `v0`, `v1`, and `v2` modules should use their next minor release and
announce the new floor prominently. Patch releases must not raise the minimum
Go version.

## Why This Fits GoForj

GoForj separates generated application policy from reusable sibling primitives:

- sibling repos own reusable cache, collection, HTTP, queue, web, events, and
  storage behavior
- `goforj` owns templates, generated composition, wiring, and developer workflow

Generic methods do not change that ownership. The method belongs in the sibling
repo that owns the concrete type. Generated consumers can adopt released methods
later without blocking the libraries that declare them.

This matters most where generated applications already expose concrete objects:

- cache managers return `*cache.Cache`
- HTTP clients are `*httpx.Client`
- job handlers receive concrete `queue.Message` values

It matters much less where generated applications intentionally depend on abstractions:

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

| Repository | Audited Go | Generic receiver-style audit | Stable decision |
|---|---:|---:|---|
| `cache` | 1.24.4 | 9 | Implemented as methods; function entry points remain. |
| `collection` | 1.24.4 | 10 viable, 1 stable-compiler rejection, 10 false positives | Implemented the 10 viable methods; retain `Zip` and constrained functions. |
| `httpx/v2` | 1.24.4 | 14 | Implemented `Client` methods; package verbs and `Do[T]` remain. |
| `ship` | 1.26.1 | 1 internal | Implemented as the stable compiler canary. |
| `execx` | 1.24.4 | 0 | Implemented `DecodeChain.As[T]` as new API. |
| `queue` | 1.24.4 | 0 | Implemented `Job.PayloadAs[T]` and `Message.PayloadAs[T]`; retain `Bind`. |
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

The public receiver-migration count is therefore 33, concentrated in three
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

### Implemented methods

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

### Implemented methods

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

### Stable compiler exception: `Zip`

The Go 1.27.0 compiler rejects the natural method shape:

```go
func (c *Collection[T]) Zip[U any](
    other *Collection[U],
) *Collection[Tuple[T, U]]
```

Instantiating the result recursively requires methods for
`Collection[Tuple[T, U]]`, which in turn requires another `Zip` result method
set. The compiler reports an instantiation cycle. The corresponding Go issue
was closed as not planned because this method-set expansion is intentionally
rejected rather than bounded heuristically. `Zip` therefore remains a package
function. This is a stable language constraint, not postponed implementation
work.

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

### Implemented methods

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

Add methods to both public concrete payload values:

```go
func (j Job) PayloadAs[T any]() (T, error)
func (m Message) PayloadAs[T any]() (T, error)
```

The queue API changed after the initial audit: `queue.Message` is now a concrete
root type rather than an alias of `bus.Context`. The method belongs directly on
that public type:

```go
payload, err := msg.PayloadAs[EmailPayload]()
if err != nil {
	return err
}
```

`PayloadAs` avoids collisions with the existing `Job.Payload(any)` setter and
the existing `Bind(any)` methods.

Queue's receiver-aware documentation tooling can index both methods directly as
`Job.PayloadAs` and `Message.PayloadAs`; no alias-specific documentation path is
needed.

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

These helpers are also legal before Go 1.27. They could improve generated
handler code without weakening the web abstraction, but they are independent of
this migration.

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

The method becomes the preferred documentation form. The function remains a
compatibility entry point backed by the same implementation where practical.

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

The adoption branches update the affected generators before regenerating docs:

- `collection/docs/readme`
- `collection/docs/gen`
- `httpx/docs/readme`
- `httpx/docs/examplegen`

Those generators now use receiver-qualified identities and paths. `cache` and
`queue` already had receiver-aware identity logic and generated their new method
entries without collisions.

`execx/docs/readme` and `execx/docs/examplegen` also key by bare name, but the
implemented `DecodeChain.As` has no current name collision, so no generator
identity change is required for that API.

After every public API change, regenerate and verify README, examples, and any
API indexes as part of the release—not as follow-up work.

### Deferred generated consumers

These library PRs declare and call ordinary receiver methods. GoForj template
adoption is outside this release wave and is not a library release gate.

Any later generated-consumer change should be designed and validated separately.
It should not be bundled into the reusable-library API correction.

### Reflection

Do not add any discovery path that relies on `reflect.Type.MethodByName` or method
indexes for generic methods. The accepted design intentionally excludes them
from reflection.

Existing reflection that decodes a concrete value, invokes legacy event
handlers, or inspects input shapes does not automatically disappear when a
method becomes generic. Remove reflection only where the new API actually makes
the runtime check unnecessary, as with `execx.As[T]` caller destinations.

## Stable Revalidation Outcome

The 2026-08-23 revalidation confirmed the final syntax, inference, interface,
and reflection rules against Go 1.27.0. It also found two repository-level
changes that this document now incorporates:

1. `Collection.Zip` is not a viable generic method because the stable compiler
   rejects its recursively expanding result method set as an instantiation
   cycle. The package function remains canonical.
2. Queue retired the public `bus.Context` alias arrangement. Payload result
   methods belong directly to concrete root `Job` and `Message` values.

Each public-library branch updates its relevant module floors, installation
guidance, CI, tests, benchmarks, and generated documentation in the same change.
No interface boundary is replaced to obtain method syntax.

Paired benchmarks found allocation parity between compatibility and method
forms across collection, cache, and HTTP hot paths, and between the old and new
execx and queue result forms. Collection retains a direct `ZipWith` function
body because delegating it through the method would exceed the compiler's inline
budget on an established hot path.

## Rollout Plan

### Phase 1: stable compiler canary — complete

Ship's internal `lighthouse.Decode[T](cmd)` is converted to `cmd.Decode[T]()`,
with production call sites and method value/expression coverage updated.

### Phase 2: direct public migrations — implemented in PR branches

Implemented in this order:

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

The order starts with the broadest language/API exercise, then the context-bound
cache surface, then the larger HTTP verb family.

### Phase 3: focused new APIs — implemented in PR branches

1. Add `execx.DecodeChain.As[T]`.
2. Add queue `Job.PayloadAs[T]` and `Message.PayloadAs[T]`.
3. Evaluate `collection.ReduceTo` or `Fold` independently.
4. Consider `web.Bind[T]` and related helpers independently of Go 1.27.

### Deferred: GoForj generated consumers

After sibling releases are available:

1. bump GoForj dependencies and its Go version together
2. replace generated cache helper calls with receiver methods
3. prefer HTTP client methods in tests and examples
4. prefer queue payload result methods in generated handlers
5. update starter kits and demo templates
6. render and test disposable Apps only under `/tmp`
7. verify generated documentation, build, and runtime behavior

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
- allocation-count and timing comparisons between method and compatibility
  forms, with an established main-branch baseline for hot paths
- documentation identity for same-named functions and methods
- reflection-dependent code paths do not expect the generic method
- lint, vet, static analysis, and generated examples

Use the GoForj cache defaults for Go commands:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
```

For a later GoForj consumer change:

- release or locally replace the sibling module intentionally
- render into `/tmp`, never into the GoForj repository
- build and test the emitted App
- verify any generated API artifacts affected by that separate change
- inspect generated README/example output in every changed sibling

## Risks

### Tool lag

Each library must validate its own compiler, docs, examples, lint, and CI paths.
Generated-App tooling is validated when GoForj actually begins emitting the new
calls; it is not coupled to declaration-only library releases.

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

### Accidental performance regressions

An additive method can still make a compatibility function exceed the compiler's
inline budget or add a wrapper frame to a hot path. Preserve allocation counts,
benchmark both call forms, and retain a direct implementation when delegation
measurably regresses an established performance-sensitive entry point. The
`collection.ZipWith` compatibility function follows that exception.

## Final Recommendation

Adopt generic methods as a focused API correction for the receiver-first
workarounds already present in `cache`, `collection`, and `httpx`.

The feature's best GoForj outcome is not “put generics everywhere.” It is:

- concrete resources read naturally from left to right
- type-changing collection pipelines remain fluent
- HTTP calls live on the client that performs them
- old function calls keep working
- interface-led primitives keep their abstraction strength
- generated applications can adopt the new syntax in a separate consumer change

That gives GoForj a materially cleaner public surface without turning a language
release into an ecosystem-wide redesign.

## Sources

- [Go 1.27 release notes](https://go.dev/doc/go1.27)
- [Accepted generic methods proposal, golang/go#77273](https://github.com/golang/go/issues/77273)
- [Official Go downloads, including Go 1.27.0](https://go.dev/dl/)
- [Go module version numbering](https://go.dev/doc/modules/version-numbers)
- [Go major-version guidance](https://go.dev/doc/modules/major-version)
- [Go modules reference: minimum Go version](https://go.dev/ref/mod#go-mod-file-go)
- Local API audit of GoForj-owned sibling modules, revalidated 2026-08-23
- [Stable compiler rejection for generic `Zip`, golang/go#80109](https://github.com/golang/go/issues/80109)
